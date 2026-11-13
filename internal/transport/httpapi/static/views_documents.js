/* AIOS dashboard: document RAG (upload, list, citations). */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, ago, state, setContent } = A;
  const V = window.AIOSViews;

  V.documents = async function () {
    const data = await api("/documents?limit=50");
    const rows = (data.items || []).map((d) =>
      "<tr><td><strong>" + esc(d.title) + '</strong><div class="small muted">' + esc(d.source || d.mime) +
      '</div></td><td><span class="pill ' + (d.status === "indexed" ? "ok" : "bad") + '">' + esc(d.status) +
      "</span></td><td>" + d.chunk_count + "</td><td>" + Math.round(d.size / 1024) + " KB" +
      '</td><td class="muted small">' + esc(ago(d.created_at)) +
      '</td><td><button class="btn ghost sm" data-chunks="' + esc(d.id) + '">Chunks</button> ' +
      '<button class="btn danger sm" data-del="' + esc(d.id) + '">Delete</button></td></tr>').join("");
    setContent(
      '<div class="toolbar"><div class="row">' +
      '<input class="input" id="doc-q" placeholder="Search your documents…">' +
      '<button class="btn secondary sm" id="doc-search">Search</button></div>' +
      '<div class="row"><input type="file" id="doc-file" class="input" style="max-width:240px">' +
      '<input class="input" id="doc-title" placeholder="Title (optional)" style="max-width:200px">' +
      '<button class="btn sm" id="doc-upload">Upload</button></div></div>' +
      '<div class="table-wrap"><table><thead><tr><th>Document</th><th>Status</th><th>Chunks</th><th>Size</th><th>Added</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="6">No documents yet. Upload text, Markdown, HTML, JSON or CSV to enable citations.</td></tr>') +
      "</tbody></table></div>" +
      '<div id="doc-results" style="margin-top:16px"></div>');
    $("#doc-upload").addEventListener("click", uploadDocument);
    $("#doc-search").addEventListener("click", searchDocuments);
    $$("[data-del]").forEach((el) => el.addEventListener("click", async () => {
      if (!confirm("Delete this document and its index?")) return;
      await api("/documents/" + el.dataset.del, { method: "DELETE" });
      toast("Document deleted", "ok");
      A.navigate("documents");
    }));
    $$("[data-chunks]").forEach((el) => el.addEventListener("click", async () => {
      const res = await api("/documents/" + el.dataset.chunks + "/chunks?limit=100");
      modal("<h3>Document chunks</h3>" + (res.items || []).map((c) =>
        '<div class="card" style="margin:8px 0"><div class="small muted">#' + c.index + " · " + c.tokens + " tokens</div><div>" +
        esc(c.content) + "</div></div>").join("") +
        '<div class="row end"><button class="btn" id="ch-close">Close</button></div>');
      $("#ch-close").addEventListener("click", closeModal);
    }));
  };

  async function uploadDocument() {
    const file = $("#doc-file").files[0];
    if (!file) return toast("Choose a file first", "bad");
    const form = new FormData();
    form.append("file", file);
    const title = $("#doc-title").value.trim();
    if (title) form.append("title", title);
    try {
      const res = await fetch(A.API + "/documents", {
        method: "POST", headers: { Authorization: "Bearer " + state.token }, body: form,
      });
      const data = await res.json();
      if (!res.ok) throw new Error((data.error && data.error.message) || "upload failed");
      toast("Indexed " + data.chunk_count + " chunks", "ok");
      A.navigate("documents");
    } catch (e) { toast(e.message, "bad"); }
  }

  async function searchDocuments() {
    const q = $("#doc-q").value.trim();
    if (!q) return;
    try {
      const res = await api("/documents/search?q=" + encodeURIComponent(q) + "&limit=6");
      const items = res.items || [];
      $("#doc-results").innerHTML = '<div class="card"><h3>Passages for “' + esc(q) + '”</h3>' +
        (items.length ? items.map((m, i) =>
          '<div style="border-top:1px solid var(--line);padding:12px 0"><div class="row" style="justify-content:space-between">' +
          '<strong>[' + (i + 1) + "] " + esc(m.title) + '</strong><span class="pill">score ' +
          Number(m.score).toFixed(2) + "</span></div>" +
          '<div class="small muted">chunk #' + m.chunk_index + " · " + esc(m.document_id) + "</div><div>" +
          esc(m.content) + "</div></div>").join("") : '<div class="empty">No matching passages.</div>') + "</div>";
    } catch (e) { toast(e.message, "bad"); }
  }
})();
