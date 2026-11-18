/* AIOS dashboard core: auth, API client, shell, routing. */
(function () {
  "use strict";
  const API = "/api/v1";
  const $ = (sel, el) => (el || document).querySelector(sel);
  const $$ = (sel, el) => Array.from((el || document).querySelectorAll(sel));
  const state = {
    token: localStorage.getItem("aios_token") || "",
    refresh: localStorage.getItem("aios_refresh") || "",
    user: null, perms: [], view: "overview",
    sessions: [], currentSession: null, messages: [], personalities: [],
  };
  function esc(v) {
    return String(v == null ? "" : v).replace(/[&<>"']/g, (c) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
    }[c]));
  }
  function fmtTime(ts) {
    if (!ts) return "—";
    const d = new Date(ts);
    return isNaN(d) ? esc(ts) : d.toLocaleString();
  }
  function ago(ts) {
    if (!ts) return "never";
    const s = Math.max(0, Math.floor((Date.now() - new Date(ts).getTime()) / 1000));
    if (s < 60) return s + "s ago";
    if (s < 3600) return Math.floor(s / 60) + "m ago";
    if (s < 86400) return Math.floor(s / 3600) + "h ago";
    return Math.floor(s / 86400) + "d ago";
  }
  function toast(msg, type) {
    const box = $("#toasts"); if (!box) return;
    const el = document.createElement("div");
    el.className = "item " + (type || "");
    el.textContent = msg;
    box.appendChild(el);
    setTimeout(() => el.remove(), 4500);
  }
  function setContent(html) { const m = $("#content"); if (m) m.innerHTML = html; }
  function modal(html) {
    closeModal();
    const wrap = document.createElement("div");
    wrap.className = "modal-backdrop"; wrap.id = "modal";
    wrap.innerHTML = '<div class="modal">' + html + "</div>";
    wrap.addEventListener("click", (e) => { if (e.target === wrap) closeModal(); });
    document.body.appendChild(wrap);
    return wrap;
  }
  function closeModal() { const m = $("#modal"); if (m) m.remove(); }
  function saveTokens(tokens) {
    state.token = tokens.access_token;
    state.refresh = tokens.refresh_token || state.refresh;
    localStorage.setItem("aios_token", state.token);
    localStorage.setItem("aios_refresh", state.refresh);
  }
  function clearTokens() {
    state.token = ""; state.refresh = "";
    localStorage.removeItem("aios_token");
    localStorage.removeItem("aios_refresh");
  }
  function can(perm) {
    return state.perms.indexOf("admin") >= 0 || state.perms.indexOf(perm) >= 0;
  }
  async function rawFetch(path, opts) {
    opts = opts || {};
    const headers = Object.assign({ "Content-Type": "application/json" }, opts.headers || {});
    if (opts.auth !== false && state.token) headers.Authorization = "Bearer " + state.token;
    return fetch(API + path, {
      method: opts.method || "GET", headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    });
  }
  async function refreshTokens() {
    if (!state.refresh) return false;
    const res = await fetch(API + "/auth/refresh", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: state.refresh }),
    });
    if (!res.ok) return false;
    saveTokens((await res.json()).tokens);
    return true;
  }
  async function api(path, opts) {
    opts = opts || {};
    let res = await rawFetch(path, opts);
    if (res.status === 401 && state.refresh && !opts._retried) {
      if (await refreshTokens()) { opts._retried = true; res = await rawFetch(path, opts); }
    }
    if (res.status === 204) return null;
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (e) { data = { error: { message: text } }; }
    if (!res.ok) {
      const err = new Error((data && data.error && data.error.message) || ("HTTP " + res.status));
      err.status = res.status;
      throw err;
    }
    return data;
  }
  let authMode = "login";
  function renderLogin(mode) {
    authMode = mode || "login";
    const reg = authMode === "register";
    document.body.innerHTML =
      '<div class="login-wrap"><div class="login-card">' +
      '<div class="brand"><div class="logo">A</div><div><h1>AIOS</h1><p>Personal AI Operating System</p></div></div>' +
      '<form id="login-form">' +
      (reg ? '<div class="field"><label>نام نمایشی</label><input class="input" id="name" autocomplete="name" placeholder="نام شما"></div>' : '') +
      '<div class="field"><label>ایمیل</label><input class="input" id="email" type="email" autocomplete="username" value="' + (reg ? "" : "admin@aios.local") + '" required></div>' +
      '<div class="field"><label>رمز عبور</label><input class="input" id="password" type="password" autocomplete="' + (reg ? "new-password" : "current-password") + '" required></div>' +
      '<button class="btn block" id="login-btn" type="submit">' + (reg ? "ساخت حساب" : "ورود") + '</button>' +
      '<p class="small muted" style="margin-top:14px">' + (reg ? "حساب داری؟" : "حساب نداری؟") +
      ' <a href="#" id="auth-toggle">' + (reg ? "وارد شو" : "ثبت‌نام کن") + '</a></p>' +
      '<p class="small muted">داده‌هایت روی همین سرور می‌ماند.</p>' +
      '</form></div></div><div id="toasts" class="toast"></div>';
    $("#login-form").addEventListener("submit", login);
    $("#auth-toggle").addEventListener("click", (e) => { e.preventDefault(); renderLogin(reg ? "login" : "register"); });
  }
  async function login(ev) {
    ev.preventDefault();
    const btn = $("#login-btn");
    btn.disabled = true;
    try {
      const reg = authMode === "register";
      const res = await fetch(API + (reg ? "/auth/register" : "/auth/login"), {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: $("#email").value.trim(), password: $("#password").value, display_name: $("#name") ? $("#name").value.trim() : "" }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error((data.error && data.error.message) || "Login failed");
      saveTokens(data.tokens);
      state.user = data.user;
      state.perms = data.permissions || [];
      await shell();
    } catch (e) {
      toast(e.message || "Login failed", "bad");
    } finally { btn.disabled = false; }
  }

  const NAV = [
    { group: "Workspace", items: [
      ["overview", "Overview", "◎"], ["chat", "Chat", "✎"], ["sessions", "Sessions", "▤"],
      ["myai", "هوش مصنوعی من", "⚡"], ["memory", "Memory", "❖"], ["documents", "Documents", "❐"], ["devices", "Devices", "◈"],
    ]},
    { group: "Intelligence", items: [
      ["agents", "Skills & Agents", "✦"], ["models", "Model Router", "⇄"], ["usage", "Usage & Cost", "◷"],
    ]},
    { group: "Platform", items: [
      ["plugins", "Plugins", "⧉"], ["queue", "Task Queue", "≣"], ["settings", "Settings", "⚙"],
    ]},
    { group: "Admin", items: [
      ["users", "Users", "☰"], ["logs", "Audit Log", "▦"], ["system", "System", "⌁"], ["help", "راهنما", "؟"],
    ]},
  ];
  function itemVisible(id) {
    switch (id) {
      case "users": return can("users:read");
      case "logs": return can("logs:read");
      case "settings": return can("settings:read");
      case "plugins": return can("plugins:read");
      case "models": return can("models:read");
      case "queue": return can("queue:read");
      case "memory": return can("memory:read");
      case "devices": return can("devices:read");
      default: return true;
    }
  }
  async function refreshModelChip() {
    const label = $("#model-chip-label"), chip = $("#model-chip");
    if (!label) return;
    try {
      const sel = localStorage.getItem("aios_chat_model") || "";
      if (sel) {
        const parts = sel.split("::");
        if (parts[0] === "sys") parts.shift();
        label.textContent = parts.join(" · ");
        chip.classList.remove("bad");
        return;
      }
      const d = await api("/me/models");
      const p = (d.items || [])[0];
      if (!p) { label.textContent = "بدون مدل"; chip.classList.add("bad"); return; }
      label.textContent = p.provider + " · " + (p.default_model || (p.models || [])[0] || "");
      chip.classList.remove("bad");
    } catch (e) {}
  }

  function renderShell() {
    const user = state.user || { display_name: "User", email: "", role: "user" };
    const initials = (user.display_name || user.email || "U").slice(0, 1).toUpperCase();
    let nav = "";
    NAV.forEach((group) => {
      const items = group.items.filter((i) => itemVisible(i[0]));
      if (!items.length) return;
      nav += '<div class="nav-label">' + group.group + "</div>";
      items.forEach((i) => {
        nav += '<div class="nav-item" data-view="' + i[0] + '"><span class="ico">' + i[2] +
          "</span><span>" + i[1] + "</span></div>";
      });
    });
    document.body.innerHTML =
      '<div class="shell"><aside class="sidebar">' +
      '<div class="brand"><div class="logo">A</div><div><h1>AIOS</h1><p>v1.0 · local-first</p></div></div>' +
      nav + '<div class="spacer"></div>' +
      '<div class="user-chip"><div class="avatar">' + esc(initials) + "</div>" +
      '<div style="min-width:0"><div class="name">' + esc(user.display_name || "User") + "</div>" +
      '<div class="role">' + esc(user.role || "") + "</div></div></div>" +
      '<button class="btn ghost sm" id="logout" style="margin-top:8px">Sign out</button>' +
      '</aside><div class="main">' +
      '<header class="topbar"><h2 id="view-title">Overview</h2><div class="spacer"></div>' +
      '<button class="model-chip" id="model-chip" title="Active model"><span class="dot"></span><span id="model-chip-label">model</span></button>' +
      '<button class="icon-btn" id="help-btn" title="راهنما">؟</button>' +
      '<button class="icon-btn" id="theme-btn" title="Toggle theme">◐</button>' +
      '<span class="badge"><span class="dot"></span> connected</span>' +
      '<span class="badge" id="clock"></span></header>' +
      '<div class="content" id="content"><div class="empty">Loading…</div></div>' +
      "</div></div>";
    $$(".nav-item").forEach((el) => el.addEventListener("click", () => navigate(el.dataset.view)));
    $("#logout").addEventListener("click", logout);
    if ($("#help-btn")) $("#help-btn").addEventListener("click", () => navigate("help"));
    refreshModelChip();
    setInterval(() => {
      const c = $("#clock");
      if (c) c.textContent = new Date().toLocaleTimeString();
    }, 1000);
  }
  const TITLES = {
    overview: "Overview", chat: "Chat", myai: "هوش مصنوعی من", help: "راهنما", character: "Characters", sessions: "Sessions", memory: "Memory", documents: "Documents", devices: "Devices",
    agents: "Skills & Agents", models: "Model Router", usage: "Usage & Cost", plugins: "Plugins",
    queue: "Task Queue", settings: "Settings", users: "Users", logs: "Audit Log", system: "System",
  };
  function navigate(view) {
    state.view = view;
    $$(".nav-item").forEach((el) => el.classList.toggle("active", el.dataset.view === view));
    const t = $("#view-title");
    if (t) t.textContent = TITLES[view] || view;
    const fn = window.AIOSViews && window.AIOSViews[view];
    if (!fn) { setContent('<div class="empty">This view is not available.</div>'); return; }
    setContent('<div class="empty">Loading…</div>');
    return Promise.resolve(fn()).catch((e) => {
      setContent('<div class="empty">' + esc(e.message) + "</div>");
      toast(e.message, "bad");
    });
  }
  async function shell() {
    if (!state.user) {
      const me = await api("/auth/me");
      state.user = me.user;
      state.perms = me.permissions || [];
    }
    renderShell();
    navigate("overview");
    if (!localStorage.getItem("aios_onboarded")) {
      localStorage.setItem("aios_onboarded", "1");
      setTimeout(() => { if (window.AIOSViews && window.AIOSViews.help) navigate("help"); }, 400);
    }
  }
  async function logout() {
    try { await api("/auth/logout", { method: "POST", body: { refresh_token: state.refresh } }); } catch (e) {}
    clearTokens();
    state.user = null;
    renderLogin();
  }
  async function boot() {
    if (!state.token) return renderLogin();
    try { await shell(); } catch (e) { clearTokens(); renderLogin(); }
  }
  window.AIOS = {
    API, state, $, $$, esc, fmtTime, ago, toast, setContent, modal, closeModal,
    api, can, navigate, boot, renderShell, renderLogin, logout, saveTokens,
  };
  window.addEventListener("load", () => window.AIOS.boot());
})();

