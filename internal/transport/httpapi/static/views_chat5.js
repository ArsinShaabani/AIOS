/* AIOS v2 chat (part 2b): the chat screen, character/model pickers. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, state, can, setContent } = A;
  const V = window.AIOSViews, X = V._chat2, C = X.state;

  async function loadPersonalities() {
    try {
      const d = await api("/personalities");
      return (d.items || []).filter((p) => p.is_preset || !p.owner_id || p.owner_id === (state.user && state.user.id));
    } catch (e) { return []; }
  }

  async function loadModels() {
    const out = { items: [], current: "" };
    if (!can("models:read")) return out;
    try {
      const d = await api("/admin/providers");
      (d.items || []).forEach((p) => {
        if (!p.enabled) return;
        const models = (p.models && p.models.length ? p.models : [p.default_model]).filter(Boolean);
        models.forEach((m) => out.items.push({ value: p.name + "::" + m, label: p.name + " · " + m }));
        if (p.default) out.current = p.name + "::" + (p.default_model || models[0] || "");
      });
    } catch (e) {
      try { (await api("/models")).providers.forEach((p) => out.items.push({ value: p.name, label: p.name })); } catch (e2) {}
    }
    return out;
  }

  V.chat = async function () {
    const pers = await loadPersonalities();
    state.personalities = pers;
    C.personality = localStorage.getItem("aios_default_character") || (pers[0] && pers[0].id) || "";
    const models = await loadModels();
    await X.loadSessions();
    C.session = null;
    setContent(
      '<div class="chat2"><aside class="side"><div class="new"><button class="btn block" id="c2-new">+ New chat</button></div><div id="c2-sessions"></div></aside>' +
      '<section class="pane"><div class="head">' +
      '<select id="c2-char" title="Character">' + pers.map((p) =>
        '<option value="' + esc(p.id) + '"' + (p.id === C.personality ? " selected" : "") + ">" + X.charEmoji(p) + " " + esc(p.name) + "</option>").join("") + "</select>" +
      '<select id="c2-model" title="Model">' + (models.items.length
        ? models.items.map((m) => '<option value="' + esc(m.value) + '"' + (m.value === models.current ? " selected" : "") + ">" + esc(m.label) + "</option>").join("")
        : "<option>configure a provider</option>") + "</select>" +
      '<span class="spacer"></span><span class="pill" id="c2-status">ready</span>' +
      '<button class="icon-btn" id="c2-voice" title="Voice">🎙</button></div>' +
      '<div class="msgs" id="c2-msgs"><div class="empty-chat"><div class="big">👋 Hi' +
      (state.user && state.user.display_name ? ", " + esc(state.user.display_name) : "") + "</div>" +
      '<div class="muted">Ask anything. Markdown, code blocks and tool runs are shown inline.</div><div class="suggest">' +
      ["Explain my devices", "Summarize my documents", "Write a Python script", "Plan my day"].map((t) =>
        '<button data-sug="' + esc(t) + '">' + esc(t) + "</button>").join("") + "</div></div></div>" +
      '<div class="composer2"><textarea id="c2-input" rows="1" placeholder="Message AIOS…"></textarea>' +
      '<div><button class="btn" id="c2-send">Send</button><button class="btn ghost stop-btn hidden" id="c2-stop">Stop</button></div></div>' +
      '<div class="hint" style="padding:0 14px 10px">Enter to send · Shift+Enter for a new line · Esc stops generation</div>' +
      "</section></div>");

    $("#c2-new").addEventListener("click", () => {
      C.session = null;
      $("#c2-msgs").innerHTML = '<div class="empty-chat"><div class="big">New conversation</div><div class="muted">Say something to begin.</div></div>';
      X.loadSessions();
    });
    $("#c2-send").addEventListener("click", () => X.send($("#c2-input").value));
    $("#c2-stop").addEventListener("click", () => { if (C.abort) C.abort.abort(); });
    $("#c2-char").addEventListener("change", (e) => { C.personality = e.target.value; });
    $("#c2-model").addEventListener("change", async (e) => {
      const parts = e.target.value.split("::"), provider = parts.shift(), model = parts.join("::");
      try {
        await api("/admin/providers/default", { method: "POST", body: { provider, model } });
        toast("Default model updated", "ok");
      } catch (err) { toast(err.message, "bad"); }
    });
    const input = $("#c2-input");
    input.addEventListener("input", () => { input.style.height = "auto"; input.style.height = Math.min(180, input.scrollHeight) + "px"; });
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); X.send(input.value); }
      if (e.key === "Escape" && C.abort) C.abort.abort();
    });
    $$("[data-sug]").forEach((b) => b.addEventListener("click", () => { input.value = b.dataset.sug; X.send(b.dataset.sug); }));
    $("#c2-voice").addEventListener("click", () => {
      if (!window.AIOSVoice) return toast("Voice UI unavailable", "bad");
      window.AIOSVoice.toggle({
        onTranscript: (t) => X.send(t),
        onReply: (t) => X.msgNode("assistant", t),
        onSession: (id) => { C.session = id; },
        onError: (e) => toast(e, "bad"),
      });
    });
    input.focus();
  };
})();
