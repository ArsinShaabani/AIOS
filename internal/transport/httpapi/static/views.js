/* AIOS dashboard: shared helpers and the overview view. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, esc, api, setContent, state } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});

  V._empty = function (text) { return '<div class="empty">' + esc(text) + "</div>"; };
  V._card = function (label, value, sub) {
    return '<div class="card stat"><div class="muted small">' + esc(label) + '</div><div class="big">' +
      esc(value) + "</div>" + (sub ? '<div class="sub">' + esc(sub) + "</div>" : "") + "</div>";
  };
  V._chart = function (points) {
    if (!points || !points.length) return V._empty("No usage recorded yet.");
    const w = 720, h = 150, pad = 22;
    const max = Math.max.apply(null, points.map((p) => p.tokens || p.requests || 1));
    const bw = (w - pad * 2) / points.length;
    let bars = "";
    points.forEach((p, i) => {
      const v = p.tokens || p.requests || 0;
      const bh = Math.max(2, (v / max) * (h - pad * 2));
      const x = pad + i * bw + bw * 0.15;
      const y = h - pad - bh;
      bars += '<rect class="bar" x="' + x.toFixed(1) + '" y="' + y.toFixed(1) + '" width="' +
        (bw * 0.7).toFixed(1) + '" height="' + bh.toFixed(1) + '"></rect>';
      if (i % Math.ceil(points.length / 8) === 0) {
        bars += '<text class="chart-label" x="' + x.toFixed(1) + '" y="' + (h - 6) + '">' +
          esc(String(p.day).slice(5)) + "</text>";
      }
    });
    return '<svg class="chart" viewBox="0 0 ' + w + " " + h + '" preserveAspectRatio="none">' +
      '<defs><linearGradient id="barGrad" x1="0" x2="0" y1="0" y2="1">' +
      '<stop offset="0" stop-color="#4f8ef7"/><stop offset="1" stop-color="#7c5cff"/></linearGradient></defs>' +
      '<line class="axis" x1="' + pad + '" y1="' + (h - pad) + '" x2="' + (w - pad) + '" y2="' + (h - pad) + '"/>' +
      bars + "</svg>";
  };

  V.overview = async function () {
    const [ov, usage, daily, models] = await Promise.all([
      api("/admin/overview"), api("/usage?all=true"), api("/usage/daily?all=true"), api("/models"),
    ]);
    const o = ov.overview || {}, sys = ov.system || {}, u = usage.totals || {};
    const providerRows = (models.providers || []).map((p) =>
      "<tr><td>" + esc(p.name) + "</td><td>" + esc(p.kind) + '</td><td><span class="pill ' +
      (p.healthy ? "ok" : "bad") + '">' + esc(p.breaker) + '</span></td><td class="mono">' +
      esc((p.models || []).join(", ")) + "</td></tr>").join("");
    setContent(
      '<div class="grid cols-4">' +
      V._card("Users", o.users, "registered principals") +
      V._card("Sessions", o.sessions, (o.messages || 0) + " messages") +
      V._card("Devices", o.devices, ((o.devices_by_status && o.devices_by_status.online) || 0) + " online") +
      V._card("Memories", o.memories, "long-term + semantic") +
      "</div>" +
      '<div class="grid cols-4" style="margin-top:16px">' +
      V._card("Requests today", u.requests, (u.prompt_tokens || 0) + " in / " + (u.completion_tokens || 0) + " out") +
      V._card("Cost today", "$" + Number(u.cost_usd || 0).toFixed(4), "budget $" + Number(usage.budget_usd || 0).toFixed(2) + "/day") +
      V._card("Agents", o.agents, (o.pending_tasks || 0) + " queued tasks") +
      V._card("Uptime", Math.floor((ov.uptime_sec || 0) / 60) + "m", "v" + (ov.version || "")) +
      "</div>" +
      '<div class="split" style="margin-top:16px"><div class="card"><h3>Token usage (30 days)</h3>' +
      V._chart(daily.items) + "</div>" +
      '<div class="card"><h3>Host</h3><div class="kv">' +
      '<div class="k">CPU</div><div>' + Number(sys.cpu_percent || 0).toFixed(1) + '%' +
      '<div class="progress" style="margin-top:6px"><span style="width:' + Math.min(100, sys.cpu_percent || 0) + '%"></span></div></div>' +
      '<div class="k">Memory</div><div>' + (sys.mem_used_mb || 0) + " / " + (sys.mem_total_mb || 0) + " MB" +
      '<div class="progress" style="margin-top:6px"><span style="width:' +
      Math.min(100, (sys.mem_used_mb || 0) / Math.max(1, sys.mem_total_mb || 1) * 100) + '%"></span></div></div>' +
      '<div class="k">Load (1/5/15)</div><div>' + Number(sys.load_1 || 0).toFixed(2) + " / " +
      Number(sys.load_5 || 0).toFixed(2) + " / " + Number(sys.load_15 || 0).toFixed(2) + "</div>" +
      '<div class="k">Heap</div><div>' + Number(sys.heap_mb || 0).toFixed(1) + " MB</div>" +
      '<div class="k">Goroutines</div><div>' + (sys.goroutines || 0) + "</div>" +
      '<div class="k">Audit events</div><div>' + (o.audit_events || 0) + "</div>" +
      "</div></div></div>" +
      '<div class="card" style="margin-top:16px"><h3>Model providers</h3><div class="table-wrap"><table>' +
      "<thead><tr><th>Provider</th><th>Kind</th><th>State</th><th>Models</th></tr></thead><tbody>" +
      (providerRows || '<tr><td colspan="4">No providers</td></tr>') + "</tbody></table></div></div>"
    );
  };
})();
