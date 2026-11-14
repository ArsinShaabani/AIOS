/* AIOS v2 models (part 2): page render, catalog, Ollama actions. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, setContent, can } = A;
  const V = window.AIOSViews, M = V._models, size = M.size;

  function ollamaRows(ollama) {
    if (!(ollama.items || []).length) {
      return '<div class="small muted">' + esc(ollama.error || "No models installed yet.") + "</div>";
    }
    return ollama.items.map((m) => '<div class="model-row"><div><b>' + esc(m.name) + '</b><div class="small muted">' +
      esc(m.params || m.family || "") + " · " + size(m.size) + "</div></div><span class=\"spacer\"></span>" +
      '<button class="btn sm ghost" data-use="' + esc(m.name) + '">Use</button>' +
      '<button class="btn sm danger" data-rm="' + esc(m.name) + '">Delete</button></div>').join("");
  }

  V.models = async function () {
    if (!can("models:read")) return setContent('<div class="empty">No permission to manage models.</div>');
    const { providers, catalog, ollama } = await M.load();
    const cards = (providers.items || []).map(M.providerCard).join("") || '<div class="empty">No providers configured.</div>';
    const cat = (catalog.items || []).map((c) =>
      '<div class="cat" data-cat="' + esc(c.id) + '"><div class="t">' + esc(c.label) +
      (c.local ? '<span class="pill ok">local</span>' : c.key_required ? '<span class="pill">key</span>' : '<span class="pill accent">no key</span>') +
      '</div><div class="d">' + esc(c.note) + "</div></div>").join("");
    const pulls = (ollama.pulls || providers.pulls || []).map((p) =>
      '<div class="small" style="margin-top:8px">' + esc(p.model) + " — " + esc(p.status) +
      (p.error ? ' <span style="color:var(--bad)">' + esc(p.error) + "</span>" : "") + "</div>" +
      (!p.done ? '<div class="progress"><i></i></div>' : "")).join("");

    setContent(
      '<div class="page-head"><h3>Models</h3><span class="muted small">' +
      (providers.default_provider ? "default: " + esc(providers.default_provider) + " / " + esc(providers.default_model) : "") +
      '</span><span class="spacer"></span><button class="btn" id="pv-add">+ Add provider</button></div>' +
      '<div class="grid">' + cards + "</div>" +
      '<div class="section-title">Ollama models</div><div class="card"><div class="row">' +
      '<input class="input" id="ol-pull" placeholder="pull a model, e.g. qwen2.5:0.5b" style="max-width:360px">' +
      '<button class="btn secondary" id="ol-pull-btn">Pull</button><span class="spacer"></span>' +
      '<span class="small muted">' + esc(ollama.base_url || providers.ollama_url || "") + "</span></div>" +
      '<div style="margin-top:12px">' + ollamaRows(ollama) + "</div>" + pulls + "</div>" +
      '<div class="section-title">Quick add — keyless first</div><div class="cat-grid">' + cat + "</div>");

    $("#pv-add").addEventListener("click", () => M.openForm(null, null));
    $$("[data-cat]").forEach((el) => el.addEventListener("click", () => {
      const c = catalog.items.find((x) => x.id === el.dataset.cat);
      if (c) M.openForm(null, c);
    }));
    $$("[data-edit]").forEach((el) => el.addEventListener("click", () => {
      const p = providers.items.find((x) => x.name === el.dataset.edit);
      M.openForm(p, null);
    }));
    $$("[data-del]").forEach((el) => el.addEventListener("click", async () => {
      if (!confirm("Delete provider " + el.dataset.del + "?")) return;
      try {
        await api("/admin/providers/" + encodeURIComponent(el.dataset.del), { method: "DELETE" });
        toast("Deleted", "ok"); V.models();
      } catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-default]").forEach((el) => el.addEventListener("click", async () => {
      const p = providers.items.find((x) => x.name === el.dataset.default);
      const model = p.default_model || (p.models && p.models[0]) || prompt("Model name");
      if (!model) return;
      try {
        await api("/admin/providers/default", { method: "POST", body: { provider: p.name, model } });
        toast("Default updated", "ok"); V.models();
      } catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-test]").forEach((el) => el.addEventListener("click", async () => {
      toast("Testing " + el.dataset.test + "…");
      try {
        const r = await api("/admin/providers/" + encodeURIComponent(el.dataset.test) + "/test", { method: "POST", body: {} });
        if (r.ok) toast("OK · " + Math.round(r.tok_per_sec || 0) + " tok/s · " + r.latency_ms + "ms · " + (r.reply || ""), "ok");
        else toast(r.error || "test failed", "bad");
      } catch (e) { toast(e.message, "bad"); }
    }));
    $("#ol-pull-btn").addEventListener("click", async () => {
      const model = $("#ol-pull").value.trim();
      if (!model) return;
      try {
        await api("/admin/ollama/pull", { method: "POST", body: { model } });
        toast("Downloading " + model + "…", "ok"); V.models();
      } catch (e) { toast(e.message, "bad"); }
    });
    $$("[data-rm]").forEach((el) => el.addEventListener("click", async () => {
      if (!confirm("Delete model " + el.dataset.rm + "?")) return;
      try {
        await api("/admin/ollama/models?name=" + encodeURIComponent(el.dataset.rm), { method: "DELETE" });
        V.models();
      } catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-use]").forEach((el) => el.addEventListener("click", async () => {
      try {
        await api("/admin/providers/default", { method: "POST", body: { provider: "ollama", model: el.dataset.use } });
        toast("Default model set", "ok"); V.models();
      } catch (e) { toast(e.message, "bad"); }
    }));
    if ((ollama.pulls || []).some((p) => !p.done)) {
      setTimeout(() => { if (A.state.view === "models") V.models(); }, 2500);
    }
  };
})();
