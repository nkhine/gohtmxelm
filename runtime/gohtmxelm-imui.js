// gohtmxelm IMUI runtime — immediate-mode canvas islands for high-frequency
// tooling surfaces. It owns lifecycle, input normalisation, resize handling,
// redraw scheduling, SSE delivery, and command posting. Host modules own draw
// logic and domain policy.

class GoHTMXElmIMUIRuntime {
  constructor(config = {}) {
    this.config = config;
    this.modules = new Map();
    this.instances = new Map(); // islandId -> record
    this.nodes = new WeakMap(); // canvas -> islandId
    this.failedNodes = new WeakSet();
    this.sources = [];
    this.active = false;
    this.usingBroker = !!window.GoHTMXElmBroker;
    this.observeRemovals();
    document.addEventListener("gohtmxelm:sse", (event) => {
      this.usingBroker = true;
      this.closeSources();
      this.deliverSSE(event.detail?.event, event.detail?.data);
    });
    this.resume();
  }

  register(name, module) {
    if (!name || !module) return;
    this.modules.set(name, module);
    this.mountAll(document);
  }

  mountAll(root = document) {
    root.querySelectorAll("[data-gohtmxelm-imui-module]").forEach((node) => this.mount(node));
  }

  mount(canvas) {
    if (!(canvas instanceof HTMLCanvasElement)) return;
    if (this.nodes.has(canvas) || this.failedNodes.has(canvas)) return;

    const moduleName = canvas.dataset.gohtmxelmImuiModule;
    const islandId = canvas.dataset.gohtmxelmImuiId || canvas.id;
    const module = this.modules.get(moduleName);
    if (!moduleName || !islandId) return this.fail(canvas, "missing module or island id");
    if (!module) return this.log(`IMUI module not registered yet: ${moduleName}`);

    let props;
    try {
      props = JSON.parse(canvas.dataset.props || "{}");
    } catch (err) {
      console.error("Invalid IMUI data-props JSON", canvas, err);
      return this.fail(canvas, "invalid props");
    }

    let events = null;
    try {
      const parsed = JSON.parse(canvas.dataset.events || "null");
      if (Array.isArray(parsed) && parsed.length > 0) events = new Set(parsed);
    } catch (err) {
      console.error("Invalid IMUI data-events JSON", canvas, err);
      return this.fail(canvas, "invalid events");
    }

    const record = {
      islandId,
      moduleName,
      module,
      canvas,
      ctx: canvas.getContext("2d"),
      events,
      commandURL: canvas.dataset.commandUrl || "",
      model: null,
      frame: 0,
      dpr: 1,
      resizeObserver: null,
      cleanup: [],
    };
    if (!record.ctx) return this.fail(canvas, "2d context unavailable");

    const api = this.api(record);
    record.model = module.init ? module.init(api, props) : {};
    this.instances.set(islandId, record);
    this.nodes.set(canvas, islandId);
    this.wireInput(record);
    this.wireResize(record);
    this.resize(record);
    this.invalidate(record);
    this.emit("imui-mounted", { islandId, module: moduleName });
  }

  unmount(canvas) {
    const islandId = this.nodes.get(canvas);
    if (!islandId) return;
    const record = this.instances.get(islandId);
    if (!record) return;
    if (record.frame) cancelAnimationFrame(record.frame);
    record.resizeObserver?.disconnect();
    record.cleanup.forEach((fn) => fn());
    try {
      record.module.destroy?.(record.model, this.api(record));
    } finally {
      this.instances.delete(islandId);
      this.nodes.delete(canvas);
      this.emit("imui-unmounted", { islandId });
    }
  }

  api(record) {
    return {
      islandId: record.islandId,
      canvas: record.canvas,
      ctx: record.ctx,
      dpr: record.dpr,
      invalidate: () => this.invalidate(record),
      command: (command) => this.command(record, command),
      emit: (name, detail = {}) => this.emit(name, { islandId: record.islandId, ...detail }),
    };
  }

  invalidate(record) {
    if (record.frame) return;
    record.frame = requestAnimationFrame(() => {
      record.frame = 0;
      this.resize(record);
      record.ctx.save();
      record.ctx.setTransform(record.dpr, 0, 0, record.dpr, 0, 0);
      try {
        record.module.draw?.(record.model, this.api(record));
      } finally {
        record.ctx.restore();
      }
    });
  }

  resize(record) {
    const rect = record.canvas.getBoundingClientRect();
    const cssWidth = rect.width || record.canvas.width || 300;
    const cssHeight = rect.height || record.canvas.height || 150;
    const dpr = window.devicePixelRatio || 1;
    const width = Math.max(1, Math.round(cssWidth * dpr));
    const height = Math.max(1, Math.round(cssHeight * dpr));
    if (record.canvas.width !== width || record.canvas.height !== height) {
      record.canvas.width = width;
      record.canvas.height = height;
    }
    record.dpr = dpr;
  }

  wireResize(record) {
    if (!window.ResizeObserver) return;
    record.resizeObserver = new ResizeObserver(() => this.invalidate(record));
    record.resizeObserver.observe(record.canvas);
  }

