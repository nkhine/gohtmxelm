// gohtmxelm broker — the single, generic connective tissue between Elm islands
// and the rest of the page (HTMX, Datastar, SSE, the server).
//
// Responsibilities, and nothing more:
//   • mount / unmount Elm islands declared with data-elm-module
//   • speak one versioned envelope: { version, type, source, target, payload }
//   • route by target: a named island, "broadcast", "others", or "broker"
//   • hold shared broker state (STATE_SET / STATE_PATCH) and replay it to islands
//   • bridge any number of SSE sources to islands as generic SSE_EVENT messages
//   • run htmx.ajax swaps requested by islands (HTMX_SWAP)
//
// It contains NO application policy (no store endpoints, no optimistic locking,
// no activity log). Host pages observe `gohtmxelm:*` DOM CustomEvents and add
// their own behaviour on top. See demo/static/demo-ui.js for an example.

const PROTOCOL_VERSION = 1;

class GoHTMXElmBroker {
  constructor(config = {}) {
    this.version = PROTOCOL_VERSION;
    this.config = config;
    this.apps = new Map(); // islandId -> { app, node }
    this.nodes = new WeakMap(); // node -> islandId
    this.failedNodes = new WeakSet();
    this.state = {};
    this.lastSSE = new Map(); // event name -> last forwarded payload
    this.sources = []; // [{ url, names, source, retry }]
    this.active = false;
    this.observeRemovals();
    this.resume();
  }

  // ── Island lifecycle ──────────────────────────────────────────────────────

  mountAll(root = document) {
    root.querySelectorAll(".elm-island").forEach((node) => this.mount(node));
  }

  mount(node) {
    if (this.nodes.has(node) || this.failedNodes.has(node)) return;
    const moduleName = node.dataset.elmModule;
    const islandId = node.dataset.islandId || node.id;
    if (!moduleName || !islandId) return this.fail(node, "missing module or island id");
    const ElmModule = window.Elm?.[moduleName];
    if (!ElmModule) return this.log(`Elm module not loaded yet: ${moduleName}`);

    let props;
    try {
      props = JSON.parse(node.dataset.props || "{}");
    } catch (err) {
      console.error("Invalid data-props JSON", node, err);
      return this.fail(node, "invalid props");
    }

    let app;
    try {
      app = ElmModule.init({ node, flags: { ...props, islandId } });
    } catch (err) {
      console.error(`Elm init failed for island ${islandId}`, err);
      return this.fail(node, "Elm.init failed");
    }
    this.apps.set(islandId, { app, node });
    this.nodes.set(node, islandId);
    this.wirePorts(islandId, app);
    this.emit("mounted", { islandId, module: moduleName });
  }

  wirePorts(islandId, app) {
    if (!app.ports?.brokerOut) {
      this.log(`island ${islandId} has no brokerOut port`);
      return;
    }
    app.ports.brokerOut.subscribe((event) => this.handleEvent(islandId, event));
  }

  unmount(node) {
    const islandId = this.nodes.get(node);
    if (!islandId) return;
    this.apps.delete(islandId);
    this.nodes.delete(node);
  }

  // ── Envelope handling ─────────────────────────────────────────────────────

  handleEvent(sourceId, rawEvent) {
    const event = this.normalise(sourceId, rawEvent);
    if (!event) return;

    switch (event.type) {
      case "READY":
        this.completeHandshake(sourceId);
        return;
      case "STATE_SET":
        this.applyStateSet(event);
        break;
      case "STATE_PATCH":
        this.applyStatePatch(event);
        break;
      case "HTMX_SWAP":
        this.executeHtmxSwap(event.payload);
        this.emit("htmx-swap", event.payload);
        return;
    }
    this.route(event);
  }

  normalise(sourceId, rawEvent) {
    if (!rawEvent || typeof rawEvent !== "object") return null;
    if (rawEvent.version !== this.version) {
      console.warn("broker: unsupported envelope version", rawEvent);
      return null;
    }
    if (typeof rawEvent.type !== "string" || !rawEvent.type) return null;
    if (typeof rawEvent.target !== "string" || !rawEvent.target) return null;
    if (rawEvent.source) {
      // source is stamped by the broker; an island may not name itself.
      console.warn("broker: ignoring caller-supplied source", rawEvent);
    }
    return {
      version: rawEvent.version,
      type: rawEvent.type,
      source: sourceId,
      target: rawEvent.target,
      payload: rawEvent.payload || {},
    };
  }

  completeHandshake(islandId) {
    this.sendTo(islandId, this.envelope("BROKER_READY", islandId, { islandId }));
    // Replay the last value seen on each SSE source so late-mounting islands
    // are immediately current without waiting for the next push.
    for (const [event, data] of this.lastSSE) {
      this.sendTo(islandId, this.envelope("SSE_EVENT", islandId, { event, data }));
    }
  }

  applyStateSet(event) {
    const { key, value } = event.payload;
    if (typeof key !== "string" || !key) return;
    this.state = { ...this.state, [key]: value };
    this.emit("state-set", { key, value, source: event.source });
  }

  applyStatePatch(event) {
    if (event.payload && typeof event.payload === "object" && !Array.isArray(event.payload)) {
      this.state = { ...this.state, ...event.payload };
      this.emit("state-patch", { patch: event.payload, source: event.source });
    }
  }

  // ── Routing ───────────────────────────────────────────────────────────────

