/* AIOS dashboard: chat screen and sessions list. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, state, setContent, ago } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});

  V.chat = async function () {
    const C = V._chat;
    await C.loadSessions();
    state.currentSession = null;
    setContent('<div class="chat-layout"><div class="sessions" id="sessions">' + C.sessionItems() +
      '</div><div class="chat-pane"><div class="chat-head">' +
      '<div><div style="font-weight:600">New conversation</div>' +
      '<div class="small muted">messages are stored locally</div></div><div class="spacer"></div>' +
      '<button class="btn ghost sm" id="new-chat">New</button>' +
      '<button class="btn secondary sm" id="voice-btn">🎙 Voice</button></div>' +
      '<div class="messages" id="messages"></div>' +
      '<div class="composer"><textarea id="composer" placeholder="Ask anything… (Enter to send, Shift+Enter for a new line)"></textarea>' +
      '<button class="btn" id="send-btn">Send</button></div></div></div>');
    C.bindSessionClicks();
    $("#send-btn").addEventListener("click", C.sendMessage);
    $("#composer").addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); C.sendMessage(); }
    });
    $("#new-chat").addEventListener("click", () => {
      state.currentSession = null;
      $("#messages").innerHTML = "";
    });
    $("#voice-btn").addEventListener("click", () => {
      if (!window.AIOSVoice) return toast("Voice UI unavailable", "bad");
      window.AIOSVoice.toggle({
        onTranscript: (t) => C.bubble("user", t),
        onReply: (t) => C.bubble("assistant", t),
        onSession: (id) => { state.currentSession = id; },
        onError: (e) => toast(e, "bad"),
      });
    });
  };

  V.sessions = async function () {
    const data = await api("/sessions?limit=100");
    const rows = (data.items || []).map((s) =>
      '<tr><td><a href="#" data-open="' + esc(s.id) + '">' + esc(s.title || "Untitled") + "</a></td>" +
      "<td>" + esc(s.channel) + "</td><td>" + s.message_count + "</td><td>" + esc(ago(s.updated_at)) +
      '</td><td><button class="btn ghost sm" data-del="' + esc(s.id) + '">Delete</button>' +
      (A.can("users:read") ? ' <button class="btn ghost sm" data-prompt="' + esc(s.id) + '">Prompt</button>' : "") +
      "</td></tr>").join("");
    setContent('<div class="toolbar"><div class="muted small">' + (data.total || 0) +
      ' conversations</div><button class="btn sm" id="go-chat">New conversation</button></div>' +
      '<div class="table-wrap"><table><thead><tr><th>Title</th><th>Channel</th><th>Messages</th><th>Updated</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="5">No conversations</td></tr>') + "</tbody></table></div>");
    $("#go-chat").addEventListener("click", () => A.navigate("chat"));
    $$("[data-del]").forEach((el) => el.addEventListener("click", async () => {
      if (!confirm("Delete this conversation?")) return;
      await api("/sessions/" + el.dataset.del, { method: "DELETE" });
      toast("Session deleted", "ok");
      A.navigate("sessions");
    }));
    $$("[data-open]").forEach((el) => el.addEventListener("click", async (e) => {
      e.preventDefault();
      await A.navigate("chat");
      await V._chat.openSession(el.dataset.open);
    }));
    $$("[data-prompt]").forEach((el) => el.addEventListener("click", async () => {
      try {
        const p = await A.api("/admin/prompt/" + encodeURIComponent(el.dataset.prompt));
        const msgs = (p.messages || []).map((m) => "[" + m.role + "]\n" + m.content).join("\n\n");
        A.modal("<h3>Rendered prompt</h3><p class=\"small muted\">~" + p.estimated_tokens +
          " tokens · agent " + A.esc(p.agent_id) + " · personality " + A.esc(p.personality_id) + "</p>" +
          "<pre class=\"mono\" style=\"white-space:pre-wrap;max-height:50vh;overflow:auto;background:#0c1320;padding:12px;border-radius:8px\">" +
          A.esc("SYSTEM\n" + p.system + "\n\n" + msgs) + "</pre>" +
          '<div class="row end"><button class="btn" id="prompt-close">Close</button></div>');
        A.$("#prompt-close").addEventListener("click", A.closeModal);
      } catch (err) { A.toast(err.message, "bad"); }
    }));
  };
})();
