/* AIOS: my AI providers (BYOK). */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal } = A;
  const V = window.AIOSViews;
  const PRESETS = [
    { id: "ollama", name: "Ollama (این سرور)", kind: "ollama", base_url: "http://127.0.0.1:11434", model: "qwen2.5:3b" },
    { id: "groq", name: "Groq (رایگان)", kind: "openai", base_url: "https://api.groq.com/openai/v1", model: "llama-3.3-70b-versatile" },
    { id: "gemini", name: "Google Gemini (رایگان)", kind: "gemini", base_url: "https://generativelanguage.googleapis.com/v1beta", model: "gemini-2.0-flash" },
    { id: "openrouter", name: "OpenRouter", kind: "openai", base_url: "https://openrouter.ai/api/v1", model: "meta-llama/llama-3.3-70b-instruct:free" },
    { id: "openai", name: "OpenAI", kind: "openai", base_url: "https://api.openai.com/v1", model: "gpt-4o-mini" },
    { id: "anthropic", name: "Anthropic", kind: "anthropic", base_url: "https://api.anthropic.com", model: "claude-3-5-haiku-latest" },
    { id: "custom", name: "سفارشی (OpenAI-compatible)", kind: "openai", base_url: "", model: "" },
  ];

  function form(p) {
    p = p || {};
    return '<h3>افزودن سرویس‌دهنده هوش مصنوعی</h3>' +
      '<p class="small muted">کلید فقط روی همین سرور و رمزنگاری‌شده ذخیره می‌شود.</p>' +
      '<div class="form-grid"><div class="field"><label>سرویس آماده</label><select id="p-preset">' +
      PRESETS.map((x) => '<option value="' + x.id + '">' + esc(x.name) + "</option>").join("") + "</select></div>" +
      '<div class="field"><label>نام دلخواه</label><input class="input" id="p-name" value="' + esc(p.name || "") + '" placeholder="my-ai"></div>' +
      '<div class="field"><label>نوع</label><select id="p-kind"><option value="openai">OpenAI-compatible</option><option value="anthropic">Anthropic</option><option value="gemini">Gemini</option><option value="ollama">Ollama</option></select></div>' +
      '<div class="field"><label>Base URL</label><input class="input" id="p-base" dir="ltr" value="' + esc(p.base_url || "") + '"></div>' +
      '<div class="field"><label>API Key</label><input class="input" id="p-key" dir="ltr" type="password" placeholder="' + (p.has_api_key ? "ذخیره شده؛ برای تغییر وارد کن" : "برای Ollama خالی بگذار") + '"></div>' +
      '<div class="field"><label>مدل پیش‌فرض</label><input class="input" id="p-model" dir="ltr" value="' + esc(p.default_model || "") + '"></div>' +
      '<div class="field"><label>حداکثر توکن پاسخ</label><input class="input" id="p-maxtok" type="number" value="' + (p.max_tokens || 8192) + '"></div>' +
      '<div class="field"><label>Context window</label><input class="input" id="p-ctx" type="number" value="' + (p.context_window || 32768) + '"></div></div>' +
      '<div class="field"><label>مدل‌ها (هر خط یا کاما)</label><textarea id="p-models" dir="ltr" class="mono">' + esc((p.models || []).join("\n")) + "</textarea></div>" +
      '<div class="row"><button class="btn secondary sm" id="p-discover">دریافت خودکار مدل‌ها</button><span class="small muted" id="p-disc"></span></div>' +
      '<div class="row end"><button class="btn ghost" id="p-cancel">انصراف</button><button class="btn" id="p-save">ذخیره</button></div>';
  }

  function bind(p) {
    $("#p-preset").addEventListener("change", () => {
      const x = PRESETS.find((i) => i.id === $("#p-preset").value);
      if (!x) return;
      $("#p-base").value = x.base_url;
      $("#p-kind").value = x.kind;
      if (!$("#p-name").value) $("#p-name").value = x.id === "custom" ? "" : x.id;
      if (!$("#p-model").value) $("#p-model").value = x.model;
    });
    $("#p-cancel").addEventListener("click", closeModal);
    $("#p-discover").addEventListener("click", async () => {
      $("#p-disc").textContent = "در حال دریافت…";
      try {
        const d = await api("/me/providers/discover", { method: "POST", body: {
          name: $("#p-name").value, kind: $("#p-kind").value, base_url: $("#p-base").value.trim(), api_key: $("#p-key").value.trim() } });
        $("#p-models").value = (d.models || []).join("\n");
        if (!$("#p-model").value && (d.models || []).length) $("#p-model").value = d.models[0];
        $("#p-disc").textContent = (d.models || []).length + " مدل";
      } catch (e) { $("#p-disc").textContent = e.message; }
    });
    $("#p-save").addEventListener("click", async () => {
      const models = $("#p-models").value.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean);
      try {
        await api("/me/providers/" + encodeURIComponent($("#p-name").value.trim().toLowerCase()), { method: "PUT", body: {
          kind: $("#p-kind").value, base_url: $("#p-base").value.trim(), api_key: $("#p-key").value.trim(),
          default_model: $("#p-model").value.trim(), models: models,
          max_tokens: Number($("#p-maxtok").value) || 8192, context_window: Number($("#p-ctx").value) || 32768 } });
        closeModal(); toast("ذخیره شد", "ok"); V.myai();
      } catch (e) { toast(e.message, "bad"); }
    });
  }

  V._myai = { PRESETS: PRESETS, form: form, bind: bind,
    open: function (p) { modal(form(p)); bind(p || {}); } };
})();
