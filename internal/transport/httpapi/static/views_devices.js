/* AIOS devices: richer registration form. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, esc, api, toast, modal, closeModal } = A;
  const V = window.AIOSViews;
  const KINDS = ["sensor", "light", "switch", "robot", "esp32", "raspberry_pi", "ros2", "camera", "speaker", "other"];
  const PROTOS = ["mqtt", "http", "websocket", "rosbridge", "tcp", "home_assistant"];
  const CONNS = ["wifi", "ethernet", "mqtt", "websocket", "http", "serial/usb", "ros2", "zigbee", "matter"];
  const TONES = ["دوستانه", "رسمی", "کوتاه", "بازیگوش", "آرام"];
  function meta(d, k, def) { return (d && d.metadata && d.metadata[k] !== undefined) ? d.metadata[k] : def; }
  function form(d) {
    d = d || {};
    const caps = JSON.stringify(d.capabilities || [{ name: "power", type: "switch" }], null, 0);
    const reqs = (meta(d, "requirements", []) || []).join("\n");
    return '<h3>' + (d.id ? "ویرایش دستگاه" : "ثبت دستگاه جدید") + "</h3>" +
      '<div class="form-grid">' +
      '<div class="field"><label>نام</label><input class="input" id="d-name" value="' + esc(d.name || "") + '"></div>' +
      '<div class="field"><label>نوع</label><select id="d-kind">' + KINDS.map((k) => '<option' + (d.kind === k ? " selected" : "") + ">" + k + "</option>").join("") + "</select></div>" +
      '<div class="field"><label>پروتکل</label><select id="d-proto">' + PROTOS.map((k) => '<option' + (d.protocol === k ? " selected" : "") + ">" + k + "</option>").join("") + "</select></div>" +
      '<div class="field"><label>روش اتصال</label><select id="d-conn">' + CONNS.map((k) => '<option' + (meta(d, "connection", "wifi") === k ? " selected" : "") + ">" + k + "</option>").join("") + "</select></div>" +
      '<div class="field"><label>سخت‌افزار</label><input class="input" id="d-hw" value="' + esc(d.hardware || "") + '" placeholder="ESP32-S3 / Pi 5 / STM32"></div>' +
      '<div class="field"><label>اتاق</label><input class="input" id="d-room" value="' + esc(d.room || "") + '"></div>' +
      '<div class="field"><label>مدل هوش مصنوعی</label><select id="d-model"><option value="">مدل سرور (پیش‌فرض)</option></select></div>' +
      '<div class="field"><label>لحن پاسخ</label><select id="d-tone">' + TONES.map((t) => '<option' + (meta(d, "tone", "") === t ? " selected" : "") + ">" + esc(t) + "</option>").join("") + "</select></div>" +
      '<div class="field"><label>سبک پاسخ</label><select id="d-style"><option value="short">کوتاه</option><option value="natural"' + (meta(d, "style") === "natural" ? " selected" : "") + '>طبیعی</option><option value="detailed"' + (meta(d, "style") === "detailed" ? " selected" : "") + '>مفصل</option></select></div>' +
      '<div class="field"><label>زبان</label><select id="d-lang"><option value="fa">فارسی</option><option value="en">English</option></select></div></div>' +
      '<div class="field"><label>توضیح / کاربرد</label><textarea id="d-desc">' + esc(meta(d, "description", "")) + "</textarea></div>" +
      '<div class="field"><label>نیازها و محدودیت‌ها (هر خط)</label><textarea id="d-reqs">' + esc(reqs) + "</textarea></div>" +
      '<div class="field"><label>قابلیت‌ها (JSON)</label><textarea id="d-caps" class="mono" dir="ltr">' + esc(caps) + "</textarea></div>" +
      '<div class="row end"><button class="btn ghost" id="d-cancel">انصراف</button><button class="btn" id="d-save">' + (d.id ? "ذخیره" : "ثبت و دریافت توکن") + "</button></div>";
  }
  async function openForm(d) {
    modal(form(d));
    try {
      const m = await api("/me/models");
      const sel = $("#d-model");
      (m.items || []).forEach((p) => (p.models || [p.default_model]).forEach((x) => {
        if (!x) return;
        const v = p.provider + "::" + x;
        sel.innerHTML += '<option value="' + esc(v) + '"' + (meta(d, "provider", "") + "::" + meta(d, "model", "") === v ? " selected" : "") + ">" + esc(v) + "</option>";
      }));
    } catch (e) {}
    $("#d-cancel").addEventListener("click", closeModal);
    $("#d-save").addEventListener("click", async () => {
      let caps;
      try { caps = JSON.parse($("#d-caps").value || "[]"); } catch (e) { return toast("JSON قابلیت‌ها نامعتبر است", "bad"); }
      const mp = ($("#d-model").value || "").split("::");
      const metadata = Object.assign({}, (d && d.metadata) || {}, {
        description: $("#d-desc").value.trim(), connection: $("#d-conn").value,
        tone: $("#d-tone").value, style: $("#d-style").value, language: $("#d-lang").value,
        provider: mp[0] || "", model: mp.slice(1).join("::"),
        requirements: $("#d-reqs").value.split("\n").map((s) => s.trim()).filter(Boolean),
      });
      const body = { name: $("#d-name").value.trim(), kind: $("#d-kind").value, protocol: $("#d-proto").value,
        hardware: $("#d-hw").value.trim(), room: $("#d-room").value.trim(), capabilities: caps, metadata: metadata };
      try {
        if (d && d.id) {
          await api("/devices/" + d.id, { method: "PATCH", body: body });
          closeModal(); toast("ذخیره شد", "ok"); V.devices();
          return;
        }
        const res = await api("/devices", { method: "POST", body: body });
        modal('<h3>توکن دستگاه</h3><p class="small muted">فقط یک بار نمایش داده می‌شود.</p>' +
          '<pre class="mono" dir="ltr" style="white-space:pre-wrap">' + esc(res.token) + '</pre>' +
          '<div class="row end"><button class="btn" id="tok-close">باشه</button></div>');
        $("#tok-close").addEventListener("click", () => { closeModal(); V.devices(); });
      } catch (e) { toast(e.message, "bad"); }
    });
  }
  V._dev = { openForm: openForm, meta: meta };
})();
