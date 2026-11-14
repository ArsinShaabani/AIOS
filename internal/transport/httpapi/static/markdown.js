/* Minimal, dependency-free Markdown renderer for chat bubbles. */
(function () {
  "use strict";
  const esc = (s) => String(s == null ? "" : s)
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;").replace(/'/g, "&#39;");

  function inline(s) {
    return s
      .replace(/`([^`\n]+)`/g, "<code>$1</code>")
      .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
      .replace(/(^|[\s(])\*([^*\n]+)\*/g, "$1<em>$2</em>")
      .replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  }

  function renderBlocks(text) {
    const lines = String(text || "").replace(/\r\n/g, "\n").split("\n");
    let html = "", i = 0, list = null, para = [];
    const flushPara = () => {
      if (para.length) { html += "<p>" + inline(esc(para.join(" "))) + "</p>"; para = []; }
    };
    const closeList = () => { if (list) { html += "</" + list + ">"; list = null; } };
    while (i < lines.length) {
      const line = lines[i];
      if (/^```/.test(line.trim())) {
        flushPara(); closeList();
        const lang = line.trim().slice(3).trim() || "code";
        const buf = [];
        i++;
        while (i < lines.length && !/^```/.test(lines[i].trim())) { buf.push(lines[i]); i++; }
        html += '<div class="codeblock"><div class="cb-head"><span>' + esc(lang) +
          '</span><button class="cb-copy" type="button">Copy</button></div><pre><code>' +
          esc(buf.join("\n")) + "</code></pre></div>";
        i++;
        continue;
      }
      const h = /^(#{1,4})\s+(.*)$/.exec(line);
      if (h) {
        flushPara(); closeList();
        const level = Math.min(4, h[1].length + 2);
        html += "<h" + level + ">" + inline(esc(h[2])) + "</h" + level + ">";
        i++; continue;
      }
      if (/^\s*([-*+]|\d+\.)\s+/.test(line)) {
        flushPara();
        const ordered = /^\s*\d+\./.test(line);
        const tag = ordered ? "ol" : "ul";
        if (list !== tag) { closeList(); html += "<" + tag + ">"; list = tag; }
        html += "<li>" + inline(esc(line.replace(/^\s*([-*+]|\d+\.)\s+/, ""))) + "</li>";
        i++; continue;
      }
      if (/^\s*>\s?/.test(line)) {
        flushPara(); closeList();
        html += "<blockquote>" + inline(esc(line.replace(/^\s*>\s?/, ""))) + "</blockquote>";
        i++; continue;
      }
      if (/^\s*([-*_])\1{2,}\s*$/.test(line)) { flushPara(); closeList(); html += "<hr>"; i++; continue; }
      if (!line.trim()) { flushPara(); closeList(); i++; continue; }
      para.push(line.trim());
      i++;
    }
    flushPara(); closeList();
    return html;
  }

  function render(text) {
    // Split on fenced blocks so paragraphs/lists outside them keep formatting.
    return renderBlocks(text);
  }

  document.addEventListener("click", (e) => {
    const btn = e.target.closest && e.target.closest(".cb-copy");
    if (!btn) return;
    const pre = btn.closest(".codeblock").querySelector("pre");
    navigator.clipboard.writeText(pre.innerText).then(() => {
      btn.textContent = "Copied";
      setTimeout(() => { btn.textContent = "Copy"; }, 1500);
    }).catch(() => { btn.textContent = "Failed"; });
  });

  window.AIOSMarkdown = { render, esc };
})();
