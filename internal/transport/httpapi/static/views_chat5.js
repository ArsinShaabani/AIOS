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
      '<button class="icon-btn" id="c2-char-pick" title="Choose character">👤</button>' +
      '<button class="icon-btn" id="c2-attach" title="Attach file">📎</button>' +
      '<button class="icon-btn" id="c2-opts" title="Advanced model options">⚙</button>' +
      '<span class="spacer"></span><span class="pill" id="c2-status">ready</span>' +
      '<button class="icon-btn" id="c2-voice" title="Voice">🎙</button></div>' +
      '<div id="c2-files" class="chips"></div>' +
      '<div id="c2-opts-panel" class="opts hidden"><div class="opt"><label>Max tokens</label><input id="opt-max" type="number" min="64" max="200000" step="64" placeholder="provider default"></div>' +
      '<div class="opt"><label>Temperature</label><input id="opt-temp" type="number" min="0" max="2" step="0.05" placeholder="auto"></div>' +
      '<div class="opt"><label>Context window</label><input id="opt-ctx" type="number" min="1024" max="2000000" step="1024" placeholder="model default"></div>' +
      '<div class="opt"><label>Reasoning effort</label><select id="opt-effort"><option value="">auto</option><option value="low">low</option><option value="medium">medium</option><option value="high">high</option></select></div>' +
      '<div class="opt"><button class="btn ghost sm" id="opt-reset">Reset</button></div></div>' +
      '<input type="file" id="c2-file" multiple hidden accept="text/*,.md,.json,.csv,.py,.js,.ts,.go,.html,.css,.yml,.yaml,.sh,.sql,image/*">' +
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
    // ---- attachments -------------------------------------------------
    const fileInput = $("#c2-file");
    $("#c2-attach").addEventListener("click", () => fileInput.click());
    fileInput.addEventListener("change", () => addFiles(Array.from(fileInput.files || [])));
    const dropZone = $("#c2-msgs");
    ["dragover", "dragenter"].forEach((ev) => dropZone.addEventListener(ev, (e) => { e.preventDefault(); dropZone.classList.add("dragging"); }));
    ["dragleave", "drop"].forEach((ev) => dropZone.addEventListener(ev, () => dropZone.classList.remove("dragging")));
    dropZone.addEventListener("drop", (e) => { e.preventDefault(); addFiles(Array.from(e.dataTransfer.files || [])); });
    input.addEventListener("paste", (e) => {
      const items = Array.from((e.clipboardData && e.clipboardData.files) || []);
      if (items.length) { e.preventDefault(); addFiles(items); }
    });
    function addFiles(files) {
      files.slice(0, 6).forEach((f) => {
        if (f.size > 5 * 1024 * 1024) { toast(f.name + " is larger than 5MB", "bad"); return; }
        if (/^image\//.test(f.type)) {
          C.files.push({ name: f.name, mime: f.type, text: "[image attached: " + f.name + ". A vision-capable model is required to inspect it.]" });
          renderFiles();
          return;
        }
        const reader = new FileReader();
        reader.onload = () => {
          C.files.push({ name: f.name, mime: f.type || "text/plain", text: String(reader.result || "").slice(0, 200000) });
          renderFiles();
        };
        reader.onerror = () => toast("Cannot read " + f.name, "bad");
        reader.readAsText(f);
      });
    }
    function renderFiles() {
      const bar = $("#c2-files");
      if (!bar) return;
      bar.innerHTML = C.files.map((f, i) =>
        '<span class="chip">📎 ' + esc(f.name) + '<button data-rm="' + i + '" title="Remove">×</button></span>').join("");
      bar.querySelectorAll("[data-rm]").forEach((b) => b.addEventListener("click", () => {
        C.files.splice(Number(b.dataset.rm), 1); renderFiles();
      }));
    }
    window.AIOSChatExtras = { renderFiles };

    // ---- advanced options -------------------------------------------
    const optsPanel = $("#c2-opts-panel");
    const optFields = { max_tokens: "#opt-max", temperature: "#opt-temp", context_window: "#opt-ctx", reasoning_effort: "#opt-effort" };
    function loadOpts() {
      let o = {};
      try { o = JSON.parse(localStorage.getItem("aios_chat_options") || "{}"); } catch (e) { o = {}; }
      Object.entries(optFields).forEach(([k, sel]) => { if (o[k] != null) $(sel).value = o[k]; });
    }
    function saveOpts() {
      const o = {};
      Object.entries(optFields).forEach(([k, sel]) => { const v = $(sel).value; if (v !== "") o[k] = k === "reasoning_effort" ? v : Number(v); });
      localStorage.setItem("aios_chat_options", JSON.stringify(o));
      toast("Chat options saved", "ok");
    }
    loadOpts();
    $("#c2-opts").addEventListener("click", () => optsPanel.classList.toggle("hidden"));
    Object.values(optFields).forEach((sel) => $(sel).addEventListener("change", saveOpts));
    $("#opt-reset").addEventListener("click", () => {
      localStorage.removeItem("aios_chat_options");
      Object.values(optFields).forEach((sel) => { $(sel).value = ""; });
      toast("Options reset to model defaults", "ok");
    });

    // ---- character picker -------------------------------------------
    async function openCharPicker() {
      const pers = await loadPersonalities();
      const cards = pers.map((p) => {
        const emoji = X.charEmoji(p);
        return '<button class="char-card' + (p.id === C.personality ? " active" : "") + '" data-pid="' + esc(p.id) + '">' +
          '<span class="cc-avatar">' + emoji + '</span><span class="cc-name">' + esc(p.name) + '</span>' +
          '<span class="cc-desc">' + esc(p.description || (p.tone || "") + " · " + (p.response_length || "")) + '</span>' +
          '<span class="cc-traits">🎭 ' + esc(p.tone || "neutral") + ' · 😄 ' + (p.humor || 0) + '/10 · 💬 ' + esc(p.response_length || "medium") + '</span></button>';
      }).join("");
      A.modal('<div class="pv-head"><strong>Choose a character</strong><span><button class="btn ghost sm" id="cc-new">＋ New</button> ' +
        '<button class="btn ghost sm" onclick="window.AIOS.closeModal()">Close</button></span></div>' +
        '<div class="char-grid">' + (cards || '<p class="muted">No characters yet.</p>') + '</div>');
      $$("#modal .char-card").forEach((card) => card.addEventListener("click", () => {
        C.personality = card.dataset.pid;
        const sel = $("#c2-char");
        if (sel) sel.value = C.personality;
        const chosen = pers.find((p) => p.id === C.personality);
        if (chosen) toast("Character: " + chosen.name, "ok");
        A.closeModal();
      }));
      const newBtn = $("#cc-new");
      if (newBtn) newBtn.addEventListener("click", () => { A.closeModal(); A.navigate("character"); });
    }
    $("#c2-char-pick").addEventListener("click", openCharPicker);

    input.focus();
  };
})();
