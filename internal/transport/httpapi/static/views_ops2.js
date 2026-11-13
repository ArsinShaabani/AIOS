/* AIOS dashboard: queue and settings. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, ago, fmtTime, setContent } = A;
  const V = window.AIOSViews;
  V.queue = async function () {
    const [q, tasks] = await Promise.all([api("/admin/queue"), api("/tasks?limit=100")]);
    const rows = (tasks.items || []).map((t) =>
      "<tr><td>" + esc(t.kind) + '</td><td><span class="pill ' + (t.status === "done" ? "ok" : t.status === "dead" ? "bad" : "warn") +
      '">' + esc(t.status) + "</span></td><td>" + t.attempts + "/" + t.max_attempts + "</td><td>" +
      esc(fmtTime(t.run_at)) + '</td><td class="muted small">' + esc(t.last_error || "") + "</td></tr>").join("");
    setContent('<div class="grid cols-4">' + V._card("Queue depth", (q.queue || {}).depth || 0, "pending jobs") +
      V._card("Running", (q.queue || {}).running || 0, "in flight") +
      V._card("Processed", (q.queue || {}).processed || 0, "since boot") +
      V._card("Failed", (q.queue || {}).failed || 0, "exhausted") + "</div>" +
      '<div class="card" style="margin-top:16px"><h3>Durable tasks</h3><div class="table-wrap"><table>' +
      "<thead><tr><th>Kind</th><th>Status</th><th>Attempts</th><th>Run at</th><th>Last error</th></tr></thead><tbody>" +
      (rows || '<tr><td colspan="5">No tasks</td></tr>') + "</tbody></table></div></div>");
  };
  V.settings = async function () {
    const data = await api("/settings");
    const rows = (data.items || []).map((s) =>
      '<tr><td class="mono">' + esc(s.key) + '</td><td class="mono small">' + esc(JSON.stringify(s.value)) +
      '</td><td class="muted small">' + esc(ago(s.updated_at)) + '</td><td><button class="btn ghost sm" data-del="' +
      esc(s.key) + '">Delete</button></td></tr>').join("");
    setContent('<div class="toolbar"><div class="muted small">Global settings</div>' +
      '<button class="btn sm" id="set-add">Set value</button></div>' +
      '<div class="table-wrap"><table><thead><tr><th>Key</th><th>Value</th><th>Updated</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="4">No settings yet. Provider keys belong in config.yaml; integration settings live here.</td></tr>') +
      "</tbody></table></div>" +
      '<div class="card" style="margin-top:16px"><h3>Security note</h3><p class="small muted">Secrets are AES-256-GCM encrypted at rest and never returned by the API. Keys live in <span class="mono">data/keys</span> with 0600 permissions.</p></div>');
    $("#set-add").addEventListener("click", () => {
      modal('<h3>Set setting</h3><div class="field"><label>Key</label><input class="input" id="s-key"></div>' +
        '<div class="field"><label>JSON value</label><textarea class="mono" id="s-val">{"enabled": true}</textarea></div>' +
        '<div class="row end"><button class="btn ghost" id="s-cancel">Cancel</button><button class="btn" id="s-save">Save</button></div>');
      $("#s-cancel").addEventListener("click", closeModal);
      $("#s-save").addEventListener("click", async () => {
        try {
          const value = JSON.parse($("#s-val").value);
          await api("/settings/" + encodeURIComponent($("#s-key").value), { method: "PUT", body: value });
          closeModal(); toast("Saved", "ok"); A.navigate("settings");
        } catch (e) { toast(e.message, "bad"); }
      });
    });
    $$("[data-del]").forEach((el) => el.addEventListener("click", async () => {
      await api("/settings/" + encodeURIComponent(el.dataset.del), { method: "DELETE" });
      A.navigate("settings");
    }));
  };
})();
