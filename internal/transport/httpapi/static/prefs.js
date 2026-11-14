/* AIOS v2: browser-stored appearance preferences. */
(function () {
  "use strict";
  const KEY = "aios_prefs";
  const DEFAULTS = { theme: "dark", accent: "#4f8ef7", fontSize: 14, density: "default", motion: true, codeFontSize: 12.5, chatWidth: 900 };
  function get() {
    try { return Object.assign({}, DEFAULTS, JSON.parse(localStorage.getItem(KEY) || "{}")); }
    catch (e) { return Object.assign({}, DEFAULTS); }
  }
  function apply(p) {
    p = p || get();
    const root = document.documentElement;
    root.dataset.theme = p.theme === "system"
      ? (window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark") : p.theme;
    root.dataset.density = p.density;
    root.style.setProperty("--accent", p.accent);
    root.style.setProperty("--fs", p.fontSize + "px");
    root.style.setProperty("--code-fs", p.codeFontSize + "px");
    root.style.setProperty("--chat-max", p.chatWidth + "px");
    document.body.classList.toggle("reduce-motion", !p.motion);
  }
  function set(patch) {
    const p = Object.assign(get(), patch);
    localStorage.setItem(KEY, JSON.stringify(p));
    apply(p);
    return p;
  }
  window.AIOSPrefs = { get, set, apply, DEFAULTS, KEY };
  if (document.body) apply();
})();
