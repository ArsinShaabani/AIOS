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
    const out = { items: [{ value: "", label: "سرور (پیش‌فرض)" }], current: localStorage.getItem("aios_chat_model") || "" };
    try {
      const d = await api("/me/models");
      (d.items || []).forEach((p) => {
        (p.models || [p.default_model]).filter(Boolean).forEach((m) =>
          out.items.push({ value: p.provider + "::" + m, label: p.provider + " · " + m }));
      });
    } catch (e) {}
    try {
      if (can("models:read")) {
        const d = await api("/admin/providers");
        (d.items || []).forEach((p) => {
          if (!p.enabled) return;
          (p.models && p.models.length ? p.models : [p.default_model]).filter(Boolean).forEach((m) =>
            out.items.push({ value: "sys::" + p.name + "::" + m, label: "سرور · " + p.name + " · " + m }));
        });
      }
    } catch (e) {}
    return out;
  }

  V.chat = async function () {
    const pers = await loadPersonalities();
    state.personalities = pers;
    C.personality = localStorage.getItem("aios_default_character") || (pers[0] && pers[0].id) || "";
    const models = await loadModels();
    const cur = localStorage.getItem("aios_chat_model") || "";
    const curParts = cur.split("::");
    if (curParts[0] === "sys") curParts.shift();
    C.provider = curParts.shift() || "";
    C.model = curParts.join("::");
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
      '<select id="c2-effort" class="effort-select" title="Reasoning effort"><option value="">effort: auto</option><option value="low">effort: low</option><option value="medium">effort: medium</option><option value="high">effort: high</option></select>' +
      '<span class="spacer"></span><span class="pill" id="c2-status">ready</span>' +
      '<button class="icon-btn" id="c2-voice" title="Voice">🎙</button></div>' +
      '<div id="c2-files" class="chips"></div>' +
      '<input type="file" id="c2-file" multiple hidden accept="text/*,.md,.json,.csv,.py,.js,.ts,.go,.html,.css,.yml,.yaml,.sh,.sql,image/*">' +
      '<div class="msgs" id="c2-msgs"><div class="empty-chat"><div class="big">👋 Hi' +
      (state.user && state.user.display_name ? ", " + esc(state.user.display_name) : "") + "</div>" +
      '<div class="muted">هر سؤالی داری بپرس؛ کد، جدول و ابزارها همین‌جا نمایش داده می‌شوند.</div>' +
      (models.items.length <= 1 ? '<div class="guide"><strong>برای شروع، هوش مصنوعی خودت را وصل کن</strong><p>از منوی «هوش مصنوعی من» یک سرویس‌دهنده اضافه کن — مثلاً Ollama روی همین سرور (بدون کلید) یا Groq/Gemini با کلید رایگان. سپس مدل را از بالای چت انتخاب کن.</p><button class="btn sm" id="c2-go-myai">افزودن مدل</button></div>' : '') +
      '<div class="suggest">' +
      ["دستگاه‌هایم را توضیح بده", "اسنادم را خلاصه کن", "یک اسکریپت پایتون بنویس", "برنامه امروزم را بچین"].map((t) =>
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
    if ($("#c2-go-myai")) $("#c2-go-myai").addEventListener("click", () => A.navigate("myai"));
    $("#c2-stop").addEventListener("click", () => { if (C.abort) C.abort.abort(); });
    $("#c2-char").addEventListener("change", (e) => { C.personality = e.target.value; });
    $("#c2-model").addEventListener("change", (e) => {
      const parts = e.target.value.split("::");
      const sys = parts[0] === "sys";
      if (sys) parts.shift();
      const provider = sys ? "" : (parts.shift() || ""), model = sys ? "" : parts.join("::");
      C.provider = provider; C.model = model;
      localStorage.setItem("aios_chat_model", e.target.value);
      toast("مدل انتخاب شد", "ok");
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

    // ---- reasoning effort only (everything else comes from the provider) ----
    const effortSel = $("#c2-effort");
    function storedOptions() {
      try { return JSON.parse(localStorage.getItem("aios_chat_options") || "{}"); } catch (e) { return {}; }
    }
    effortSel.value = storedOptions().reasoning_effort || "";
    effortSel.addEventListener("change", () => {
      const o = storedOptions();
      // Drop deprecated per-chat overrides so the provider/model config wins.
      delete o.temperature; delete o.max_tokens; delete o.context_window;
      if (effortSel.value) o.reasoning_effort = effortSel.value; else delete o.reasoning_effort;
      localStorage.setItem("aios_chat_options", JSON.stringify(o));
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
