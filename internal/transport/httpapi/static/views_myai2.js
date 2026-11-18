/* AIOS: tunnel API keys + My AI page. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, setContent } = A;
  const V = window.AIOSViews, X = V._myai;

  async function keys(providers) {
    let list = [];
    try { list = (await api("/auth/api-keys")).items || []; } catch (e) {}
    const models = [];
    providers.forEach((p) => (p.models || [p.default_model]).forEach((m) => { if (m) models.push({ p: p.name, m: m }); }));
    const rows = list.map((k) => "<tr><td><strong>" + esc(k.name) + '</strong><div class="small muted mono">' + esc(k.prefix) +
      '…</div></td><td>' + esc(k.provider || "سرور") + " / " + esc(k.model || "—") + '</td><td class="muted small">' +
      esc((k.last_used_at || "—").slice(0, 16)) + '</td><td><button class="btn ghost sm" data-revoke="' + esc(k.id) + '">حذف</button></td></tr>').join("");
    const options = models.map((x) => '<option value="' + esc(x.p + "::" + x.m) + '">' + esc(x.p + " · " + x.m) + "</option>").join("");
    return '<div class="card"><div class="section-title" style="margin-top:0">کلیدهای API تانل</div>' +
      '<p class="small muted">کلید را به هر برنامه/ایجنتی بده. آدرس پایه: <span class="mono" dir="ltr">' + esc(location.origin) +
      '/v1</span> — کلید اصلی سرویس‌دهنده هرگز به برنامه مقصد نمی‌رسد.</p>' +
      (models.length ? '<div class="row"><select id="k-provider">' + options + '</select>' +
        '<input class="input" id="k-name" placeholder="نام کلید" style="max-width:200px">' +
        '<button class="btn sm" id="k-create">ساخت کلید</button></div>' :
        '<div class="empty small">اول یک سرویس‌دهنده اضافه کن.</div>') +
      '<div class="table-wrap" style="margin-top:12px"><table><thead><tr><th>نام</th><th>سرویس / مدل</th><th>آخرین استفاده</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="4">هنوز کلیدی نساخته‌ای.</td></tr>') + "</tbody></table></div></div>";
  }

  V.myai = async function () {
    const d = await api("/me/providers");
    const providers = d.items || [];
    const cards = providers.map((p) =>
      '<div class="card provider-card"><div class="row"><div><strong>' + esc(p.name) + "</strong>" +
      '<div class="small muted mono" dir="ltr">' + esc(p.kind) + " · " + esc(p.base_url) + "</div></div>" +
      '<span class="pill ' + (p.enabled ? "ok" : "bad") + '">' + (p.enabled ? "فعال" : "خاموش") + "</span></div>" +
      '<div class="small muted">مدل: <span class="mono">' + esc(p.default_model || "—") + "</span> · " +
      esc((p.models || []).length) + " مدل · سقف پاسخ " + esc(p.max_tokens || 8192) + "</div>" +
      '<div class="row" style="margin-top:10px"><button class="btn secondary sm" data-test="' + esc(p.name) + '">تست</button>' +
      '<button class="btn ghost sm" data-edit="' + esc(p.name) + '">ویرایش</button>' +
      '<button class="btn ghost sm" data-del="' + esc(p.name) + '">حذف</button>' +
      '<span class="small muted" data-out="' + esc(p.name) + '"></span></div></div>').join("");
    setContent('<div class="page-head"><div><h3>هوش مصنوعی من</h3>' +
      '<p class="muted small">کلید سرویس‌های خودت را وصل کن و با یک کلید AIOS به هر برنامه‌ای بده.</p></div>' +
      '<button class="btn" id="myai-add">+ افزودن سرویس‌دهنده</button></div>' +
      '<div class="guide"><strong>در سه قدم شروع کن</strong><p>۱) کلید سرویس‌دهنده (Groq، Gemini، OpenRouter یا Ollama محلی) را ذخیره کن. ۲) یک کلید API بساز و مدل را انتخاب کن. ۳) در برنامه مقصد Base URL را <span class="mono" dir="ltr">' + esc(location.origin) + '/v1</span> و کلید AIOS را بگذار.</p></div>' +
      (cards ? '<div class="grid2">' + cards + "</div>" : '<div class="empty">هنوز سرویس‌دهنده‌ای نداری.</div>') +
      await keys(providers));
    $("#myai-add").addEventListener("click", () => X.open(null));
    $$("[data-edit]").forEach((b) => b.addEventListener("click", () => X.open(providers.find((p) => p.name === b.dataset.edit))));
    $$("[data-del]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("حذف شود؟")) return;
      try { await api("/me/providers/" + encodeURIComponent(b.dataset.del), { method: "DELETE" }); V.myai(); } catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-test]").forEach((b) => b.addEventListener("click", async () => {
      const out = $("[data-out='" + b.dataset.test + "']");
      out.textContent = "…";
      try { const r = await api("/me/providers/" + encodeURIComponent(b.dataset.test) + "/test", { method: "POST" }); out.textContent = "✔ " + r.latency_ms + "ms"; }
      catch (e) { out.textContent = "✖ " + e.message; }
    }));
    if ($("#k-create")) $("#k-create").addEventListener("click", async () => {
      const parts = $("#k-provider").value.split("::");
      try {
        const r = await api("/auth/api-keys", { method: "POST", body: { name: $("#k-name").value || "tunnel", provider: parts[0], model: parts.slice(1).join("::") } });
        modal('<h3>کلید API شما</h3><p class="small muted">فقط یک بار نمایش داده می‌شود.</p><pre class="mono" dir="ltr">' + esc(r.token) + '</pre><div class="row end"><button class="btn" id="k-done">کپی کردم</button></div>');
        $("#k-done").addEventListener("click", () => { closeModal(); V.myai(); });
      } catch (e) { toast(e.message, "bad"); }
    });
    $$("[data-revoke]").forEach((b) => b.addEventListener("click", async () => {
      if (!confirm("کلید حذف شود؟")) return;
      try { await api("/auth/api-keys/" + encodeURIComponent(b.dataset.revoke), { method: "DELETE" }); V.myai(); } catch (e) { toast(e.message, "bad"); }
    }));
  };
})();