  route(event) {
    if (event.target === "broker") return;
    if (event.target === "broadcast") return this.broadcast(event);
    if (event.target === "others") return this.broadcast(event, event.source);
    this.sendTo(event.target, event);
  }

  broadcast(event, exceptId = null) {
    for (const islandId of this.apps.keys()) {
      if (islandId !== exceptId) this.sendTo(islandId, event);
    }
  }

  sendTo(islandId, event) {
    const record = this.apps.get(islandId);
    if (!record?.app?.ports?.brokerIn) return;
    queueMicrotask(() => {
      record.app.ports.brokerIn.send({ ...event, brokerState: structuredClone(this.state) });
    });
  }

  envelope(type, target, payload) {
    return { version: this.version, type, source: "broker", target, payload };
  }

  // ── HTMX bridge ───────────────────────────────────────────────────────────

  executeHtmxSwap(payload = {}) {
    const { selector, url, swap = "innerHTML" } = payload;
    if (!selector || !url || !window.htmx) return;
    const target = document.querySelector(selector);
    if (target) window.htmx.ajax("GET", url, { target, swap });
  }

  // ── SSE bridge ────────────────────────────────────────────────────────────

  connectSources() {
    if (!this.active) return;
    const configs = Array.isArray(this.config.sources) ? this.config.sources : [];
    if (this.sources.length === 0) {
      this.sources = configs.map((c) => ({
        url: c.url,
        names: Array.isArray(c.events) && c.events.length ? c.events : ["message"],
        source: null,
        retry: null,
      }));
    }
    this.sources.forEach((s) => this.openSource(s));
  }

  openSource(s) {
    if (!this.active || s.source) return;
    const source = new EventSource(s.url);
    s.source = source;
    s.names.forEach((name) => {
      source.addEventListener(name, (event) => this.forwardSSE(name, event));
    });
    source.onopen = () => this.emit("source-open", { url: s.url });
    source.onerror = () => {
      if (!this.active || source !== s.source) return;
      this.emit("source-error", { url: s.url });
      source.close();
      s.source = null;
      s.retry = setTimeout(() => {
        s.retry = null;
        this.openSource(s);
      }, 3000);
    };
  }

  forwardSSE(name, event) {
    let data = event.data;
    try {
      data = JSON.parse(event.data);
    } catch (_) {
      // Plain-text SSE payloads are valid; forward them unchanged.
    }
    this.lastSSE.set(name, data);
    this.emit("sse", { event: name, data });
    this.broadcast(this.envelope("SSE_EVENT", "broadcast", { event: name, data }));
  }

  // ── Activation / teardown ─────────────────────────────────────────────────

  pause() {
    this.active = false;
    this.sources.forEach((s) => {
      if (s.retry) clearTimeout(s.retry);
      s.retry = null;
      if (s.source) s.source.close();
      s.source = null;
    });
  }

  resume() {
    if (this.active) return;
    this.active = true;
    this.connectSources();
    this.mountAll(document);
    if (window.htmx?.process && document.body) window.htmx.process(document.body);
  }

  // ── DOM observation ───────────────────────────────────────────────────────

  observeRemovals() {
    if (!document.body) return;
    new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const removed of mutation.removedNodes) this.unmountRemovedNode(removed);
      }
    }).observe(document.body, { childList: true, subtree: true });
  }

  unmountRemovedNode(node) {
    if (!(node instanceof Element)) return;
    if (node.classList.contains("elm-island")) this.unmount(node);
    node.querySelectorAll?.(".elm-island").forEach((child) => this.unmount(child));
  }

  // ── Host integration ──────────────────────────────────────────────────────

  // emit dispatches a DOM CustomEvent (gohtmxelm:<name>) so host pages can layer
  // their own behaviour without forking the broker.
  emit(name, detail) {
    document.dispatchEvent(new CustomEvent(`gohtmxelm:${name}`, { detail }));
  }

  fail(node, reason) {
    node.dataset.elmMountFailed = reason;
    this.failedNodes.add(node);
    this.emit("mount-failed", { reason });
  }

  log(message) {
    if (this.config.debug) console.debug(`[gohtmxelm] ${message}`);
  }
}

function configFromScript() {
  const script = document.currentScript;
  let sources = [];
  try {
    sources = JSON.parse(script?.dataset.sources || "[]");
  } catch (err) {
    console.error("broker: invalid data-sources JSON", err);
  }
  return { sources, debug: script?.dataset.debug === "true" };
}

window.GoHTMXElmBroker = new GoHTMXElmBroker(configFromScript());

document.addEventListener("DOMContentLoaded", () => {
  window.GoHTMXElmBroker.mountAll(document);
});

window.addEventListener("pagehide", () => {
  window.GoHTMXElmBroker.pause();
});

window.addEventListener("pageshow", (event) => {
  // A bfcache restore can revive stale ports and streams; reload for a clean
  // server-owned render.
  if (event.persisted) {
    window.location.reload();
    return;
  }
  window.GoHTMXElmBroker.resume();
});

document.body.addEventListener("htmx:afterSettle", () => {
  window.GoHTMXElmBroker.mountAll(document);
});

document.body.addEventListener("htmx:afterSwap", (e) => {
  const targetId = e.target?.id || null;
  const url = e.detail?.requestConfig?.path || null;
  window.GoHTMXElmBroker.broadcast(
    window.GoHTMXElmBroker.envelope("HTMX_AFTER_SWAP", "broadcast", { targetId, url })
  );
  window.GoHTMXElmBroker.emit("htmx-after-swap", { targetId, url });
});
