/* AIOS v2 models (part 1): data loading, cards, add/edit form. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, esc, api, toast, modal, closeModal } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});

  function size(n) {
    if (!n) return "—";
    return n >= 1e9 ? (n / 1e9).toFixed(2) + " GB" : Math.round(n / 1e6) + " MB";
  }

  async function load() {
    const [providers, catalog, ollama] = await Promise.all([
      api("/admin/providers").catch(() => ({ items: [] })),
      api("/admin/providers/catalog").catch(() => ({ items: [] })),
      api("/admin/ollama/models").catch(() => ({ items: [], error: "ollama not reachable" })),
    ]);
    return { providers, catalog, ollama };
  }

  function providerCard(p) {
    return '<div class="card provider-card"><div class="top"><span class="name">' + esc(p.name) + "</span>" +
      (p.default ? '<span class="pill accent">default</span>' : "") + '<span class="spacer"></span>' +
      (p.healthy ? '<span class="pill ok">healthy</span>' : '<span class="pill bad">offline</span>') + "</div>" +
      '<div class="url">' + esc(p.kind) + " · " + esc(p.base_url || "local") + "</div>" +
      '<div class="models">' + (p.models || []).map((m) => '<span class="pill">' + esc(m) + "</span>").join("") + "</div>" +
      '<div class="small muted">' + (p.has_api_key ? "API key " + esc(p.api_key_hint) : "no API key") + "</div>" +
      (p.last_error ? '<div class="small" style="color:var(--bad)">' + esc(p.last_error) + "</div>" : "") +
      '<div class="foot"><button class="btn sm secondary" data-test="' + esc(p.name) + '">Test</button>' +
      '<button class="btn sm ghost" data-edit="' + esc(p.name) + '">Edit</button>' +
      '<button class="btn sm ghost" data-default="' + esc(p.name) + '">Set default</button>' +
      (p.default ? "" : '<button class="btn sm danger" data-del="' + esc(p.name) + '">Delete</button>') +
      "</div></div>";
  }

  function form(p, preset) {
    p = p || {}; preset = preset || {};
    const models = (p.models || preset.models || [preset.model]).filter(Boolean).join(", ");
    return "<h3>" + (p.name ? "Edit provider" : "Add provider") + "</h3>" +
      '<div class="small muted">Keys are AES-GCM encrypted at rest and applied without a restart.</div>' +
      '<div class="field"><label>Name</label><input id="pv-name" value="' + esc(p.name || preset.id || "") + '"' + (p.name ? " disabled" : "") + '></div>' +
      '<div class="field"><label>Kind</label><select id="pv-kind">' +
      ["openai", "anthropic", "gemini", "ollama"].map((k) =>
        "<option" + ((p.kind || preset.kind) === k ? " selected" : "") + ">" + k + "</option>").join("") + "</select></div>" +
      '<div class="field"><label>Base URL</label><input id="pv-url" value="' + esc(p.base_url || preset.base_url || "") + '"></div>' +
      '<div class="field"><label>API key</label><input id="pv-key" type="password" placeholder="' +
      (p.has_api_key ? "leave blank to keep " + esc(p.api_key_hint) : (preset.key_required ? "required" : "optional")) + '"></div>' +
      '<div class="field"><label>Models (comma separated)</label><input id="pv-models" value="' + esc(models) + '"></div>' +
      '<div class="field"><label>Default model</label><input id="pv-model" value="' + esc(p.default_model || preset.model || "") + '"></div>' +
      '<div class="row"><label class="switch"><input type="checkbox" id="pv-enabled"' + (p.enabled === false ? "" : " checked") + "> enabled</label>" +
      '<span class="spacer"></span><label class="small muted">priority <input id="pv-priority" type="number" value="' + (p.priority || 0) + '" style="width:70px"></label></div>' +
      '<div class="row end" style="margin-top:16px"><button class="btn ghost" id="pv-cancel">Cancel</button>' +
      '<button class="btn" id="pv-save">Save &amp; apply</button></div>';
  }

  function openForm(p, preset) {
    modal(form(p, preset));
    $("#pv-cancel").addEventListener("click", closeModal);
    $("#pv-save").addEventListener("click", async () => {
      const name = $("#pv-name").value.trim().toLowerCase();
      const body = {
        kind: $("#pv-kind").value, base_url: $("#pv-url").value.trim(), api_key: $("#pv-key").value,
        models: $("#pv-models").value.split(",").map((s) => s.trim()).filter(Boolean),
        default_model: $("#pv-model").value.trim(), enabled: $("#pv-enabled").checked,
        priority: Number($("#pv-priority").value) || 0,
      };
      try {
        await api("/admin/providers/" + encodeURIComponent(name), { method: "PUT", body });
        toast("Provider saved and applied", "ok");
        closeModal(); V.models();
      } catch (e) { toast(e.message, "bad"); }
    });
  }

  V._models = { load, providerCard, openForm, size };
})();
