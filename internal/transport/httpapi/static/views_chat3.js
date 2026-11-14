/* AIOS v2 chat engine (part 1): helpers, message nodes, sessions. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, state, setContent } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});
  const MD = () => window.AIOSMarkdown || { render: (t) => esc(t) };
  const EMOJI = ["🤖", "🧠", "🦊", "🐼", "🚀", "🧙", "👩‍💻", "🕵️", "🌙", "⚡", "🌱", "🐙"];
  const C = { sending: false, abort: null, session: null, lastUser: "", personality: "", model: "", sessions: [], files: [] };

function chatOptions() {
  try {
    const o = JSON.parse(localStorage.getItem("aios_chat_options") || "{}");
    return o.reasoning_effort ? { reasoning_effort: o.reasoning_effort } : {};
  } catch (e) { return {}; }
}

  function hash(s) { let h = 0; for (let i = 0; i < String(s).length; i++) h = (h * 31 + String(s).charCodeAt(i)) | 0; return h; }
  function charEmoji(p) {
    const map = JSON.parse(localStorage.getItem("aios_char_emoji") || "{}");
    return map[p.id] || EMOJI[Math.abs(hash(p.id || p.name)) % EMOJI.length];
  }
  function wishScroll() { const h = $("#c2-msgs"); if (h) h.scrollTop = h.scrollHeight; }

  function msgNode(role, text, meta) {
    const wrap = document.createElement("div");
    wrap.className = "msg " + role;
    const ch = (state.personalities || []).find((p) => p.id === C.personality);
    const emoji = role === "user" ? "🙂" : role === "tool" ? "⚙" : (ch ? charEmoji(ch) : "🤖");
    wrap.innerHTML = '<div class="av">' + emoji + '</div><div style="min-width:0"><div class="bubble"></div>' +
      '<div class="meta"></div><div class="actions"></div></div>';
    const bubble = wrap.querySelector(".bubble");
    if (role === "assistant") bubble.innerHTML = MD().render(text); else bubble.textContent = text;
    if (meta) wrap.querySelector(".meta").textContent = meta;
    const host = $("#c2-msgs");
    if (host) host.appendChild(wrap);
    return wrap;
  }

  function thinking() {
    const node = msgNode("assistant", "");
    const bubble = node.querySelector(".bubble");
    bubble.innerHTML = '<div class="thinking-line"><span class="typing"><i></i><i></i><i></i></span>' +
      '<span class="think-label">Thinking…</span><span class="mono small">0.0s</span></div>';
    const started = Date.now();
    const iv = setInterval(() => {
      const t = node.querySelector(".thinking-line .mono");
      if (t) t.textContent = ((Date.now() - started) / 1000).toFixed(1) + "s";
    }, 100);
    return { node, bubble, content: "", stop: () => clearInterval(iv) };
  }

  function sessionList() {
    const host = $("#c2-sessions");
    if (!host) return;
    host.innerHTML = C.sessions.map((s) =>
      '<div class="s-item' + (s.id === C.session ? " active" : "") + '" data-sid="' + esc(s.id) + '">' +
      '<div style="font-weight:600;font-size:13px">' + esc(s.title || "Untitled") + "</div>" +
      '<div class="small muted">' + esc(A.ago(s.updated_at)) + " · " + s.message_count + " msgs</div></div>").join("") ||
      '<div class="small muted" style="padding:12px">No conversations yet</div>';
    $$("[data-sid]", host).forEach((el) => el.addEventListener("click", () => openSession(el.dataset.sid)));
  }

  async function loadSessions() {
    try { const d = await api("/sessions?limit=50"); C.sessions = d.items || []; sessionList(); } catch (e) {}
  }

  async function openSession(id) {
    try {
      const d = await api("/sessions/" + encodeURIComponent(id) + "/messages?limit=200");
      C.session = id;
      $("#c2-msgs").innerHTML = "";
      (d.items || []).forEach((m) => {
        if (m.role === "tool") msgNode("tool", "⚙ " + (m.name || "tool") + ": " + m.content);
        else msgNode(m.role, m.content, m.agent_id ? m.agent_id + (m.model ? " · " + m.model : "") : "");
      });
      sessionList(); wishScroll();
    } catch (e) { toast(e.message, "bad"); }
  }

  V._chat2 = { send: null, msgNode, thinking, loadSessions, openSession, sessionList,
    state: C, charEmoji, hash, EMOJI, wishScroll };
})();
