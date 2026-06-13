class GoHTMXElmBroker {
  constructor(config = {}) {
    this.version = 1;
    this.apps = new Map();
    this.nodes = new WeakMap();
    this.failedNodes = new WeakSet();
    this.state = {};
    this.config = config;
    this.source = null;
    this.retry = null;
    this.active = false;
    this.observeRemovals();
    this.resume();
  }

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

    try {
      const app = ElmModule.init({ node, flags: { ...props, islandId } });
      this.apps.set(islandId, { app, node });
      this.nodes.set(node, islandId);
      this.wirePorts(islandId, app);
      this.sendTo(islandId, {
        version: this.version,
        type: "BROKER_READY",
        source: "broker",
        target: islandId,
        payload: { islandId },
      });
    } catch (err) {
      console.error(`Elm init failed for island ${islandId}`, err);
      this.fail(node, "Elm.init failed");
    }
  }

  wirePorts(islandId, app) {
    if (!app.ports?.brokerOut) return;
    app.ports.brokerOut.subscribe((event) => this.handleEvent(islandId, event));
  }

  handleEvent(sourceId, rawEvent) {
    const event = this.normaliseEvent(sourceId, rawEvent);
    if (!event) return;
    if (event.type === "STATE_SET") {
      this.state = { ...this.state, [event.payload.key]: event.payload.value };
    }
    if (event.type === "STATE_PATCH") {
      this.state = { ...this.state, ...event.payload };
    }
    if (event.type === "HTMX_SWAP") {
      this.executeHtmxSwap(event.payload || {});
      return;
    }
    if (event.target === "broadcast") return this.broadcast(event);
    if (event.target === "others") return this.broadcast(event, event.source);
    if (event.target !== "broker") this.sendTo(event.target, event);
  }

  normaliseEvent(sourceId, rawEvent) {
    if (!rawEvent || typeof rawEvent !== "object") return null;
    if (rawEvent.version !== this.version) return null;
    if (!rawEvent.type || !rawEvent.target) return null;
    return {
      version: rawEvent.version,
      type: rawEvent.type,
      source: sourceId,
      target: rawEvent.target,
      payload: rawEvent.payload || {},
    };
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
      record.app.ports.brokerIn.send({
        ...event,
        brokerState: structuredClone(this.state),
      });
    });
  }

  executeHtmxSwap(payload) {
    const { selector, url, swap = "innerHTML" } = payload;
    if (!selector || !url || !window.htmx) return;
    const target = document.querySelector(selector);
    if (target) window.htmx.ajax("GET", url, { target, swap });
  }

  connectEvents() {
    if (!this.active || !this.config.events || this.source) return;
    const source = new EventSource(this.config.events);
    this.source = source;
    const names = this.config.eventNames.length ? this.config.eventNames : ["message"];
    names.forEach((name) => {
      source.addEventListener(name, (event) => this.forwardSSE(name, event));
    });
    source.onerror = () => {
      if (!this.active || source !== this.source) return;
      source.close();
      this.source = null;
      this.retry = setTimeout(() => {
        this.retry = null;
        this.connectEvents();
      }, 3000);
    };
  }

  forwardSSE(name, event) {
    let data = event.data;
    try {
      data = JSON.parse(event.data);
    } catch (_) {
      // Plain text SSE payloads are valid; forward them unchanged.
    }
    this.broadcast({
      version: this.version,
      type: "SSE_EVENT",
      source: "broker",
      target: "broadcast",
      payload: { event: name, data },
    });
  }

  observeRemovals() {
    if (!document.body) return;
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const removed of mutation.removedNodes) this.unmountRemovedNode(removed);
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  unmountRemovedNode(node) {
    if (!(node instanceof Element)) return;
    if (node.classList.contains("elm-island")) this.unmount(node);
    node.querySelectorAll?.(".elm-island").forEach((child) => this.unmount(child));
  }

  unmount(node) {
    const islandId = this.nodes.get(node);
    if (!islandId) return;
    this.apps.delete(islandId);
    this.nodes.delete(node);
  }

  pause() {
    this.active = false;
    if (this.retry) clearTimeout(this.retry);
    this.retry = null;
    if (this.source) this.source.close();
    this.source = null;
  }

  resume() {
    if (this.active) return;
    this.active = true;
    this.connectEvents();
    this.mountAll(document);
    if (window.htmx?.process && document.body) window.htmx.process(document.body);
  }

  fail(node, reason) {
    node.dataset.elmMountFailed = reason;
    this.failedNodes.add(node);
  }

  log(message) {
    if (this.config.debug) console.debug(`[gohtmxelm] ${message}`);
  }
}

function configFromScript() {
  const script = document.currentScript;
  return {
    events: script?.dataset.events || "",
    eventNames: (script?.dataset.eventNames || "")
      .split(",")
      .map((name) => name.trim())
      .filter(Boolean),
    debug: script?.dataset.debug === "true",
  };
}

window.GoHTMXElmBroker = new GoHTMXElmBroker(configFromScript());

document.addEventListener("DOMContentLoaded", () => {
  window.GoHTMXElmBroker.mountAll(document);
});

window.addEventListener("pagehide", () => {
  window.GoHTMXElmBroker.pause();
});

window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    window.location.reload();
    return;
  }
  window.GoHTMXElmBroker.resume();
});

document.body.addEventListener("htmx:afterSettle", () => {
  window.GoHTMXElmBroker.mountAll(document);
});
