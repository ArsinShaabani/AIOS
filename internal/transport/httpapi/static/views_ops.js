/* AIOS dashboard: usage and plugins. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, setContent } = A;
  const V = window.AIOSViews;
  V.usage = async function () {
    const [totals, daily, providers] = await Promise.all([
      api("/usage?all=true"), api("/usage/daily?all=true"), api("/usage/providers"),
    ]);
    const t = totals.totals || {};
    const rows = (providers.items || []).map((p) => "<tr><td>" + esc(p.provider) + "</td><td>" + p.requests +
      "</td><td>" + p.tokens + "</td><td>$" + Number(p.cost_usd).toFixed(4) + "</td></tr>").join("");
    setContent('<div class="grid cols-4">' + V._card("Requests", t.requests, "selected period") +
      V._card("Prompt tokens", t.prompt_tokens, "input") + V._card("Completion tokens", t.completion_tokens, "output") +
      V._card("Cost", "$" + Number(t.cost_usd || 0).toFixed(4), "avg " + Math.round(t.avg_latency_ms || 0) + " ms") +
      '</div><div class="card" style="margin-top:16px"><h3>Daily tokens</h3>' + V._chart(daily.items) +
      '</div><div class="card" style="margin-top:16px"><h3>By provider</h3><div class="table-wrap"><table>' +
      "<thead><tr><th>Provider</th><th>Requests</th><th>Tokens</th><th>Cost</th></tr></thead><tbody>" +
      (rows || '<tr><td colspan="4">No usage</td></tr>') + "</tbody></table></div></div>");
  };
  V.plugins = async function () {
    const data = await api("/plugins");
    const rows = (data.items || []).map((p) =>
      "<tr><td><strong>" + esc(p.name) + '</strong><div class="small muted">' + esc(p.description) +
      "</div></td><td>" + esc(p.version) + '</td><td class="mono small">' + esc(p.endpoint) +
      '</td><td><span class="pill ' + (p.enabled ? "ok" : "warn") + '">' + (p.enabled ? "on" : "off") +
      '</span></td><td>' + (p.tools || []).map((t) => '<span class="tool-chip">' + esc(t.name) + "</span>").join(" ") +
      '</td><td><button class="btn danger sm" data-del="' + esc(p.id) + '">Remove</button></td></tr>').join("");
    setContent('<div class="toolbar"><div class="muted small">' + (data.items || []).length +
      ' plugins</div><button class="btn sm" id="plug-add">Install plugin</button></div>' +
      '<div class="table-wrap"><table><thead><tr><th>Plugin</th><th>Version</th><th>Endpoint</th><th>State</th><th>Tools</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="6">No plugins installed. Plugins expose HTTP tools, like VSCode extensions.</td></tr>') +
      "</tbody></table></div>");
    $("#plug-add").addEventListener("click", () => {
      const sample = '{\n  "name": "weather",\n  "version": "1.0.0",\n  "description": "Weather tools",\n  "endpoint": "http://127.0.0.1:7070",\n  "permissions": ["internet"],\n  "tools": [{"name":"weather_now","description":"Current weather","method":"POST","path":"/tools/weather_now","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]\n}';
      modal('<h3>Install plugin</h3><div class="field"><label>Manifest (JSON)</label><textarea class="mono" id="p-json">' +
        esc(sample) + '</textarea></div><div class="row end"><button class="btn ghost" id="p-cancel">Cancel</button>' +
        '<button class="btn" id="p-save">Install</button></div>');
      $("#p-cancel").addEventListener("click", closeModal);
      $("#p-save").addEventListener("click", async () => {
        try {
          const manifest = JSON.parse($("#p-json").value);
          await api("/plugins", { method: "POST", body: Object.assign({ enabled: true }, manifest) });
          closeModal(); toast("Plugin installed", "ok"); A.navigate("plugins");
        } catch (e) { toast(e.message, "bad"); }
      });
    });
    $$("[data-del]").forEach((el) => el.addEventListener("click", async () => {
      if (!confirm("Remove this plugin?")) return;
      await api("/plugins/" + el.dataset.del, { method: "DELETE" });
      A.navigate("plugins");
    }));
  };
})();
