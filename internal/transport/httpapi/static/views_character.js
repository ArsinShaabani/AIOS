/* AIOS v2: character gallery and editor. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, setContent, modal, closeModal } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});
  const EMOJI = ["🤖", "🧠", "🦊", "🐼", "🚀", "🧙", "👩‍💻", "🕵️", "🌙", "⚡", "🌱", "🐙"];
  const TONES = ["friendly", "professional", "concise", "playful", "empathetic", "direct"];
  function hash(s) { let h = 0; for (let i = 0; i < String(s).length; i++) h = (h * 31 + String(s).charCodeAt(i)) | 0; return h; }
  function emojiFor(p) {
    const map = JSON.parse(localStorage.getItem("aios_char_emoji") || "{}");
    return map[p.id] || EMOJI[Math.abs(hash(p.id || p.name)) % EMOJI.length];
  }

  function card(p) {
    const def = localStorage.getItem("aios_default_character") === p.id;
    return '<div class="card char-card' + (def ? " default" : "") + '"><div class="emoji">' + emojiFor(p) + "</div>" +
      '<div class="nm">' + esc(p.name) + (p.is_preset ? ' <span class="pill">preset</span>' : "") + "</div>" +
      '<div class="desc">' + esc(p.description || p.custom_prompt || "No description") + "</div>" +
      '<div class="traits"><span class="pill">' + esc(p.tone) + '</span><span class="pill">humor ' + p.humor + "</span>" +
      '<span class="pill">' + esc(p.response_length) + "</span></div>" +
      '<div class="foot row" style="margin-top:12px"><button class="btn sm secondary" data-use="' + esc(p.id) + '">' +
      (def ? "Default ✓" : "Use") + '</button><button class="btn sm ghost" data-edit="' + esc(p.id) + '">Edit</button>' +
      (!p.is_preset ? '<button class="btn sm danger" data-del="' + esc(p.id) + '">Delete</button>' : "") + "</div></div>";
  }

  function editor(p) {
    p = p || { tone: "friendly", humor: 4, emotional_level: 5, response_length: "medium" };
    return "<h3>" + (p.id ? "Edit character" : "New character") + "</h3>" +
      '<div class="field"><label>Name</label><input id="ch-name" value="' + esc(p.name || "") + '"></div>' +
      '<div class="field"><label>Tagline</label><input id="ch-desc" value="' + esc(p.description || "") + '" placeholder="Calm technical copilot"></div>' +
      '<div class="field"><label>Tone</label><select id="ch-tone">' +
      TONES.map((t) => "<option" + (p.tone === t ? " selected" : "") + ">" + t + "</option>").join("") + "</select></div>" +
      '<div class="slider"><span>Humor</span><span class="v" id="ch-humor-v">' + (p.humor || 0) + '</span><input id="ch-humor" type="range" min="0" max="10" value="' + (p.humor || 0) + '"></div>' +
      '<div class="slider"><span>Warmth</span><span class="v" id="ch-emo-v">' + (p.emotional_level || 0) + '</span><input id="ch-emo" type="range" min="0" max="10" value="' + (p.emotional_level || 0) + '"></div>' +
      '<div class="field"><label>Length</label><select id="ch-len">' +
      ["short", "medium", "long"].map((t) => "<option" + (p.response_length === t ? " selected" : "") + ">" + t + "</option>").join("") + "</select></div>" +
      '<div class="field"><label>Custom instructions</label><textarea id="ch-prompt" placeholder="Speak Persian, give examples, never invent facts…">' + esc(p.custom_prompt || "") + "</textarea></div>" +
      '<div class="row end"><button class="btn ghost" id="ch-cancel">Cancel</button><button class="btn" id="ch-save">Save</button></div>';
  }

  function openEditor(p) {
    modal(editor(p));
    $("#ch-humor").addEventListener("input", (e) => { $("#ch-humor-v").textContent = e.target.value; });
    $("#ch-emo").addEventListener("input", (e) => { $("#ch-emo-v").textContent = e.target.value; });
    $("#ch-cancel").addEventListener("click", closeModal);
    $("#ch-save").addEventListener("click", async () => {
      const body = {
        name: $("#ch-name").value.trim(), description: $("#ch-desc").value.trim(),
        tone: $("#ch-tone").value, style: p && p.style ? p.style : "mixed",
        humor: Number($("#ch-humor").value), emotional_level: Number($("#ch-emo").value),
        response_length: $("#ch-len").value, custom_prompt: $("#ch-prompt").value.trim(),
      };
      if (!body.name) return toast("Name is required", "bad");
      try {
        const saved = p && p.id
          ? await api("/personalities/" + encodeURIComponent(p.id), { method: "PUT", body })
          : await api("/personalities", { method: "POST", body });
        toast("Character saved", "ok"); closeModal(); V.character();
      } catch (e) { toast(e.message, "bad"); }
    });
  }

  V.character = async function () {
    const items = (await api("/personalities")).items || [];
    setContent('<div class="page-head"><h3>Characters</h3><span class="muted small">' + items.length +
      ' personalities</span><span class="spacer"></span><button class="btn" id="ch-new">+ New character</button></div>' +
      '<div class="grid">' + items.map(card).join("") + "</div>" +
      '<div class="card" style="margin-top:16px"><div class="section-title" style="margin-top:0">How characters work</div>' +
      '<div class="small muted">Tone, humor, warmth and length are compiled into the system prompt; custom instructions are appended last. The default character applies to new chats and can be switched per conversation.</div></div>');
    $("#ch-new").addEventListener("click", () => openEditor(null));
    $$("[data-edit]").forEach((el) => el.addEventListener("click", () => openEditor(items.find((x) => x.id === el.dataset.edit))));
    $$("[data-use]").forEach((el) => el.addEventListener("click", () => {
      localStorage.setItem("aios_default_character", el.dataset.use);
      toast("Default character set", "ok"); V.character();
    }));
    $$("[data-del]").forEach((el) => el.addEventListener("click", async () => {
      if (!confirm("Delete this character?")) return;
      try { await api("/personalities/" + encodeURIComponent(el.dataset.del), { method: "DELETE" }); V.character(); }
      catch (e) { toast(e.message, "bad"); }
    }));
  };
})();
