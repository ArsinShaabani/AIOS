/* AIOS dashboard: users, audit log and system diagnostics. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, ago, fmtTime, can, setContent } = A;
  const V = window.AIOSViews;
  V.users = async function () {
    if (!can("users:read")) return setContent(V._empty("Administrator access required."));
    const data = await api("/admin/users?limit=100");
    const rows = (data.items || []).map((u) =>
      "<tr><td><strong>" + esc(u.display_name || u.email) + '</strong><div class="small muted">' + esc(u.email) +
      '</div></td><td><select class="input sm" data-role="' + esc(u.id) + '">' + ["user", "operator", "viewer", "admin"].map((r) =>
        '<option' + (u.role === r ? " selected" : "") + ">" + r + "</option>").join("") + '</select></td><td><span class="pill ' +
      (u.status === "active" ? "ok" : "bad") + '">' + esc(u.status) + '</span></td><td class="muted small">' +
      esc(ago(u.last_login_at)) + '</td><td><button class="btn ghost sm" data-reset="' + esc(u.id) + '">رمز</button> ' +
      '<button class="btn ghost sm" data-disable="' + esc(u.id) + '" data-status="' + esc(u.status) + '">' +
      (u.status === "active" ? "غیرفعال" : "فعال") + '</button> ' +
      '<button class="btn ghost sm" data-del="' + esc(u.id) + '">حذف</button></td></tr>').join("");
    setContent('<div class="toolbar"><div class="muted small">' + (data.total || 0) +
      ' users</div><button class="btn sm" id="user-add">Add user</button></div>' +
      '<div class="table-wrap"><table><thead><tr><th>User</th><th>Role</th><th>Status</th><th>Last login</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="5">No users</td></tr>') + "</tbody></table></div>");
    $$("[data-role]").forEach((sel) => sel.addEventListener("change", async () => {
      try { await api("/admin/users/" + sel.dataset.role, { method: "PATCH", body: { role: sel.value } }); toast("نقش تغییر کرد", "ok"); }
      catch (e) { toast(e.message, "bad"); A.navigate("users"); }
    }));
    $$("[data-disable]").forEach((b) => b.addEventListener("click", async () => {
      try { await api("/admin/users/" + b.dataset.disable, { method: "PATCH", body: { status: b.dataset.status === "active" ? "disabled" : "active" } }); A.navigate("users"); }
      catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-reset]").forEach((b) => b.addEventListener("click", async () => {
      const pwd = prompt("رمز جدید (حداقل ۸ کاراکتر):");
      if (!pwd) return;
      try { await api("/admin/users/" + b.dataset.reset + "/reset-password", { method: "POST", body: { password: pwd } }); toast("رمز تغییر کرد", "ok"); }
      catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-del]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("کاربر حذف شود؟")) return;
      try { await api("/admin/users/" + b.dataset.del, { method: "DELETE" }); A.navigate("users"); }
      catch (e) { toast(e.message, "bad"); }
    }));
    $("#user-add").addEventListener("click", () => {
      modal('<h3>Add user</h3><div class="form-grid">' +
        '<div class="field"><label>Email</label><input class="input" id="u-email"></div>' +
        '<div class="field"><label>Display name</label><input class="input" id="u-name"></div>' +
        '<div class="field"><label>Password</label><input class="input" type="password" id="u-pass"></div>' +
        '<div class="field"><label>Role</label><select id="u-role"><option>user</option><option>operator</option><option>viewer</option><option>admin</option></select></div>' +
        '</div><div class="row end"><button class="btn ghost" id="u-cancel">Cancel</button><button class="btn" id="u-save">Create</button></div>');
      $("#u-cancel").addEventListener("click", closeModal);
      $("#u-save").addEventListener("click", async () => {
        try {
          await api("/admin/users", { method: "POST", body: {
            email: $("#u-email").value, display_name: $("#u-name").value,
            password: $("#u-pass").value, role: $("#u-role").value,
          } });
          closeModal(); toast("User created", "ok"); A.navigate("users");
        } catch (e) { toast(e.message, "bad"); }
      });
    });
  };
  V.logs = async function () {
    if (!can("logs:read")) return setContent(V._empty("Administrator access required."));
    const data = await api("/admin/audit?limit=100");
    const rows = (data.items || []).map((e) =>
      "<tr><td>" + esc(fmtTime(e.created_at)) + "</td><td>" + esc(e.actor_type) + "</td><td>" +
      esc(e.action) + "</td><td>" + esc(e.resource) + '</td><td><span class="pill ' +
      (e.status === "success" ? "ok" : "bad") + '">' + esc(e.status) + '</span></td><td class="mono small">' +
      esc(e.ip || "") + "</td></tr>").join("");
    setContent('<div class="toolbar"><div class="muted small">' + (data.total || 0) +
      ' events</div></div><div class="table-wrap"><table><thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Resource</th><th>Status</th><th>IP</th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="6">No events</td></tr>') + "</tbody></table></div>");
  };
  V.system = async function () {
    const [sys, ready, queue] = await Promise.all([
      api("/admin/system"), fetch("/readyz").then((r) => r.json()), api("/admin/queue"),
    ]);
    const s = sys.system || {};
    const checks = (ready.checks || []).map((c) =>
      '<tr><td>' + esc(c.name) + '</td><td><span class="pill ' + (c.status === "ok" ? "ok" : "bad") + '">' +
      esc(c.status) + "</span></td><td>" + c.latency_ms + 'ms</td><td class="muted small">' + esc(c.detail || "") + "</td></tr>").join("");
    setContent('<div class="grid cols-4">' + V._card("CPU", Number(s.cpu_percent || 0).toFixed(1) + "%", s.num_cpu + " cores") +
      V._card("Memory", (s.mem_used_mb || 0) + " MB", "of " + (s.mem_total_mb || 0) + " MB") +
      V._card("Goroutines", s.goroutines, "heap " + Number(s.heap_mb || 0).toFixed(1) + " MB") +
      V._card("Version", sys.version, "uptime " + s.uptime_sec + "s") + "</div>" +
      '<div class="card" style="margin-top:16px"><h3>Health checks</h3><div class="table-wrap"><table>' +
      "<thead><tr><th>Check</th><th>Status</th><th>Latency</th><th>Detail</th></tr></thead><tbody>" + checks +
      "</tbody></table></div></div>" +
      '<div class="card" style="margin-top:16px"><h3>Queue</h3><pre class="mono">' +
      esc(JSON.stringify(queue, null, 2)) + "</pre>" +
      '<button class="btn danger sm" id="prune">Run maintenance</button></div>');
    $("#prune").addEventListener("click", async () => {
      const res = await api("/admin/maintenance/prune", { method: "POST", body: {} });
      toast("Pruned: " + JSON.stringify(res), "ok");
    });
  };
})();
