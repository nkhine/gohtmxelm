class ElmIslandBroker {
  constructor() {
    this.version = 1;
    this.apps = new Map();
    this.nodes = new WeakMap();
    this.failedNodes = new WeakSet();
    this.state = {};
    // Gap 3: track the highest store sequence seen so stale SSE deltas are
    // discarded, and per-key versions so optimistic-lock POSTs carry the
    // correct expected version.
    this.storeSeq = 0;
    this.storeVersions = new Map();
    // Last stopwatch snapshot seen over SSE, replayed to islands that mount
    // late so the Elm lap analyzer is correct even while the timer is idle.
    this.lastStopwatch = null;
    this.active = false;
    this.storeSource = null;
    this.stopwatchSource = null;
    this.storeRetry = null;
    this.stopwatchRetry = null;
    this.observeRemovals();
    this.resume();
  }

  mountAll(root = document) {
    root.querySelectorAll(".elm-island").forEach((node) => {
      this.mount(node);
    });
  }

  mount(node) {
    if (this.nodes.has(node) || this.failedNodes.has(node)) {
      return;
    }
    const moduleName = node.dataset.elmModule;
    const islandId = node.dataset.islandId || node.id;
    if (!moduleName || !islandId) {
      this.fail(node, "missing data-elm-module or island id");
      return;
    }
    const ElmModule = window.Elm?.[moduleName];
    if (!ElmModule) {
      console.warn(`Elm module not loaded yet: ${moduleName}`);
      return;
    }
    const props = this.parseProps(node);
    if (props === null) {
      this.fail(node, "invalid data-props JSON");
      return;
    }
    let app;
    try {
      app = ElmModule.init({
        node,
        flags: {
          ...props,
          islandId,
        },
      });
    } catch (err) {
      console.error(`Elm init failed for island ${islandId}`, err);
      this.fail(node, "Elm.init failed");
      return;
    }
    this.apps.set(islandId, { app, node });
    this.nodes.set(node, islandId);
    this.wirePorts(islandId, app);
    this.addActivityEntry("broker", islandId, `mounted island`);
  }

  wirePorts(islandId, app) {
    if (!app.ports?.brokerOut) {
      console.warn(`Island ${islandId} has no brokerOut port`);
      return;
    }
    app.ports.brokerOut.subscribe((event) => {
      this.handleEvent(islandId, event);
    });
  }

  handleEvent(sourceId, rawEvent) {
    const event = this.normaliseEvent(sourceId, rawEvent);
    if (!event) {
      return;
    }
    if (event.type === "READY") {
      this.sendTo(sourceId, {
        version: this.version,
        type: "BROKER_READY",
        source: "broker",
        target: sourceId,
        payload: {
          islandId: sourceId,
        },
      });
      // Replay the latest stopwatch snapshot to this island so analytics are
      // populated immediately, without waiting for the next state change.
      if (this.lastStopwatch) {
        this.sendTo(sourceId, {
          version: this.version,
          type: "STOPWATCH_SNAPSHOT",
          source: "broker",
          target: sourceId,
          payload: this.lastStopwatch,
        });
      }
      return;
    }
    // Gap 1: Elm triggers an HTMX swap without a server round-trip.
    if (event.type === "HTMX_SWAP") {
      this.addActivityEntry("elm", "htmx", `HTMX_SWAP → ${event.payload.url} into ${event.payload.selector}`);
      this.executeHtmxSwap(event.payload);
      return;
    }
    if (!this.reduce(event)) {
      return;
    }
    this.route(event);
  }

  normaliseEvent(sourceId, rawEvent) {
    if (!rawEvent || typeof rawEvent !== "object") {
      console.warn("Invalid broker event", rawEvent);
      return null;
    }
    if (rawEvent.version !== this.version) {
      console.warn("Unsupported or missing broker event version", rawEvent);
      return null;
    }
    if (typeof rawEvent.type !== "string" || rawEvent.type.length === 0) {
      console.warn("Broker event missing type", rawEvent);
      return null;
    }
    if (typeof rawEvent.target !== "string" || rawEvent.target.length === 0) {
      console.warn("Broker event missing target", rawEvent);
      return null;
    }
    if (rawEvent.source) {
      console.warn("Broker event must not include source; source is stamped by broker", rawEvent);
      return null;
    }
    return {
      version: rawEvent.version,
      type: rawEvent.type,
      source: sourceId,
      target: rawEvent.target,
      payload: rawEvent.payload || {},
    };
  }

  reduce(event) {
    switch (event.type) {
      case "STATE_SET":
        return this.reduceStateSet(event);
      case "STATE_PATCH":
        return this.reduceStatePatch(event);
      case "SEND":
        return true;
      default:
        return true;
    }
  }

  reduceStateSet(event) {
    const { key, value } = event.payload;
    if (typeof key !== "string" || key.length === 0) {
      console.warn("Invalid STATE_SET payload", event);
      return false;
    }
    this.state = {
      ...this.state,
      [key]: value,
    };
    this.addActivityEntry("elm", "go", `STATE_SET ${key} from ${event.source}`);
    this.syncToStore(key, value, event.source);
    return true;
  }

  // Gap 3: carry the known version for optimistic locking; handle 409 by
  // logging — the SSE stream will deliver the winning value automatically.
  // source attributes the write to the originating island (e.g. "app-a"),
  // so every pane can show who last wrote.
  syncToStore(key, value, source) {
    const storeValue =
      typeof value === "string" ? value : JSON.stringify(value);
    const version = this.storeVersions.get(key) || 0;
    fetch("/api/store", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, value: storeValue, source, version }),
    })
      .then((res) => {
        if (res.status === 409) {
          console.warn(
            `store conflict on key "${key}" (expected version ${version}); ` +
              "value will be corrected by the next SSE event"
          );
          this.addActivityEntry("go", "broker", `409 conflict on key "${key}" — SSE will correct`);
        }
      })
      .catch((err) => console.warn("store sync failed", err));
  }

  reduceStatePatch(event) {
    if (
      !event.payload ||
      typeof event.payload !== "object" ||
      Array.isArray(event.payload)
    ) {
      console.warn("Invalid STATE_PATCH payload", event);
      return false;
    }
    this.state = {
      ...this.state,
      ...event.payload,
    };
    return true;
  }

  route(event) {
    switch (event.target) {
      case "broker":
        return;
      case "broadcast":
        this.broadcast(event);
        return;
      case "others":
        this.broadcast(event, event.source);
        return;
      default:
        this.sendTo(event.target, event);
    }
  }

  broadcast(event, exceptId = null) {
    for (const islandId of this.apps.keys()) {
      if (islandId === exceptId) {
        continue;
      }
      this.sendTo(islandId, event);
    }
  }

  sendTo(islandId, event) {
    const record = this.apps.get(islandId);
    if (!record?.app?.ports?.brokerIn) {
      return;
    }
    queueMicrotask(() => {
      record.app.ports.brokerIn.send({
        ...event,
        brokerState: structuredClone(this.state),
      });
    });
  }

  // Gap 1: execute an htmx.ajax call requested by an Elm island, targeting a
  // CSS selector in the existing server-rendered DOM.
  executeHtmxSwap(payload) {
    const { selector, url } = payload;
    if (!selector || !url) {
      console.warn("HTMX_SWAP missing selector or url", payload);
      return;
    }
    const el = document.querySelector(selector);
    if (!el) {
      console.warn("HTMX_SWAP target element not found:", selector);
      return;
    }
    if (!window.htmx) {
      console.warn("htmx not available, cannot execute swap");
      return;
    }
    window.htmx.ajax("GET", url, { target: el, swap: "innerHTML" });
  }

  // Gap 3: open an EventSource to /api/events.
  // store-hydrate events are applied unconditionally (they carry the full
  // current state on every reconnect).
  // store-change deltas are discarded when seq <= storeSeq to protect against
  // stale redelivery on flaky connections.
  connectStore() {
    if (!this.active || this.storeSource) {
      return;
    }
    const source = new EventSource("/api/events");
    this.storeSource = source;

    source.addEventListener("store-hydrate", (e) => {
      this.applyStoreEvent(e, false);
    });

    source.addEventListener("store-change", (e) => {
      this.applyStoreEvent(e, true);
    });

    source.onopen = () => {
      this.updateSseStatus(true);
      this.addActivityEntry("sse", "broker", "EventSource connected");
    };

    source.onerror = () => {
      if (!this.active || source !== this.storeSource) {
        return;
      }
      this.updateSseStatus(false);
      this.addActivityEntry("sse", "broker", "EventSource error — reconnecting in 3s");
      source.close();
      this.storeSource = null;
      this.storeRetry = setTimeout(() => {
        this.storeRetry = null;
        this.connectStore();
      }, 3000);
    };
  }

  applyStoreEvent(e, checkSeq) {
    let parsed;
    try {
      parsed = JSON.parse(e.data);
    } catch (err) {
      console.warn("store event parse error", err);
      return;
    }
    const { key, value, source, deleted, version, seq } = parsed;

    // Gap 3: discard stale deltas; hydration events bypass the check.
    if (checkSeq && seq !== undefined && seq <= this.storeSeq) {
      return;
    }
    if (seq !== undefined) this.storeSeq = Math.max(this.storeSeq, seq);

    if (deleted) {
      const { [key]: _gone, ...rest } = this.state;
      this.state = rest;
      this.storeVersions.delete(key);
    } else {
      if (version !== undefined) this.storeVersions.set(key, version);
      this.state = { ...this.state, [key]: value };
    }

    if (checkSeq) {
      const verb = deleted ? "STORE_DELETE" : "STORE_CHANGE";
      this.addActivityEntry("sse", "elm", `${verb} key="${key}" by=${source || "?"}`);
      this.flashStoreRow(key);
    } else {
      this.addActivityEntry("sse", "broker", `hydrate key="${key}"`);
    }

    this.broadcast({
      version: this.version,
      type: "STORE_CHANGE",
      source: "broker",
      target: "broadcast",
      payload: { key, value: deleted ? "" : value, source: source || "unknown", deleted: !!deleted },
    });

    const storeEl = document.getElementById("store-entries");
    if (storeEl && window.htmx) {
      window.htmx.trigger(storeEl, "store-refresh");
    }
  }

  // Stopwatch JSON stream: emits only on discrete state changes. Each event
  // is forwarded to Elm islands (the lap analyzer) and re-triggers the HTMX
  // controls fragment so every tab converges on the same control state.
  connectStopwatch() {
    if (!this.active || this.stopwatchSource) {
      return;
    }
    const source = new EventSource("/api/stopwatch/events");
    this.stopwatchSource = source;

    source.addEventListener("stopwatch-state", (e) => {
      this.applyStopwatchEvent(e);
    });

    source.onerror = () => {
      if (!this.active || source !== this.stopwatchSource) {
        return;
      }
      this.addActivityEntry("sse", "broker", "stopwatch stream error — reconnecting in 3s");
      source.close();
      this.stopwatchSource = null;
      this.stopwatchRetry = setTimeout(() => {
        this.stopwatchRetry = null;
        this.connectStopwatch();
      }, 3000);
    };
  }

  applyStopwatchEvent(e) {
    let snap;
    try {
      snap = JSON.parse(e.data);
    } catch (err) {
      console.warn("stopwatch event parse error", err);
      return;
    }

    const payload = {
      running: !!snap.running,
      laps: Array.isArray(snap.laps) ? snap.laps : [],
    };
    this.lastStopwatch = payload;

    this.addActivityEntry(
      "sse",
      "elm",
      `STOPWATCH_SNAPSHOT running=${payload.running} laps=${payload.laps.length}`
    );

    // Feed the Elm lap analyzer.
    this.broadcast({
      version: this.version,
      type: "STOPWATCH_SNAPSHOT",
      source: "broker",
      target: "broadcast",
      payload,
    });

    // Re-render the HTMX controls in every tab. The tab that initiated the
    // change already swapped via its POST response; this keeps the others
    // in sync. HTMX processes the swapped-in buttons, so they stay wired.
    if (window.htmx) {
      window.htmx.trigger(document.body, "stopwatch-state-change");
    }
  }

  observeRemovals() {
    if (!document.body) {
      return;
    }
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const removedNode of mutation.removedNodes) {
          this.unmountRemovedNode(removedNode);
        }
      }
    });
    observer.observe(document.body, {
      childList: true,
      subtree: true,
    });
  }

  unmountRemovedNode(node) {
    if (!(node instanceof Element)) {
      return;
    }
    if (node.classList.contains("elm-island")) {
      this.unmount(node);
    }
    node.querySelectorAll?.(".elm-island").forEach((child) => {
      this.unmount(child);
    });
  }

  unmount(node) {
    const islandId = this.nodes.get(node);
    if (!islandId) {
      return;
    }
    this.apps.delete(islandId);
    this.nodes.delete(node);
  }

  pause() {
    this.active = false;
    this.clearRetryTimers();
    this.closeSources();
    this.updateSseStatus(false);
  }

  resume() {
    if (this.active) {
      return;
    }
    this.active = true;
    this.connectStore();
    this.connectStopwatch();
    this.mountAll(document);
    if (window.htmx?.process && document.body) {
      window.htmx.process(document.body);
      window.htmx.trigger(document.body, "stopwatch-state-change");
    }
  }

  clearRetryTimers() {
    if (this.storeRetry) {
      clearTimeout(this.storeRetry);
      this.storeRetry = null;
    }
    if (this.stopwatchRetry) {
      clearTimeout(this.stopwatchRetry);
      this.stopwatchRetry = null;
    }
  }

  closeSources() {
    if (this.storeSource) {
      this.storeSource.close();
      this.storeSource = null;
    }
    if (this.stopwatchSource) {
      this.stopwatchSource.close();
      this.stopwatchSource = null;
    }
  }

  parseProps(node) {
    try {
      return JSON.parse(node.dataset.props || "{}");
    } catch (err) {
      console.error("Invalid data-props JSON", node, err);
      return null;
    }
  }

  fail(node, reason) {
    node.dataset.elmMountFailed = reason;
    this.failedNodes.add(node);
  }

  // ── Visual helpers ────────────────────────────────────────────────────────

  updateSseStatus(connected) {
    const el = document.getElementById("sse-status");
    const txt = document.getElementById("sse-status-text");
    if (!el || !txt) return;
    el.className = `sse-status ${connected ? "connected" : "disconnected"}`;
    txt.textContent = connected ? "SSE live" : "SSE disconnected";
  }

  addActivityEntry(from, to, description) {
    const container = document.getElementById("activity-entries");
    if (!container) return;

    // Remove the placeholder entry on first real event.
    const placeholder = container.querySelector(".log-entry:only-child");
    if (placeholder && placeholder.querySelector(".log-time")?.textContent === "--:--:--") {
      placeholder.remove();
    }

    const now = new Date();
    const time = [now.getHours(), now.getMinutes(), now.getSeconds()]
      .map((n) => String(n).padStart(2, "0"))
      .join(":");

    const fromClass = {
      elm: "log-from-elm",
      htmx: "log-from-htmx",
      sse: "log-from-sse",
      go: "log-from-go",
      datastar: "log-from-datastar",
      broker: "log-from-sse",
    }[from] || "";

    const entry = document.createElement("div");
    entry.className = "log-entry";
    entry.innerHTML =
      `<span class="log-time">${time}</span>` +
      `<span class="log-msg"><span class="${fromClass}">[${from}→${to}]</span> ${description}</span>`;

    container.prepend(entry);

    // Cap at 50 entries to avoid unbounded growth.
    while (container.children.length > 50) {
      container.removeChild(container.lastChild);
    }
  }

  // Flash the store table row for the given key after the DOM refreshes.
  flashStoreRow(key) {
    // The store-refresh HTMX swap happens shortly after; wait for it to settle.
    setTimeout(() => {
      const row = document.querySelector(`[data-store-key="${CSS.escape(key)}"]`);
      if (!row) return;
      row.classList.remove("store-row-flash");
      // Force reflow to restart the animation if it's already running.
      void row.offsetWidth;
      row.classList.add("store-row-flash");
    }, 120);
  }
}

