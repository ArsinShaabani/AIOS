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
        html += '<div class="codeblock" data-lang="' + esc(lang) + '"><div class="cb-head"><span>' + esc(lang) +
          '</span><span class="cb-actions"><button class="cb-preview" type="button">Preview</button>' +
          '<button class="cb-copy" type="button">Copy</button></span></div><pre><code>' +
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

  const PREVIEWABLE = ["html", "css", "javascript", "js", "svg", "markdown", "md"];

  function previewDoc(lang, code) {
    const l = lang.toLowerCase();
    if (l === "html") return code;
    if (l === "svg") return '<!doctype html><body style="display:grid;place-items:center;height:100vh;background:#0b0e14">' + code + "</body>";
    if (l === "css") return '<!doctype html><style>body{font:16px system-ui;padding:24px;background:#fff;color:#111}' + code + '</style><h1>Preview</h1><p class="demo">The quick brown fox jumps over the lazy dog.</p><button>Button</button><div class="card">Card content</div>';
    if (l === "js" || l === "javascript") return '<!doctype html><body style="font:16px system-ui;padding:24px"><h1>JS preview</h1><pre id="out"></pre><script>' + code + "<\/script></body>";
    if (l === "md" || l === "markdown") {
      const html = render(code);
      return '<!doctype html><meta charset=utf-8><body style="font:16px system-ui;padding:24px;max-width:820px;margin:auto;line-height:1.55">' + html + "</body>";
    }
    return null;
  }

  document.addEventListener("click", (e) => {
    const pv = e.target.closest && e.target.closest(".cb-preview");
    if (pv) {
      const block = pv.closest(".codeblock");
      const lang = block.dataset.lang || "code";
      const code = block.querySelector("pre").innerText;
      const doc = PREVIEWABLE.indexOf(lang.toLowerCase()) >= 0 ? previewDoc(lang, code) : null;
      if (!doc) { window.AIOS && window.AIOS.toast("Preview supports HTML, CSS, JS, SVG and Markdown", "bad"); return; }
      if (window.AIOS && window.AIOS.modal) {
        window.AIOS.modal('<div class="pv-head"><strong>Preview · ' + esc(lang) + '</strong>' +
          '<button class="btn ghost sm" onclick="window.AIOS.closeModal()">Close</button></div>' +
          '<iframe class="pv-frame" sandbox="allow-scripts" srcdoc="' + esc(doc) + '"></iframe>');
      }
      return;
    }
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
