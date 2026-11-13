/* AIOS dashboard: memory browser. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, ago, setContent } = A;
  const V = window.AIOSViews;
  V.memory = async function () {
    const data = await api("/memories?limit=100");
    const rows = (data.items || []).map((m) =>
      '<tr><td><span class="pill">' + esc(m.kind) + "</span></td><td>" + esc(m.content) +
      "</td><td>" + Number(m.importance).toFixed(2) + '</td><td class="muted small">' + esc(ago(m.created_at)) +
      '</td><td><button class="btn ghost sm" data-del="' + esc(m.id) + '">Delete</button></td></tr>').join("");
    setContent('<div class="toolbar"><div class="muted small">' + (data.total || 0) +
      ' memories</div><button class="btn sm" id="mem-add">Add memory</button></div>' +
      '<div class="table-wrap"><table><thead><tr><th>Kind</th><th>Content</th><th>Importance</th><th>Created</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="5">Nothing stored yet. Memories are extracted from conversations automatically.</td></tr>') +
      "</tbody></table></div>" +
      '<div class="card" style="margin-top:16px"><h3>Three memory tiers</h3><p class="small muted">' +
      "Short-term = this session. Long-term = durable facts and preferences. Semantic = embeddings for related wording, using the local CPU embedder by default.</p>" +
      '<button class="btn danger sm" id="mem-wipe">Delete all my memories</button></div>');
    $("#mem-add").addEventListener("click", () => {
      modal('<h3>Add memory</h3><div class="field"><label>Content</label><textarea id="m-content"></textarea></div>' +
        '<div class="row end"><button class="btn ghost" id="m-cancel">Cancel</button><button class="btn" id="m-save">Save</button></div>');
      $("#m-cancel").addEventListener("click", closeModal);
      $("#m-save").addEventListener("click", async () => {
        await api("/memories", { method: "POST", body: { content: $("#m-content").value } });
        closeModal(); toast("Memory saved", "ok"); A.navigate("memory");
      });
    });
    $$("[data-del]").forEach((el) => el.addEventListener("click", async () => {
      await api("/memories/" + el.dataset.del, { method: "DELETE" });
      toast("Deleted", "ok"); A.navigate("memory");
    }));
    $("#mem-wipe").addEventListener("click", async () => {
      if (!confirm("Delete ALL your memories?")) return;
      const res = await api("/memories", { method: "DELETE" });
      toast("Deleted " + res.deleted + " memories", "ok"); A.navigate("memory");
    });
  };
})();
