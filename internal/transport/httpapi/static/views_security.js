/* AIOS dashboard: security posture and account keys. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, setContent, can, ago, navigate } = A;
  const V = window.AIOSViews;

  V.security = async function () {
    let keys = [], audit = [];
    try { keys = (await api("/auth/api-keys")).items || []; } catch (e) { /* non-fatal */ }
    if (can("logs:read")) {
      try { audit = (await api("/admin/audit?limit=15")).items || []; } catch (e) { /* non-fatal */ }
    }
    const cards = [
      ["Authentication", "JWT access + rotating refresh tokens", "Refresh reuse revokes the whole token family"],
      ["Authorization", "Role-based access control", "admin / operator / user / viewer / device"],
      ["Encryption at rest", "AES-256-GCM keyring", "Provider keys and secrets are never returned"],
      ["Transport", "TLS termination at the proxy", "Put nginx/Caddy in front and force HTTPS"],
    ].map((c) => '<div class="card"><div class="label">' + esc(c[0]) + '</div><h3>' + esc(c[1]) +
      '</h3><div class="small muted">' + esc(c[2]) + "</div></div>").join("");

    const keyRows = keys.map((k) =>
      '<tr><td class="mono">' + esc(k.prefix) + '…</td><td>' + esc(k.name) + "</td><td>" + esc((k.scopes || []).join(", ")) +
      "</td><td>" + (k.provider ? esc(k.provider) + (k.model ? " / " + esc(k.model) : "") : '<span class="muted">internal</span>') +
      '</td><td class="muted small">' + (k.last_used_at ? esc(ago(k.last_used_at)) : "never") +
      '</td><td><button class="btn ghost sm" data-revoke="' + esc(k.id) + '">Revoke</button></td></tr>').join("");

    const auditRows = audit.map((e) =>
      '<tr><td class="mono small">' + esc(e.action) + "</td><td>" + esc(e.status || "") +
      '</td><td class="muted small">' + esc(e.actor_id || "system") + "</td><td>" + esc(ago(e.created_at)) + "</td></tr>").join("");

    setContent('<div class="page-head"><div><h2>Security</h2><div class="muted">Posture, personal API keys and the recent security trail.</div></div>' +
      '<div class="row"><button class="btn ghost" id="sec-users">Users</button><button class="btn ghost" id="sec-logs">Audit log</button><button class="btn ghost" id="sec-settings">Settings</button></div></div>' +
      '<div class="grid cards">' + cards + "</div>" +
      '<div class="card" style="margin-top:16px"><div class="section-title">Your API keys</div>' +
      '<div class="table-wrap"><table><thead><tr><th>Prefix</th><th>Name</th><th>Scopes</th><th>Provider</th><th>Last used</th><th></th></tr></thead><tbody>' +
      (keyRows || '<tr><td colspan="6">No personal keys. Create tunnel keys in «هوش مصنوعی من».</td></tr>') + "</tbody></table></div></div>" +
      (can("logs:read") ? '<div class="card" style="margin-top:16px"><div class="section-title">Recent security events</div>' +
        '<div class="table-wrap"><table><thead><tr><th>Action</th><th>Status</th><th>Actor</th><th>When</th></tr></thead><tbody>' +
        (auditRows || '<tr><td colspan="4">No events yet.</td></tr>') + "</tbody></table></div></div>" : "") +
      '<div class="card" style="margin-top:16px"><h3>Hardening checklist</h3><ul class="small muted">' +
      "<li>Serve AIOS behind HTTPS; keep the keyring directory at 0600 and out of off-host backups.</li>" +
      "<li>Disable <span class=\"mono\">security.allow_registration</span> on a private server.</li>" +
      "<li>Scope tunnel keys to one provider/model and set an expiry.</li>" +
      "<li>Review the audit log after role, key or restore changes.</li></ul></div>");

    $$("[data-revoke]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("Revoke this API key?")) return;
      try { await api("/auth/api-keys/" + encodeURIComponent(b.dataset.revoke), { method: "DELETE" }); toast("Key revoked", "ok"); V.security(); }
      catch (e) { toast(e.message, "bad"); }
    }));
    if ($("#sec-users")) $("#sec-users").addEventListener("click", () => navigate("users"));
    if ($("#sec-logs")) $("#sec-logs").addEventListener("click", () => navigate("logs"));
    if ($("#sec-settings")) $("#sec-settings").addEventListener("click", () => navigate("settings"));
  };
})();
