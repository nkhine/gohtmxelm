// demo-ui.js — the demo's own behaviour layered on top of the generic broker.
//
// The reusable broker (pkg/runtime/gohtmxelm-broker.js) knows nothing about
// this app's store, optimistic locking, or teaching UI. It only emits
// `gohtmxelm:*` DOM events. Everything app-specific lives here:
//   • mirror Elm STATE_SET writes to the Go store with optimistic versioning
//   • track per-key versions + the global seq from the store SSE stream
//   • drive the HTMX store-refresh / stopwatch-controls re-renders
//   • render the activity log, SSE status pill, and row-flash animation

(function () {
  const storeVersions = new Map();
  let storeSeq = 0;

  // ── Route Elm state writes to the right endpoint ──────────────────────────
  // The host maps shared-state keys to server endpoints. "statementRange" is a
  // range-picker command (a JSON string), not a KV write.
  document.addEventListener("gohtmxelm:state-set", (e) => {
    const { key, value, source } = e.detail;
    if (key === "statementRange") return postRange(value);
    if (key === "latticeCommand") return postLatticeCommand(value, source);
    const body = typeof value === "string" ? value : JSON.stringify(value);
    const version = storeVersions.get(key) || 0;
    log("elm", "go", `STATE_SET ${key} from ${source}`);
    fetch("/api/store", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, value: body, source, version }),
    })
      .then((res) => {
        if (res.status === 409) {
          log("go", "broker", `409 conflict on "${key}" — SSE will correct`);
        }
      })
      .catch((err) => console.warn("store sync failed", err));
  });

  // The picker sends a JSON string ({"preset":"15m"} or {"from":...,"to":...}).
  function postRange(value) {
    let payload;
    try {
      payload = typeof value === "string" ? JSON.parse(value) : value;
    } catch (err) {
      return console.warn("invalid statementRange payload", value, err);
    }
    const desc = payload.relUnit ? `last ${payload.relValue} ${payload.relUnit}` : `${payload.from}..${payload.to}`;
    log("elm", "go", `range → ${desc}`);
    fetch("/api/statement/range", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }).catch((err) => console.warn("range post failed", err));
  }

  function postLatticeCommand(value, source) {
    let command;
    try {
      command = typeof value === "string" ? JSON.parse(value) : value;
    } catch (err) {
      return console.warn("invalid latticeCommand payload", value, err);
    }
    command.source = "elm";
    log("elm", "go", `lattice ${command.action || "command"} from ${source}`);
    fetch("/api/lattice/command", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ command }),
    }).catch((err) => console.warn("lattice command failed", err));
  }

  // ── React to forwarded SSE ────────────────────────────────────────────────
  document.addEventListener("gohtmxelm:sse", (e) => {
    const { event, data } = e.detail;
    if (event === "store-hydrate") return applyStore(data, false);
    if (event === "store-change") return applyStore(data, true);
    if (event === "stopwatch-state") return applyStopwatch(data);
    if (event === "statement-range-change") return applyStatementRange(data);
    if (event === "auth-presence") return applyAuthPresence(data);
    if (event === "lattice-state") return applyLattice(data);
  });

  function applyLattice(data) {
    if (!data || typeof data !== "object") return;
    log("sse", "htmx", `LATTICE seq=${data.seq} via=${data.lastSource || "?"}`);
    if (window.htmx) window.htmx.trigger(document.body, "lattice-change");
  }

  function applyAuthPresence(data) {
    const state = data && data.state ? data.state : "offline";
    const email = data && data.email ? ` ${data.email}` : "";
    log("sse", "htmx", `AUTH_PRESENCE ${state}${email}`);
    if (window.htmx) window.htmx.trigger(document.body, "auth-presence");
  }

  function applyStatementRange(data) {
    const label = data && data.label ? data.label : "range";
    const count = data && typeof data.count === "number" ? data.count : "?";
    log("sse", "htmx", `STATEMENT_RANGE ${label} (${count})`);
    if (window.htmx) window.htmx.trigger(document.body, "statement-range-change");
  }

  function applyStore(data, isDelta) {
    if (!data || typeof data !== "object") return;
    const { key, source, deleted, version, seq } = data;
    if (isDelta && seq !== undefined && seq <= storeSeq) return; // stale redelivery
    if (seq !== undefined) storeSeq = Math.max(storeSeq, seq);
    if (deleted) storeVersions.delete(key);
    else if (version !== undefined) storeVersions.set(key, version);

    if (isDelta) {
      const verb = deleted ? "STORE_DELETE" : "STORE_CHANGE";
      log("sse", "elm", `${verb} key="${key}" by=${source || "?"}`);
      flashStoreRow(key);
    } else {
      log("sse", "broker", `hydrate key="${key}"`);
    }

    const storeEl = document.getElementById("store-entries");
    if (storeEl && window.htmx) window.htmx.trigger(storeEl, "store-refresh");
  }

  function applyStopwatch(snap) {
    const running = !!(snap && snap.running);
    const laps = snap && Array.isArray(snap.laps) ? snap.laps : [];
    log("sse", "elm", `STOPWATCH_SNAPSHOT running=${running} laps=${laps.length}`);
    if (window.htmx) window.htmx.trigger(document.body, "stopwatch-state-change");
  }

  // ── Lifecycle + teaching UI ───────────────────────────────────────────────
  document.addEventListener("gohtmxelm:mounted", (e) => log("broker", e.detail.islandId, "mounted island"));
  document.addEventListener("gohtmxelm:source-open", () => setSseStatus(true));
  document.addEventListener("gohtmxelm:source-error", () => {
    setSseStatus(false);
    log("sse", "broker", "stream error — reconnecting in 3s");
  });
  document.addEventListener("gohtmxelm:htmx-swap", (e) =>
    log("elm", "htmx", `HTMX_SWAP → ${e.detail.url} into ${e.detail.selector}`)
  );
  document.addEventListener("gohtmxelm:htmx-after-swap", (e) =>
    log("htmx", "elm", `afterSwap → #${e.detail.targetId} from ${e.detail.url}`)
  );
  document.addEventListener("gohtmxelm:imui-command", (e) =>
    log("imui", "go", `canvas ${e.detail.command?.action || "command"}`)
  );

  // Datastar owns its own island; these only narrate its activity in the log.
  document.addEventListener("datastar-fetch", (e) => {
    const type = e.detail?.type || "unknown";
    const tag = e.detail?.el?.tagName?.toLowerCase() || "element";
    log("datastar", "go", `${type} from ${tag}`);
  });
  document.addEventListener("datastar-signal-patch", (e) => {
    const keys = Object.keys(e.detail || {}).join(", ") || "signals";
    log("datastar", "dom", `signal patch: ${keys}`);
  });

  // ── Interaction example extras ────────────────────────────────────────────
  // Generic overlay open/close/focus behaviour lives in the reusable
  // gohtmxelm-interactions runtime. The demo script only owns example-specific
  // notification actions.
  document.addEventListener("click", (e) => {
    const dismissToast = e.target.closest("[data-call-dismiss-toast]");
    if (dismissToast) {
      e.preventDefault();
      const target = document.querySelector(dismissToast.dataset.callDismissToast);
      if (target) target.innerHTML = "";
      return;
    }

    const action = e.target.closest("[data-call-action]");
    if (action) {
      e.preventDefault();
      runCallableAction(action.dataset.callAction);
    }
  });

  function setCallStatus(selector, text, good = true) {
    window.GoHTMXElmInteractions?.setStatus(selector, text, good);
  }

  let errorCount = 0;
  let liveState = 0;
  let uploadsOnline = true;

  function runCallableAction(action) {
    if (action === "error-banner") return showErrorBanner();
    if (action === "live-status") return cycleLiveStatus();
    if (action === "upload-broadcast") return toggleUploads();
  }

  function showErrorBanner() {
    errorCount += 1;
    const layer = document.getElementById("toast-layer");
    if (!layer) return;
    const toast = document.createElement("div");
    toast.className = "call-toast error";
    toast.innerHTML = `<div class="call-toast-head"><p>Upload ${errorCount} failed. Retrying is safe.</p><button type="button">x</button></div>`;
    toast.querySelector("button").addEventListener("click", () => toast.remove());
    layer.append(toast);
    setCallStatus("#call-status-error", `stacked error #${errorCount}`, false);
    setTimeout(() => toast.remove(), 3200);
  }

  function cycleLiveStatus() {
    const states = [
      ["Connecting", "Opening SSE channel"],
      ["Syncing", "Receiving server patches"],
      ["Online", "All views are current"],
    ];
    const [title, body] = states[liveState % states.length];
    liveState += 1;
    upsertToast("call-live-status-toast", "status", `${title}: ${body}`);
    setCallStatus("#call-status-live", title.toLowerCase());
  }

  function toggleUploads() {
    uploadsOnline = !uploadsOnline;
    const layer = document.getElementById("toast-layer");
    if (!layer) return;
    let box = document.getElementById("call-upload-box");
    if (!box) {
      box = document.createElement("div");
      box.id = "call-upload-box";
      box.className = "call-toast";
      box.innerHTML =
        `<div class="call-upload-row">` +
        ["statement.csv", "receipts.zip", "audit-log.json"]
          .map((name) => `<div class="call-upload-pill"><span>${name}</span><b>online</b></div>`)
          .join("") +
        `</div>`;
      layer.append(box);
    }
    box.querySelectorAll(".call-upload-pill").forEach((pill) => {
      pill.classList.toggle("offline", !uploadsOnline);
      pill.querySelector("b").textContent = uploadsOnline ? "online" : "offline";
    });
    setCallStatus("#call-status-upload", uploadsOnline ? "broadcast online" : "broadcast offline", uploadsOnline);
  }

  function upsertToast(id, variant, message) {
    const layer = document.getElementById("toast-layer");
    if (!layer) return;
    let toast = document.getElementById(id);
    if (!toast) {
      toast = document.createElement("div");
      toast.id = id;
      layer.append(toast);
    }
    toast.className = `call-toast ${variant}`;
    toast.innerHTML = `<div class="call-toast-head"><p>${escapeHTML(message)}</p><button type="button">x</button></div>`;
    toast.querySelector("button").addEventListener("click", () => toast.remove());
  }

  function escapeHTML(value) {
    return String(value).replace(/[&<>"']/g, (ch) => (
      { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]
    ));
  }

  function registerFiveWayIMUI() {
    if (!window.GoHTMXElmIMUI) {
      setTimeout(registerFiveWayIMUI, 50);
      return;
    }
    window.GoHTMXElmIMUI.register("FiveWayLattice", {
      init() {
        return { state: null, hover: null };
      },
      event(model, event, api) {
        if (event.name !== "lattice-state") return;
        model.state = event.data;
        api.invalidate();
      },
      input(model, input, api) {
        if (!model.state) return;
        if (input.type === "pointermove") {
          model.hover = nearestNode(model.state, input.position, api.canvas);
          api.invalidate();
        }
        if (input.type === "pointerup") {
          const hit = nearestNode(model.state, input.position, api.canvas);
          if (hit) api.command({ action: "select-node", node: hit.id, source: "imui" });
        }
        if (input.type === "keydown" && input.key === "a") {
          api.command({ action: "add-node", source: "imui" });
        }
        if (input.type === "keydown" && ["h", "j", "k", "l"].includes(input.key)) {
          api.command({ action: "select-node", node: input.key, source: "imui" });
        }
      },
      draw(model, api) {
        drawLattice(model, api);
      },
    });
  }

  function nearestNode(state, position, canvas) {
    const rect = canvas.getBoundingClientRect();
    const radius = 28;
    let best = null;
    let bestDist = radius * radius;
    for (const node of state.nodes || []) {
      const x = node.x * rect.width;
      const y = node.y * rect.height;
      const dx = position.x - x;
      const dy = position.y - y;
      const d = dx * dx + dy * dy;
      if (d <= bestDist) {
        bestDist = d;
        best = node;
      }
    }
    return best;
  }

  function drawLattice(model, api) {
    const { ctx, canvas, dpr } = api;
    const w = canvas.width / dpr;
    const h = canvas.height / dpr;
    ctx.clearRect(0, 0, w, h);
    ctx.fillStyle = "#f8fafc";
    ctx.fillRect(0, 0, w, h);

    if (!model.state) {
      ctx.fillStyle = "#64748b";
      ctx.font = "14px system-ui, sans-serif";
      ctx.fillText("Waiting for lattice-state over SSE", 24, 36);
      return;
    }

    const nodes = new Map((model.state.nodes || []).map((n) => [n.id, n]));
    ctx.lineWidth = 3;
    ctx.strokeStyle = "#94a3b8";
    for (const edge of model.state.edges || []) {
      const from = nodes.get(edge.from);
      const to = nodes.get(edge.to);
      if (!from || !to) continue;
      ctx.beginPath();
      ctx.moveTo(from.x * w, from.y * h);
      ctx.lineTo(to.x * w, to.y * h);
      ctx.stroke();
    }

    for (const node of model.state.nodes || []) {
      const x = node.x * w;
      const y = node.y * h;
      const hot = model.hover && model.hover.id === node.id;
      ctx.beginPath();
      ctx.arc(x, y, hot ? 25 : 21, 0, Math.PI * 2);
      ctx.fillStyle = node.selected ? "#0ea5e9" : "#ffffff";
      ctx.strokeStyle = hot ? "#155e75" : "#0ea5e9";
      ctx.lineWidth = hot ? 4 : 2;
      ctx.fill();
      ctx.stroke();
      ctx.fillStyle = node.selected ? "#ffffff" : "#0f172a";
      ctx.font = "700 12px system-ui, sans-serif";
      ctx.textAlign = "center";
      ctx.textBaseline = "middle";
      ctx.fillText(node.label, x, y);
    }

    ctx.textAlign = "left";
    ctx.textBaseline = "alphabetic";
    ctx.fillStyle = "#475569";
    ctx.font = "12px system-ui, sans-serif";
    ctx.fillText(`IMUI canvas: h/j/k/l select, "a" adds. seq ${model.state.seq}`, 18, h - 18);
  }

  document.addEventListener("DOMContentLoaded", registerFiveWayIMUI);

  // ── Visual helpers ────────────────────────────────────────────────────────
  function setSseStatus(connected) {
    const el = document.getElementById("sse-status");
    const txt = document.getElementById("sse-status-text");
    if (!el || !txt) return;
    el.className = `sse-status ${connected ? "connected" : "disconnected"}`;
    txt.textContent = connected ? "SSE live" : "SSE disconnected";
  }

  function flashStoreRow(key) {
    // The store-refresh swap lands shortly after; wait for it to settle.
    setTimeout(() => {
      const row = document.querySelector(`[data-store-key="${CSS.escape(key)}"]`);
      if (!row) return;
      row.classList.remove("store-row-flash");
      void row.offsetWidth; // restart the animation if mid-flight
      row.classList.add("store-row-flash");
    }, 120);
  }

  const FROM_CLASS = {
    elm: "log-from-elm",
    htmx: "log-from-htmx",
    sse: "log-from-sse",
    go: "log-from-go",
    datastar: "log-from-datastar",
    broker: "log-from-sse",
  };

  function log(from, to, description) {
    const container = document.getElementById("activity-entries");
    if (!container) return;
    const placeholder = container.querySelector(".log-entry:only-child");
    if (placeholder && placeholder.querySelector(".log-time")?.textContent === "--:--:--") {
      placeholder.remove();
    }
    const now = new Date();
    const time = [now.getHours(), now.getMinutes(), now.getSeconds()]
      .map((n) => String(n).padStart(2, "0"))
      .join(":");
    const entry = document.createElement("div");
    entry.className = "log-entry";
    entry.innerHTML =
      `<span class="log-time">${time}</span>` +
      `<span class="log-msg"><span class="${FROM_CLASS[from] || ""}">[${from}→${to}]</span> ${description}</span>`;
    container.prepend(entry);
    while (container.children.length > 50) container.removeChild(container.lastChild);
  }
})();
