/* AIOS dashboard: chat engine (streaming) and session helpers. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, state, setContent, ago } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});
  let sending = false;

  function sessionItems() {
    if (!state.sessions.length) return V._empty("No conversations yet.");
    return state.sessions.map((s) =>
      '<div class="session-item' + (state.currentSession === s.id ? " active" : "") +
      '" data-session="' + esc(s.id) + '"><div class="title">' + esc(s.title || "Untitled") +
      '</div><div class="meta">' + esc(ago(s.updated_at)) + " · " + s.message_count + " msgs</div></div>").join("");
  }
  async function loadSessions() {
    const data = await api("/sessions?limit=60");
    state.sessions = data.items || [];
  }
  function bindSessionClicks() {
    $$(".session-item").forEach((el) => el.addEventListener("click", () => openSession(el.dataset.session)));
  }
  function bubble(role, text, meta) {
    const box = document.createElement("div");
    box.className = "msg " + role;
    const body = document.createElement("div");
    body.className = "body";
    body.textContent = text || "";
    box.appendChild(body);
    if (meta) {
      const m = document.createElement("div");
      m.className = "meta";
      m.textContent = meta;
      box.appendChild(m);
    }
    const host = $("#messages");
    if (host) { host.appendChild(box); host.scrollTop = host.scrollHeight; }
    return box;
  }
  async function openSession(id) {
    state.currentSession = id;
    $$(".session-item").forEach((el) => el.classList.toggle("active", el.dataset.session === id));
    if (!id) { const m = $("#messages"); if (m) m.innerHTML = ""; return; }
    const data = await api("/sessions/" + encodeURIComponent(id) + "/messages?limit=100");
    const host = $("#messages");
    if (!host) return;
    host.innerHTML = "";
    (data.items || []).slice().reverse().forEach((m) => {
      if (m.role === "tool") bubble("tool", "🔧 " + (m.name || "tool") + ": " + m.content);
      else if (m.role === "user" || m.role === "assistant") {
        bubble(m.role, m.content, m.agent_id ? m.agent_id + (m.model ? " · " + m.model : "") : "");
      }
    });
    host.scrollTop = host.scrollHeight;
  }
  async function sendMessage() {
    const box = $("#composer");
    if (!box || sending) return;
    const text = box.value.trim();
    if (!text) return;
    box.value = "";
    sending = true;
    if ($("#send-btn")) $("#send-btn").disabled = true;
    bubble("user", text);
    const assistant = bubble("assistant", "");
    const body = assistant.querySelector(".body");
    body.innerHTML = '<span class="typing"><i></i><i></i><i></i></span>';
    let first = true, content = "";
    const meta = document.createElement("div");
    meta.className = "meta";
    try {
      const res = await fetch(A.API + "/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: "Bearer " + state.token },
        body: JSON.stringify({ message: text, session_id: state.currentSession || "", stream: true }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error((err.error && err.error.message) || "Request failed");
      }
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let idx;
        while ((idx = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0, idx).trim();
          buffer = buffer.slice(idx + 2);
          if (!frame.startsWith("data:")) continue;
          let ev;
          try { ev = JSON.parse(frame.slice(5).trim()); } catch (e) { continue; }
          if (ev.type === "session" && ev.session_id) {
            state.currentSession = ev.session_id;
          } else if (ev.type === "delta") {
            if (first) { body.textContent = ""; first = false; }
            content += ev.delta;
            body.textContent = content;
            const host = $("#messages");
            if (host) host.scrollTop = host.scrollHeight;
          } else if (ev.type === "tool_start") {
            bubble("tool", "🔧 calling " + ev.tool.name + "(" + JSON.stringify(ev.tool.arguments || {}) + ")");
          } else if (ev.type === "tool_result") {
            bubble("tool", (ev.tool.is_error ? "⚠ " : "✓ ") + ev.tool.name + ": " + (ev.tool.result || ""));
          } else if (ev.type === "done") {
            meta.textContent = [ev.agent, ev.provider, ev.model].filter(Boolean).join(" · ");
            if (ev.usage) meta.textContent += " · " + (ev.usage.total_tokens || 0) + " tokens";
          } else if (ev.type === "error") {
            toast(ev.error || "Generation failed", "bad");
            if (first) { body.textContent = "⚠ " + (ev.error || "Generation failed"); first = false; }
          }
        }
      }
      if (first) body.textContent = content || "(no content)";
      if (meta.textContent) assistant.appendChild(meta);
      await loadSessions();
      const host = $("#sessions");
      if (host) { host.innerHTML = sessionItems(); bindSessionClicks(); }
    } catch (e) {
      toast(e.message, "bad");
      body.textContent = "⚠ " + e.message;
    } finally {
      sending = false;
      if ($("#send-btn")) $("#send-btn").disabled = false;
    }
  }
  V._chat = {
    sessionItems, loadSessions, bindSessionClicks, bubble, openSession, sendMessage,
  };
})();
