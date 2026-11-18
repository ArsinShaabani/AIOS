/* AIOS: Persian onboarding and help. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, esc, setContent } = A;
  const V = window.AIOSViews;

  V.help = async function () {
    setContent('<div class="page-head"><div><h3>راهنمای شروع سریع</h3>' +
      '<p class="muted small">در سه دقیقه AIOS را راه بینداز.</p></div></div>' +
      '<div class="steps">' +
      '<div class="step"><span class="num">۱</span><div><strong>مدل هوش مصنوعی را وصل کن</strong>' +
      '<p>به «هوش مصنوعی من» برو. ساده‌ترین راه: Ollama روی همین سرور (بدون کلید) یا Groq/Gemini با کلید رایگان. کلید فقط رمزنگاری‌شده روی سرور خودت ذخیره می‌شود.</p>' +
      '<button class="btn sm" id="h-myai">افزودن مدل</button></div></div>' +
      '<div class="step"><span class="num">۲</span><div><strong>کلید تانل بساز</strong>' +
      '<p>در همان صفحه، یک کلید API بساز و سرویس/مدل را انتخاب کن. این کلید را به هر برنامه‌ای (Cursor، OpenCode، ربات، اسکریپت) بده؛ آدرس پایه <span class="mono" dir="ltr">/v1</span> است.</p></div></div>' +
      '<div class="step"><span class="num">۳</span><div><strong>چت کن یا دستگاه وصل کن</strong>' +
      '<p>در چت، مدل را از بالای صفحه انتخاب کن. برای ربات/ESP32 از «Devices» توکن دستگاه بگیر و از MQTT یا HTTP استفاده کن.</p>' +
      '<button class="btn sm secondary" id="h-chat">رفتن به چت</button></div></div></div>' +
      '<div class="grid2" style="margin-top:16px">' +
      '<div class="card"><div class="section-title" style="margin-top:0">سوال‌های رایج</div>' +
      '<p class="small"><strong>کلید من امن است؟</strong><br>بله؛ با AES-GCM رمزنگاری می‌شود و هرگز در پاسخ API یا رابط کاربری نمایش داده نمی‌شود.</p>' +
      '<p class="small"><strong>چرا پاسخ‌ها کوتاه بود؟</strong><br>سقف پاسخ هر سرویس‌دهنده را در «هوش مصنوعی من» تنظیم کن (پیش‌فرض ۸۱۹۲).</p>' +
      '<p class="small"><strong>روی ۲ گیگ رم اجرا می‌شود؟</strong><br>بله؛ برای Ollama مدل کوچک مثل qwen2.5:1.5b یا llama3.2:1b را انتخاب کن.</p></div>' +
      '<div class="card"><div class="section-title" style="margin-top:0">اتصال برنامه‌های بیرونی</div>' +
      '<pre class="mono" dir="ltr">' + esc('OPENAI_BASE_URL=' + location.origin + '/v1\nOPENAI_API_KEY=aios_...\nOPENAI_MODEL=auto') + '</pre>' +
      '<p class="small muted">هر کلید به یک سرویس/مدل قفل می‌شود؛ مدل اصلی و کلید سرویس‌دهنده مخفی می‌ماند.</p></div></div>');
    if ($("#h-myai")) $("#h-myai").addEventListener("click", () => A.navigate("myai"));
    if ($("#h-chat")) $("#h-chat").addEventListener("click", () => A.navigate("chat"));
  };
})();
