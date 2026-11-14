/* AIOS skills library + agent editor. Loaded after the base agents view. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, setContent, can } = A;
  const V = (window.AIOSViews = window.AIOSViews || {});

  async function load() {
    const [agents, catalog] = await Promise.all([
      api("/agents?all=true").catch(() => ({ items: [], tools: [] })),
      api("/agents/catalog").catch(() => ({ items: [], tools: [] })),
    ]);
    return { agents, catalog };
  }

  function toolBadges(tools) {
    return (tools || []).map((t) => '<span class="pill">' + esc(t) + "</span>").join("");
  }

  function agentCard(a) {
    return '<div class="card agent-card"><div class="top"><span class="name">' + esc(a.name) + "</span>" +
      (a.builtin ? '<span class="pill accent">built-in</span>' : '<span class="pill">custom</span>') +
      (!a.enabled ? '<span class="pill bad">disabled</span>' : "") + "</div>" +
      '<div class="small muted">' + esc(a.description || "") + "</div>" +
      '<div class="models">' + toolBadges(a.tools) + "</div>" +
      '<div class="foot"><button class="btn sm secondary" data-edit="' + esc(a.id) + '">Edit</button>' +
      '<button class="btn sm ghost" data-toggle="' + esc(a.id) + '">' + (a.enabled ? "Disable" : "Enable") + "</button>" +
      (a.builtin ? "" : '<button class="btn sm danger" data-del="' + esc(a.id) + '">Delete</button>') +
      "</div></div>";
  }

  function skillCard(s) {
    return '<div class="card skill-card"><div class="top"><span class="name">' + esc(s.emoji || "✨") + " " + esc(s.name) + "</span>" +
      '<span class="pill">' + esc(s.category || "general") + "</span></div>" +
      '<div class="small muted">' + esc(s.description) + "</div>" +
      '<div class="models">' + toolBadges((s.tools || []).slice(0, 5)) + "</div>" +
      '<div class="foot">' + (s.installed ? '<span class="pill ok">installed</span><button class="btn sm ghost" data-edit="agent_skill_' + esc(s.id) + '">Edit</button>' :
        '<button class="btn sm" data-install="' + esc(s.id) + '">Install skill</button>') + "</div></div>";
  }

  function form(a, tools) {
    a = a || {};
    const selected = new Set(a.tools || []);
    const toolBoxes = (tools || []).map((t) =>
      '<label class="tool-check"><input type="checkbox" value="' + esc(t) + '"' + (selected.has(t) ? " checked" : "") + "> " + esc(t) + "</label>").join("");
    return "<h3>" + (a.id ? "Edit agent" : "New agent") + "</h3>" +
      '<div class="field"><label>Name</label><input id="ag-name" value="' + esc(a.name || "") + '"></div>' +
      '<div class="field"><label>Description</label><input id="ag-desc" value="' + esc(a.description || "") + '"></div>' +
      '<div class="field"><label>System prompt / character</label><textarea id="ag-prompt" rows="5">' + esc(a.system_prompt || "") + "</textarea></div>" +
      '<div class="field"><label>Keywords (comma separated, used for routing)</label><input id="ag-kw" value="' + esc((a.keywords || []).join(", ")) + '"></div>' +
      '<div class="field"><label>Tools</label><div class="tool-grid">' + toolBoxes + "</div></div>" +
      '<div class="row"><label class="switch"><input type="checkbox" id="ag-enabled"' + (a.enabled === false ? "" : " checked") + "> enabled</label>" +
      '<span class="spacer"></span><label class="small muted">priority <input id="ag-pri" type="number" value="' + (a.priority || 50) + '" style="width:70px"></label></div>' +
      '<div class="row end" style="margin-top:14px"><button class="btn ghost" id="ag-cancel">Cancel</button><button class="btn" id="ag-save">Save</button></div>';
  }


  function openForm(a, tools) {
    modal(form(a, tools));
    $("#ag-cancel").addEventListener("click", closeModal);
    $("#ag-save").addEventListener("click", async () => {
      const body = {
        name: $("#ag-name").value.trim(), description: $("#ag-desc").value.trim(),
        system_prompt: $("#ag-prompt").value.trim(),
        keywords: $("#ag-kw").value.split(",").map((s) => s.trim()).filter(Boolean),
        tools: $$(".tool-check input:checked").map((i) => i.value),
        enabled: $("#ag-enabled").checked, priority: Number($("#ag-pri").value) || 50,
      };
      if (!body.name || !body.system_prompt) { toast("Name and system prompt are required", "bad"); return; }
      const editing = a && a.id;
      try {
        await api(editing ? "/agents/" + encodeURIComponent(a.id) : "/agents",
          { method: editing ? "PUT" : "POST", body: editing ? Object.assign({}, a, body, { builtin: false }) : body });
        toast("Agent saved", "ok");
        closeModal();
        V.agents();
      } catch (e) { toast(e.message, "bad"); }
    });
  }

  V.agents = async function () {
    const { agents, catalog } = await load();
    const tools = catalog.tools && catalog.tools.length ? catalog.tools : (agents.tools || []);
    setContent(
      '<div class="page-head"><div><h2>Skills &amp; Agents</h2><div class="muted">Install a skill, then edit its prompt, tools and routing keywords.</div></div>' +
      (can("agents:write") ? '<button class="btn" id="ag-new">+ New agent</button>' : "") + "</div>" +
      '<input class="search" id="ag-q" placeholder="Search skills and agents...">' +
      '<div class="section-title">Skill library</div><div class="grid cards" id="skill-grid">' + (catalog.items || []).map(skillCard).join("") + "</div>" +
      '<div class="section-title">Installed agents</div><div class="grid cards" id="agent-grid">' +
      (agents.items || []).filter((a) => a.enabled || can("agents:write")).map(agentCard).join("") + "</div>");

    if ($("#ag-new")) $("#ag-new").addEventListener("click", () => openForm({}, tools));
    $$("[data-install]").forEach((b) => b.addEventListener("click", async () => {
      b.disabled = true;
      try {
        await api("/agents/install", { method: "POST", body: { id: b.dataset.install } });
        toast("Skill installed", "ok");
        V.agents();
      } catch (e) { b.disabled = false; toast(e.message, "bad"); }
    }));
    const byId = {};
    (agents.items || []).forEach((a) => { byId[a.id] = a; });
    $$("[data-edit]").forEach((b) => b.addEventListener("click", () => openForm(byId[b.dataset.edit] || {}, tools)));
    $$("[data-toggle]").forEach((b) => b.addEventListener("click", async () => {
      const a = byId[b.dataset.toggle];
      if (!a) return;
      try {
        await api("/agents/" + encodeURIComponent(a.id), { method: "PUT",
          body: Object.assign({}, a, { enabled: !a.enabled, builtin: false }) });
        V.agents();
      } catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-del]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("Delete this agent?")) return;
      try {
        await api("/agents/" + encodeURIComponent(b.dataset.del), { method: "DELETE" });
        toast("Deleted", "ok");
        V.agents();
      } catch (e) { toast(e.message, "bad"); }
    }));
    const q = $("#ag-q");
    if (q) q.addEventListener("input", () => {
      const term = q.value.toLowerCase();
      $$("#skill-grid .card, #agent-grid .card").forEach((card) => {
        card.style.display = card.textContent.toLowerCase().includes(term) ? "" : "none";
      });
    });
  };
})();
