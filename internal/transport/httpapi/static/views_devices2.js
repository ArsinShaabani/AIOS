/* AIOS devices: list and commands. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, ago, setContent } = A;
  const V = window.AIOSViews, D = V._dev;

  V.devices = async function () {
    const data = await api("/devices?limit=100");
    const rows = (data.items || []).map((d) =>
      "<tr><td><strong>" + esc(d.name) + "</strong>" + (d.room ? '<div class="small muted">' + esc(d.room) + "</div>" : "") +
      "</td><td>" + esc(d.kind) + '<div class="small muted">' + esc(d.hardware || "") + "</div></td>" +
      "<td>" + esc(d.protocol) + '<div class="small muted">' + esc(D.meta(d, "connection", "")) + "</div></td>" +
      '<td class="small">' + esc(D.meta(d, "tone", "—")) + " / " + esc(D.meta(d, "language", "")) + "</td>" +
      '<td><span class="pill ' + (d.status === "online" ? "ok" : "bad") + '">' + esc(d.status) + "</span></td>" +
      '<td class="muted small">' + esc(ago(d.last_seen_at)) + "</td>" +
      '<td><button class="btn secondary sm" data-cmd="' + esc(d.id) + '">دستور</button> ' +
      '<button class="btn ghost sm" data-edit="' + esc(d.id) + '">ویرایش</button></td></tr>').join("");
    setContent('<div class="page-head"><div><h3>دستگاه‌ها</h3><p class="muted small">ربات، ESP32، رزبری‌پای و خانه هوشمند.</p></div>' +
      '<button class="btn" id="dev-add">+ ثبت دستگاه</button></div>' +
      '<div class="table-wrap"><table><thead><tr><th>نام</th><th>نوع</th><th>ارتباط</th><th>لحن</th><th>وضعیت</th><th>آخرین</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="7">هنوز دستگاهی ثبت نشده است.</td></tr>') + "</tbody></table></div>" +
      '<div class="card" style="margin-top:16px"><h3>اتصال سریع</h3><p class="small muted">HTTP: ارسال تلمتری به <span class="mono">/api/v1/device/telemetry</span> با توکن دستگاه. ' +
      'MQTT: <span class="mono">aios/devices/&lt;id&gt;/telemetry</span> و دستور روی <span class="mono">aios/devices/&lt;id&gt;/cmd</span>. ' +
      'ROS2 از rosbridge استفاده می‌کند.</p></div>');
    $("#dev-add").addEventListener("click", () => D.openForm(null));
    $$("[data-edit]").forEach((b) => b.addEventListener("click", async () => {
      try { D.openForm(await api("/devices/" + b.dataset.edit)); } catch (e) { toast(e.message, "bad"); }
    }));
    $$("[data-cmd]").forEach((el) => el.addEventListener("click", async () => {
      const dev = await api("/devices/" + el.dataset.cmd);
      const caps = (dev.capabilities || []).map((c) => "<option>" + esc(c.name) + "</option>").join("");
      modal("<h3>" + esc(dev.name) + '</h3><div class="field"><label>قابلیت</label><select id="c-cap">' + caps +
        '</select></div><div class="field"><label>مقدار</label><input class="input" id="c-val"></div>' +
        '<div class="row end"><button class="btn ghost" id="c-cancel">انصراف</button><button class="btn" id="c-send">ارسال</button></div>');
      $("#c-cancel").addEventListener("click", closeModal);
      $("#c-send").addEventListener("click", async () => {
        let v = $("#c-val").value;
        if (v === "true") v = true; else if (v === "false") v = false; else if (v !== "" && !isNaN(Number(v))) v = Number(v);
        try {
          const res = await api("/devices/" + dev.id + "/command", { method: "POST", body: { capability: $("#c-cap").value, value: v } });
          closeModal(); toast(res.result.ok ? "دستور ارسال شد" : "دستور ناموفق بود", res.result.ok ? "ok" : "bad");
        } catch (e) { toast(e.message, "bad"); }
      });
    }));
  };
})();
