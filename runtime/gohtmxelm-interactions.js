(function () {
  const ROOT_SELECTOR = "[data-gohtmxelm-interactions-root], #gohtmxelm-interactions, #call-layer";
  const BACKDROP_SELECTOR = "[data-gohtmxelm-backdrop], [data-call-backdrop]";
  const HOST_SELECTOR = "[data-gohtmxelm-status-target], [data-status-target]";

  function root() {
    const el = document.querySelector(ROOT_SELECTOR);
    if (!el) return null;
    Object.assign(el.style, {
      position: "fixed",
      inset: "0",
      pointerEvents: "none",
      zIndex: "80",
    });
    return el;
  }

  function value(el, preferred, fallback) {
    return el?.dataset?.[preferred] || el?.dataset?.[fallback] || "";
  }

  function open(url, statusTarget = "") {
    const target = root();
    if (!target || !window.htmx || !url) return;
    const sep = url.includes("?") ? "&" : "?";
    const finalURL = statusTarget && !url.includes("target=")
      ? `${url}${sep}target=${encodeURIComponent(statusTarget)}`
      : url;
    window.htmx.ajax("GET", finalURL, { target, swap: "beforeend" });
  }

  function close(from, result = "closed", ok = true) {
    const host = from.closest?.(HOST_SELECTOR) || from;
    const target = value(host, "gohtmxelmStatusTarget", "statusTarget");
    if (target) setStatus(target, result, ok);
    document.dispatchEvent(new CustomEvent("gohtmxelm:interaction-result", {
      detail: { target, result },
    }));
    const wrapper = from.closest?.("[data-gohtmxelm-fragment], .call-backdrop, .call-context-menu");
    if (wrapper) wrapper.remove();
  }

  function setStatus(selector, text, ok = true) {
    const el = document.querySelector(selector);
    if (!el) return;
    el.textContent = `-> ${text}`;
    el.classList.toggle("good", ok);
    el.classList.toggle("bad", !ok);
  }

  function top() {
    const items = Array.from(document.querySelectorAll(
      "[data-gohtmxelm-fragment], #call-layer .call-backdrop, #call-layer .call-context-menu"
    ));
    return items[items.length - 1] || null;
  }

  function init(rootNode = document) {
    const auto = rootNode.querySelector?.("[data-gohtmxelm-auto-resolve]:not([data-gohtmxelm-auto-started]), [data-auto-resolve]:not([data-gohtmxelm-auto-started])");
    if (auto) {
      auto.dataset.gohtmxelmAutoStarted = "true";
      const result = value(auto, "gohtmxelmAutoResolve", "autoResolve") || "external timeout resolved false";
      const delay = Number(auto.dataset.gohtmxelmAutoDelay || 3000);
      window.setTimeout(() => {
        if (document.body.contains(auto)) close(auto, result);
      }, delay);
    }

    const palette = rootNode.querySelector?.("[data-gohtmxelm-command-palette]:not([data-gohtmxelm-ready]), [data-command-palette]:not([data-gohtmxelm-ready])");
    if (palette) {
      palette.dataset.gohtmxelmReady = "true";
      const first = palette.querySelector("[data-gohtmxelm-command-item], [data-command-item]");
      if (first) first.classList.add("active");
      palette.querySelector("[data-gohtmxelm-command-search], [data-command-search]")?.focus();
    }

    const wizard = rootNode.querySelector?.("[data-gohtmxelm-wizard]:not([data-gohtmxelm-ready]), [data-call-wizard]:not([data-gohtmxelm-ready])");
    if (wizard) {
      wizard.dataset.gohtmxelmReady = "true";
      wireWizard(wizard);
      wizard.querySelector("[data-gohtmxelm-wizard-name], [data-wizard-name]")?.focus();
    }
  }

  function wireWizard(wizard) {
    const setStep = (next) => {
      const step = Math.max(0, Math.min(2, next));
      wizard.dataset.step = String(step);
      wizard.querySelectorAll("[data-gohtmxelm-wizard-step], [data-wizard-step]").forEach((el) => {
        const n = Number(el.dataset.gohtmxelmWizardStep || el.dataset.wizardStep);
        el.hidden = n !== step;
      });
      wizard.querySelectorAll(".call-wizard-bars span").forEach((bar, i) => {
        bar.classList.toggle("active", i <= step);
      });
      const back = wizard.querySelector("[data-gohtmxelm-wizard-back], [data-wizard-back]");
      const nextButton = wizard.querySelector("[data-gohtmxelm-wizard-next], [data-wizard-next]");
      const finish = wizard.querySelector("[data-gohtmxelm-wizard-finish], [data-wizard-finish]");
      if (back) back.hidden = step === 0;
      if (nextButton) nextButton.hidden = step === 2;
      if (finish) finish.hidden = step !== 2;
    };
    wizard.querySelector("[data-gohtmxelm-wizard-next], [data-wizard-next]")?.addEventListener("click", () => {
      setStep(Number(wizard.dataset.step || 0) + 1);
    });
    wizard.querySelector("[data-gohtmxelm-wizard-back], [data-wizard-back]")?.addEventListener("click", () => {
      setStep(Number(wizard.dataset.step || 0) - 1);
    });
    wizard.querySelector("[data-gohtmxelm-wizard-finish], [data-wizard-finish]")?.addEventListener("click", () => {
      const name = wizard.querySelector("[data-gohtmxelm-wizard-name], [data-wizard-name]")?.value.trim() || "anonymous";
      const plan = wizard.querySelector("[data-gohtmxelm-wizard-plan], [data-wizard-plan]")?.value || "free";
      close(wizard, `${name} (${plan})`);
    });
  }

  function moveCommandSelection(palette, delta) {
    const items = Array.from(palette.querySelectorAll("[data-gohtmxelm-command-item]:not([hidden]), [data-command-item]:not([hidden])"));
    if (!items.length) return;
    let idx = items.findIndex((item) => item.classList.contains("active"));
    if (idx < 0) idx = 0;
    items[idx].classList.remove("active");
    idx = (idx + delta + items.length) % items.length;
    items[idx].classList.add("active");
  }

  function filterCommands(palette, query) {
    if (!palette) return;
    const q = query.trim().toLowerCase();
    const items = Array.from(palette.querySelectorAll("[data-gohtmxelm-command-item], [data-command-item]"));
    items.forEach((item) => {
      item.hidden = q && !item.textContent.toLowerCase().includes(q);
      item.classList.remove("active");
    });
    const first = items.find((item) => !item.hidden);
    if (first) first.classList.add("active");
  }

  document.addEventListener("click", (e) => {
    const opener = e.target.closest("[data-gohtmxelm-open], [data-call-open]");
    if (opener) {
      e.preventDefault();
      const urlOrKind = value(opener, "gohtmxelmOpen", "callOpen");
      const status = value(opener, "gohtmxelmStatus", "status");
      const url = urlOrKind.startsWith("/") ? urlOrKind : `/api/callables/${encodeURIComponent(urlOrKind)}`;
      open(url, status);
      return;
    }

    const urlOpener = e.target.closest("[data-gohtmxelm-open-url], [data-call-open-url]");
    if (urlOpener) {
      e.preventDefault();
      open(value(urlOpener, "gohtmxelmOpenUrl", "callOpenUrl"));
      return;
    }

    const result = e.target.closest("[data-gohtmxelm-result], [data-call-result]");
    if (result) {
      e.preventDefault();
      close(result, value(result, "gohtmxelmResult", "callResult"));
      return;
    }

    const prompt = e.target.closest("[data-gohtmxelm-prompt-submit], [data-call-prompt-submit]");
    if (prompt) {
      e.preventDefault();
      const panel = prompt.closest(HOST_SELECTOR);
      const input = panel?.querySelector("[data-gohtmxelm-prompt-input], [data-call-prompt-input]");
      close(prompt, input && input.value.trim() ? `renamed ${input.value.trim()}` : "cancelled");
      return;
    }

    const asyncButton = e.target.closest("[data-gohtmxelm-async-result], [data-call-async-result]");
    if (asyncButton) {
      e.preventDefault();
      asyncButton.disabled = true;
      asyncButton.textContent = "Working...";
      window.setTimeout(() => close(asyncButton, value(asyncButton, "gohtmxelmAsyncResult", "callAsyncResult")), 850);
      return;
    }

    const settings = e.target.closest("[data-gohtmxelm-settings-save], [data-settings-save]");
    if (settings) {
      e.preventDefault();
      const panel = settings.closest(HOST_SELECTOR);
      const email = panel?.querySelector("[data-gohtmxelm-settings-email], [data-settings-email]")?.value || "daily";
      const theme = panel?.querySelector("[data-gohtmxelm-settings-theme], [data-settings-theme]")?.value || "system";
      close(settings, `saved ${email}/${theme}`);
    }
  });

  document.addEventListener("click", (e) => {
    if (e.target.matches(BACKDROP_SELECTOR)) close(e.target, "dismissed");
  });

  document.addEventListener("contextmenu", (e) => {
    const zone = e.target.closest("[data-gohtmxelm-context-zone], [data-call-context-zone]");
    if (!zone) return;
    e.preventDefault();
    const status = value(zone, "gohtmxelmStatus", "status") || "#call-status-context";
    open(`/api/callables/context-menu?x=${Math.round(e.clientX)}&y=${Math.round(e.clientY)}`, status);
  });

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      const item = top();
      if (item) close(item, "dismissed");
    }
    const palette = document.querySelector("[data-gohtmxelm-command-palette], [data-command-palette]");
    if (!palette) return;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      moveCommandSelection(palette, e.key === "ArrowDown" ? 1 : -1);
    }
    if (e.key === "Enter" && document.activeElement?.matches("[data-gohtmxelm-command-search], [data-command-search]")) {
      e.preventDefault();
      const active = palette.querySelector("[data-gohtmxelm-command-item].active, [data-command-item].active")
        || palette.querySelector("[data-gohtmxelm-command-item]:not([hidden]), [data-command-item]:not([hidden])");
      if (active) close(active, value(active, "gohtmxelmResult", "callResult"));
    }
  });

  document.addEventListener("input", (e) => {
    const search = e.target.closest("[data-gohtmxelm-command-search], [data-command-search]");
    if (search) filterCommands(search.closest("[data-gohtmxelm-command-palette], [data-command-palette]"), search.value);
  });

  document.addEventListener("htmx:afterSwap", (e) => {
    if (e.detail?.target?.matches?.(ROOT_SELECTOR)) init(e.detail.target);
  });

  window.GoHTMXElmInteractions = { open, close, setStatus, top, init };
  document.addEventListener("DOMContentLoaded", () => init(document));
})();
