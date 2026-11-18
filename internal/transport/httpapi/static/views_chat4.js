/* AIOS v2 chat (part 2a): streaming send with markdown + tool cards. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, esc, toast, state } = A;
  const V = window.AIOSViews, X = V._chat2, C = X.state;
  const MD = () => window.AIOSMarkdown || { render: (t) => esc(t) };

  X.send = async function (text) {
    if (C.sending || !text.trim()) return;
    C.sending = true; C.lastUser = text;
    X.msgNode("user", text);
    const input = $("#c2-input");
    if (input) { input.value = ""; input.style.height = "auto"; }
    const sendBtn = $("#c2-send"), stopBtn = $("#c2-stop");
    if (sendBtn) sendBtn.disabled = true;
    if (stopBtn) stopBtn.classList.remove("hidden");
    const st = X.thinking();
    C.abort = new AbortController();
    try {
      const res = await fetch(A.API + "/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: "Bearer " + state.token },
        signal: C.abort.signal,
        body: JSON.stringify({ message: text, session_id: C.session || "", stream: true, personality_id: C.personality || "", provider: C.provider || "", model: C.model || "" }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error((err.error && err.error.message) || "Request failed");
      }
      const reader = res.body.getReader(), decoder = new TextDecoder();
      let buffer = "", lastPaint = 0;
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let idx;
        while ((idx = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0, idx).trim(); buffer = buffer.slice(idx + 2);
          if (!frame.startsWith("data:")) continue;
          let ev; try { ev = JSON.parse(frame.slice(5).trim()); } catch (e) { continue; }
          if (ev.type === "session" && ev.session_id) C.session = ev.session_id;
          else if (ev.type === "delta") {
            if (!st.content) st.stop();
            st.content += ev.delta;
            const now = Date.now();
            if (now - lastPaint > 40) {
              st.bubble.innerHTML = MD().render(st.content) + '<span class="cursor"></span>';
              lastPaint = now; X.wishScroll();
            }
          } else if (ev.type === "tool_start") {
            st.bubble.insertAdjacentHTML("beforeend", '<div class="small muted mono" data-tool="' + esc(ev.tool.name) + '">⚙ ' + esc(ev.tool.name) + "…</div>");
          } else if (ev.type === "tool_result") {
            st.node.querySelectorAll('[data-tool="' + ev.tool.name + '"]').forEach((n) => n.remove());
            st.bubble.insertAdjacentHTML("beforeend", '<details class="small muted mono"><summary>' +
              (ev.tool.is_error ? "⚠ " : "✓ ") + esc(ev.tool.name) + '</summary><pre style="white-space:pre-wrap;max-height:160px;overflow:auto">' +
              esc(ev.tool.result || "") + "</pre></details>");
          } else if (ev.type === "done") {
            st.stop();
            if (!st.content && ev.content) st.content = ev.content;
            st.bubble.innerHTML = MD().render(st.content || "(no content)");
            const bits = [ev.agent, ev.provider, ev.model].filter(Boolean);
            if (ev.usage) bits.push((ev.usage.total_tokens || 0) + " tokens");
            st.node.querySelector(".meta").textContent = bits.join(" · ");
            if (ev.notice) st.bubble.insertAdjacentHTML("beforeend",
              '<div class="small" style="margin-top:8px;color:var(--warn)">⚠ ' + esc(ev.notice) + "</div>");
            const acts = st.node.querySelector(".actions");
            acts.innerHTML = '<button class="act">Copy</button>';
            acts.firstChild.addEventListener("click", () =>
              navigator.clipboard.writeText(st.content).then(() => toast("Copied", "ok")));
          } else if (ev.type === "error") {
            st.stop();
            st.bubble.innerHTML = '<span style="color:var(--bad)">⚠ ' + esc(ev.error || "Generation failed") + "</span>";
          }
        }
      }
      if (!st.content) { st.stop(); st.bubble.innerHTML = MD().render("(no content)"); }
      await X.loadSessions();
    } catch (e) {
      st.stop();
      st.bubble.innerHTML = e.name === "AbortError"
        ? MD().render(st.content || "") + '<div class="small muted">⏹ stopped</div>'
        : '<span style="color:var(--bad)">⚠ ' + esc(e.message) + "</span>";
    } finally {
      C.sending = false; C.abort = null;
      if (sendBtn) sendBtn.disabled = false;
      if (stopBtn) stopBtn.classList.add("hidden");
      X.wishScroll();
    }
  };
})();
