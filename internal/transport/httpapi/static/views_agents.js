/* AIOS dashboard: agents and model router. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { esc, api, setContent } = A;
  const V = window.AIOSViews;
  V.agents = async function () {
    const data = await api("/agents?all=true");
    const cards = (data.items || []).map((a) =>
      '<div class="card"><div class="row" style="justify-content:space-between"><h3 style="margin:0">' +
      esc(a.name) + '</h3><span class="pill ' + (a.enabled ? "ok" : "bad") + '">' + (a.enabled ? "on" : "off") +
      '</span></div><p class="small muted">' + esc(a.description) + '</p><div class="tool-list">' +
      (a.tools || []).map((t) => '<span class="tool-chip">' + esc(t) + "</span>").join("") + "</div></div>").join("");
    setContent('<div class="toolbar"><div class="muted small">' + (data.items || []).length + " agents · " +
      (data.tools || []).length + ' tools</div></div><div class="grid cols-3">' + cards + "</div>");
  };
  V.models = async function () {
    const data = await api("/models");
    const providers = (data.providers || []).map((p) =>
      '<div class="card"><div class="row" style="justify-content:space-between"><h3 style="margin:0">' + esc(p.name) +
      '</h3><span class="pill ' + (p.healthy ? "ok" : "bad") + '">' + esc(p.breaker) + "</span></div>" +
      '<div class="kv" style="grid-template-columns:110px 1fr"><div class="k">Kind</div><div>' + esc(p.kind) +
      '</div><div class="k">Models</div><div class="mono small">' + esc((p.models || []).join(", ")) +
      "</div></div></div>").join("");
    const rules = (data.routing || []).map((r) => "<tr><td>" + esc(r.name) + "</td><td>" +
      esc((r.intents || []).concat(r.keywords || []).join(", ")) + "</td><td>" + esc(r.provider || "default") +
      "</td><td>" + (r.max_tokens || "—") + "</td></tr>").join("");
    setContent('<div class="card"><h3>Routing defaults</h3><div class="kv">' +
      '<div class="k">Provider / model</div><div class="mono">' + esc(data.default.provider) + " / " + esc(data.default.model) +
      '</div><div class="k">Embeddings</div><div class="mono">' + esc(data.embedding.provider) + " / " +
      esc(data.embedding.model) + '</div></div><p class="small muted">Providers live in config.yaml; rules choose budgets per intent.</p></div>' +
      '<div class="grid cols-2" style="margin-top:16px">' + providers + "</div>" +
      '<div class="card" style="margin-top:16px"><h3>Routing rules</h3><div class="table-wrap"><table><thead><tr><th>Rule</th><th>Matches</th><th>Provider</th><th>Max tokens</th></tr></thead><tbody>' +
      (rules || '<tr><td colspan="4">No rules</td></tr>') + "</tbody></table></div></div>");
  };
})();
