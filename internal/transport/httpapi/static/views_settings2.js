/* AIOS v2: settings screen (appearance, account, data). */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, setContent, state } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});
  const P = () => window.AIOSPrefs;
  const ACCENTS = ["#4f8ef7", "#8b5cf6", "#10b981", "#f59e0b", "#ef4444", "#06b6d4"];

  function seg(id, value, options) {
    return '<div class="seg" data-seg="' + id + '">' + options.map((o) =>
      '<button data-v="' + o + '" class="' + (String(value) === o ? "active" : "") + '">' + o + "</button>").join("") + "</div>";
  }

  function appearance(p) {
    return '<div class="card"><div class="section-title" style="margin-top:0">Appearance</div>' +
      '<div class="pref-row"><div><div class="lbl">Theme</div><div class="sub">Dark, light or follow the OS</div></div>' +
      seg("theme", p.theme, ["dark", "light", "system"]) + "</div>" +
      '<div class="pref-row"><div><div class="lbl">Accent</div><div class="sub">Buttons and focus rings</div></div>' +
      '<div class="swatches" id="accent-sw">' + ACCENTS.map((c) =>
        '<span class="swatch' + (p.accent === c ? " sel" : "") + '" data-c="' + c + '" style="background:' + c + '"></span>').join("") + "</div></div>" +
      '<div class="pref-row"><div><div class="lbl">Font size</div><div class="sub">' + p.fontSize + 'px</div></div>' +
      '<input type="range" min="12" max="18" value="' + p.fontSize + '" id="fs-range" style="max-width:180px;accent-color:var(--accent)"></div>' +
      '<div class="pref-row"><div><div class="lbl">Density</div><div class="sub">Spacing of cards</div></div>' +
      seg("density", p.density, ["compact", "default", "comfy"]) + "</div>" +
      '<div class="pref-row"><div><div class="lbl">Animations</div><div class="sub">Disable on slow devices</div></div>' +
      '<label class="switch"><input type="checkbox" id="motion"' + (p.motion ? " checked" : "") + "> enabled</label></div></div>";
  }

  function account() {
    const u = state.user || {};
    return '<div class="card"><div class="section-title" style="margin-top:0">Account</div>' +
      '<div class="pref-row"><div><div class="lbl">' + esc(u.display_name || "User") + '</div><div class="sub">' + esc(u.email || "") + "</div></div>" +
      '<span class="pill">' + esc(u.role || "") + "</span></div>" +
      '<div class="pref-row"><div><div class="lbl">Sign out</div><div class="sub">End this browser session</div></div>' +
      '<button class="btn sm ghost" id="st-logout">Sign out</button></div></div>';
  }

  function dataTab() {
    return '<div class="card"><div class="section-title" style="margin-top:0">Data</div>' +
      '<div class="pref-row"><div><div class="lbl">Export conversations</div><div class="sub">Session list as JSON</div></div>' +
      '<button class="btn sm secondary" id="st-export">Export</button></div>' +
      '<div class="pref-row"><div><div class="lbl">Reset appearance</div><div class="sub">Restore defaults</div></div>' +
      '<button class="btn sm ghost" id="st-reset">Reset</button></div></div>';
  }

  V.settings = async function () {
    setContent('<div class="page-head"><h3>Settings</h3><span class="muted small">Preferences stay in this browser</span></div>' +
      '<div class="tabs" id="st-tabs"><button class="active" data-t="appearance">Appearance</button>' +
      '<button data-t="account">Account</button><button data-t="data">Data</button></div>' +
      '<div id="st-body">' + appearance(P().get()) + "</div>");
    const body = $("#st-body");
    function show(tab) {
      $$("#st-tabs button").forEach((b) => b.classList.toggle("active", b.dataset.t === tab));
      body.innerHTML = tab === "account" ? account() : tab === "data" ? dataTab() : appearance(P().get());
      bind(tab);
    }
    $$("#st-tabs button").forEach((b) => b.addEventListener("click", () => show(b.dataset.t)));

    function bind(tab) {
      if (tab === "appearance") {
        $$("[data-seg='theme'] button").forEach((b) => b.addEventListener("click", () => { P().set({ theme: b.dataset.v }); show("appearance"); }));
        $$("[data-seg='density'] button").forEach((b) => b.addEventListener("click", () => { P().set({ density: b.dataset.v }); show("appearance"); }));
        $$("#accent-sw .swatch").forEach((s) => s.addEventListener("click", () => { P().set({ accent: s.dataset.c }); show("appearance"); }));
        $("#fs-range").addEventListener("input", (e) => P().set({ fontSize: Number(e.target.value) }));
        $("#motion").addEventListener("change", (e) => P().set({ motion: e.target.checked }));
      } else if (tab === "account") {
        $("#st-logout").addEventListener("click", () => A.logout());
      } else if (tab === "data") {
        $("#st-reset").addEventListener("click", () => {
          localStorage.removeItem(P().KEY); P().apply();
          toast("Appearance reset", "ok"); show("appearance");
        });
        $("#st-export").addEventListener("click", async () => {
          try {
            const sessions = await api("/sessions?limit=200");
            const blob = new Blob([JSON.stringify(sessions, null, 2)], { type: "application/json" });
            const a = document.createElement("a");
            a.href = URL.createObjectURL(blob);
            a.download = "aios-sessions-" + new Date().toISOString().slice(0, 10) + ".json";
            a.click(); URL.revokeObjectURL(a.href);
            toast("Export ready", "ok");
          } catch (e) { toast(e.message, "bad"); }
        });
      }
    }
    bind("appearance");
  };
})();