window.ElmIslandBroker = new ElmIslandBroker();

document.addEventListener("DOMContentLoaded", () => {
  window.ElmIslandBroker.mountAll(document);
});

window.addEventListener("pagehide", () => {
  window.ElmIslandBroker.pause();
});

window.addEventListener("pageshow", (event) => {
  // The browser can restore this page from the back/forward cache with stale
  // HTMX handlers, Datastar fetch streams, and Elm ports. Reloading gives this
  // server-owned demo a fresh render while preserving stopwatch state in Go.
  if (event.persisted) {
    window.location.reload();
    return;
  }
  window.ElmIslandBroker.resume();
});

document.body.addEventListener("htmx:afterSettle", () => {
  window.ElmIslandBroker.mountAll(document);
});

// Gap 2: forward htmx:afterSwap into the broker so Elm islands can react to
// server-rendered fragment changes without polling.
document.body.addEventListener("htmx:afterSwap", (e) => {
  window.ElmIslandBroker.handleHtmxAfterSwap(e);
});

ElmIslandBroker.prototype.handleHtmxAfterSwap = function (e) {
  const targetId = e.target?.id || null;
  const url = e.detail?.requestConfig?.path || null;
  this.addActivityEntry("htmx", "elm", `afterSwap → #${targetId} from ${url}`);
  this.broadcast({
    version: this.version,
    type: "HTMX_AFTER_SWAP",
    source: "broker",
    target: "broadcast",
    payload: { targetId, url },
  });
};

// Datastar is deliberately not routed through the Elm broker: it owns its own
// DOM island. These listeners only make the teaching event log show when
// Datastar starts/finishes fetch-driven SSE work and when its signals change.
document.addEventListener("datastar-fetch", (e) => {
  const type = e.detail?.type || "unknown";
  const tag = e.detail?.el?.tagName?.toLowerCase() || "element";
  window.ElmIslandBroker.addActivityEntry("datastar", "go", `${type} from ${tag}`);
});

document.addEventListener("datastar-signal-patch", (e) => {
  const keys = Object.keys(e.detail || {}).join(", ") || "signals";
  window.ElmIslandBroker.addActivityEntry("datastar", "dom", `signal patch: ${keys}`);
});