  wireInput(record) {
    const events = ["pointerdown", "pointermove", "pointerup", "pointercancel", "wheel", "keydown", "keyup", "focus", "blur"];
    events.forEach((name) => {
      const handler = (event) => this.handleInput(record, event);
      record.canvas.addEventListener(name, handler, { passive: name !== "wheel" });
      record.cleanup.push(() => record.canvas.removeEventListener(name, handler));
    });
  }

  handleInput(record, event) {
    const input = this.normaliseInput(record, event);
    if (!input) return;
    if (event.type === "wheel") event.preventDefault();
    record.module.input?.(record.model, input, this.api(record));
  }

  normaliseInput(record, event) {
    const rect = record.canvas.getBoundingClientRect();
    const base = {
      type: event.type,
      shiftKey: !!event.shiftKey,
      altKey: !!event.altKey,
      ctrlKey: !!event.ctrlKey,
      metaKey: !!event.metaKey,
    };
    if (typeof PointerEvent !== "undefined" && event instanceof PointerEvent) {
      return {
        ...base,
        pointerId: event.pointerId,
        button: event.button,
        buttons: event.buttons,
        position: { x: event.clientX - rect.left, y: event.clientY - rect.top },
      };
    }
    if (typeof WheelEvent !== "undefined" && event instanceof WheelEvent) {
      return {
        ...base,
        delta: { x: event.deltaX, y: event.deltaY, z: event.deltaZ, mode: event.deltaMode },
        position: { x: event.clientX - rect.left, y: event.clientY - rect.top },
      };
    }
    if (typeof KeyboardEvent !== "undefined" && event instanceof KeyboardEvent) {
      return { ...base, key: event.key, code: event.code, repeat: event.repeat };
    }
    return base;
  }

  async command(record, command) {
    this.emit("imui-command", { islandId: record.islandId, command });
    if (!record.commandURL) return;
    const response = await fetch(record.commandURL, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ islandId: record.islandId, command }),
    });
    this.emit("imui-command-result", { islandId: record.islandId, ok: response.ok, status: response.status });
    if (!response.ok) throw new Error(`IMUI command failed: ${response.status}`);
  }

  deliverSSE(name, data) {
    if (!name) return;
    for (const record of this.instances.values()) {
      if (record.events && !record.events.has(name)) continue;
      record.module.event?.(record.model, { name, data }, this.api(record));
    }
  }

  connectSources() {
    this.usingBroker = this.usingBroker || !!window.GoHTMXElmBroker;
    if (!this.active || this.usingBroker) return;
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
    if (!this.active || this.usingBroker || s.source) return;
    const source = new EventSource(s.url);
    s.source = source;
    s.names.forEach((name) => {
      source.addEventListener(name, (event) => this.forwardSSE(name, event));
    });
    source.onopen = () => this.emit("imui-source-open", { url: s.url });
    source.onerror = () => {
      if (!this.active || source !== s.source) return;
      this.emit("imui-source-error", { url: s.url });
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
    this.deliverSSE(name, data);
    this.emit("imui-sse", { event: name, data });
  }

  closeSources() {
    this.sources.forEach((s) => {
      if (s.retry) clearTimeout(s.retry);
      s.retry = null;
      if (s.source) s.source.close();
      s.source = null;
    });
  }

  pause() {
    this.active = false;
    this.closeSources();
  }

  resume() {
    if (this.active) return;
    this.active = true;
    this.connectSources();
    this.mountAll(document);
  }

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
    if (node.matches?.("[data-gohtmxelm-imui-module]")) this.unmount(node);
    node.querySelectorAll?.("[data-gohtmxelm-imui-module]").forEach((child) => this.unmount(child));
  }

  emit(name, detail) {
    document.dispatchEvent(new CustomEvent(`gohtmxelm:${name}`, { detail }));
  }

  fail(node, reason) {
    node.dataset.gohtmxelmImuiMountFailed = reason;
    this.failedNodes.add(node);
    this.emit("imui-mount-failed", { reason });
  }

  log(message) {
    if (this.config.debug) console.debug(`[gohtmxelm-imui] ${message}`);
  }
}

function imuiConfigFromScript() {
  const script = document.currentScript;
  let sources = [];
  try {
    sources = JSON.parse(script?.dataset.sources || "[]");
  } catch (err) {
    console.error("imui: invalid data-sources JSON", err);
  }
  return { sources, debug: script?.dataset.debug === "true" };
}

window.GoHTMXElmIMUI = new GoHTMXElmIMUIRuntime(imuiConfigFromScript());

document.addEventListener("DOMContentLoaded", () => {
  window.GoHTMXElmIMUI.mountAll(document);
});

window.addEventListener("pagehide", () => {
  window.GoHTMXElmIMUI.pause();
});

window.addEventListener("pageshow", (event) => {
  if (event.persisted) {
    window.location.reload();
    return;
  }
  window.GoHTMXElmIMUI.resume();
});

if (document.body) {
  document.body.addEventListener("htmx:afterSettle", () => {
    window.GoHTMXElmIMUI.mountAll(document);
  });
}
