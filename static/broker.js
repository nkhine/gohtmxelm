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
    this.observeRemovals();
    this.connectStore();
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
      return;
    }
    // Gap 1: Elm triggers an HTMX swap without a server round-trip.
    if (event.type === "HTMX_SWAP") {
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
    this.syncToStore(key, value);
    return true;
  }

  // Gap 3: carry the known version for optimistic locking; handle 409 by
  // logging — the SSE stream will deliver the winning value automatically.
  syncToStore(key, value) {
    const storeValue =
      typeof value === "string" ? value : JSON.stringify(value);
    const version = this.storeVersions.get(key) || 0;
    fetch("/api/store", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, value: storeValue, version }),
    })
      .then((res) => {
        if (res.status === 409) {
          console.warn(
            `store conflict on key "${key}" (expected version ${version}); ` +
              "value will be corrected by the next SSE event"
          );
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
    const source = new EventSource("/api/events");

    source.addEventListener("store-hydrate", (e) => {
      this.applyStoreEvent(e, false);
    });

    source.addEventListener("store-change", (e) => {
      this.applyStoreEvent(e, true);
    });

    source.onerror = () => {
      source.close();
      setTimeout(() => this.connectStore(), 3000);
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
    const { key, value, version, seq } = parsed;

    // Gap 3: discard stale deltas; hydration events bypass the check.
    if (checkSeq && seq !== undefined && seq <= this.storeSeq) {
      return;
    }
    if (seq !== undefined) this.storeSeq = Math.max(this.storeSeq, seq);
    if (version !== undefined) this.storeVersions.set(key, version);

    this.state = { ...this.state, [key]: value };

    this.broadcast({
      version: this.version,
      type: "STORE_CHANGE",
      source: "broker",
      target: "broadcast",
      payload: { key, value },
    });

    const storeEl = document.getElementById("store-entries");
    if (storeEl && window.htmx) {
      window.htmx.trigger(storeEl, "store-refresh");
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
}

window.ElmIslandBroker = new ElmIslandBroker();

document.addEventListener("DOMContentLoaded", () => {
  window.ElmIslandBroker.mountAll(document);
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
  this.broadcast({
    version: this.version,
    type: "HTMX_AFTER_SWAP",
    source: "broker",
    target: "broadcast",
    payload: { targetId, url },
  });
};
