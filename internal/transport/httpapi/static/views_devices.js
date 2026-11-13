/* AIOS dashboard: devices. */
(function () {
  "use strict";
  const A = window.AIOS;
  const { $, $$, esc, api, toast, modal, closeModal, ago, setContent } = A;
  const V = window.AIOSViews;
  V.devices = async function () {
    const data = await api("/devices?limit=100");
    const rows = (data.items || []).map((d) =>
      "<tr><td><strong>" + esc(d.name) + "</strong>" + (d.room ? '<div class="small muted">' + esc(d.room) + "</div>" : "") +
      "</td><td>" + esc(d.kind) + "</td><td>" + esc(d.protocol) + '</td><td><span class="pill ' +
      (d.status === "online" ? "ok" : "bad") + '">' + esc(d.status) + "</span></td>" +
      '<td class="muted small">' + esc(ago(d.last_seen_at)) + "</td>" +
      '<td><button class="btn secondary sm" data-cmd="' + esc(d.id) + '">Command</button></td></tr>').join("");
    setContent('<div class="toolbar"><div class="muted small">' + (data.total || 0) +
      ' devices</div><button class="btn sm" id="dev-add">Register device</button></div>' +
      '<div class="table-wrap"><table><thead><tr><th>Name</th><th>Kind</th><th>Protocol</th><th>Status</th><th>Last seen</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="6">No devices yet. Register an ESP32, Raspberry Pi, robot or smart-home node.</td></tr>') +
      "</tbody></table></div>" +
      '<div class="card" style="margin-top:16px"><h3>Integration quick reference</h3>' +
      '<p class="small muted">HTTP firmware: POST telemetry to <span class="mono">/api/v1/device/telemetry</span> with a device bearer token. ' +
      'MQTT: publish <span class="mono">aios/devices/&lt;id&gt;/telemetry</span>, subscribe to <span class="mono">aios/devices/&lt;id&gt;/cmd</span>. ' +
      "ROS2 uses rosbridge; STM32/Arduino use the same MQTT contract.</p></div>");
    $("#dev-add").addEventListener("click", () => {
      modal('<h3>Register device</h3><div class="form-grid">' +
        '<div class="field"><label>Name</label><input class="input" id="d-name"></div>' +
        '<div class="field"><label>Kind</label><select id="d-kind"><option>sensor</option><option>light</option><option>robot</option><option>esp32</option><option>raspberry_pi</option><option>ros2</option></select></div>' +
        '<div class="field"><label>Protocol</label><select id="d-proto"><option>mqtt</option><option>http</option><option>rosbridge</option></select></div>' +
        '<div class="field"><label>Room</label><input class="input" id="d-room"></div></div>' +
        '<div class="field"><label>Capabilities (JSON)</label><textarea id="d-caps" class="mono">[{"name":"power","type":"switch"}]</textarea></div>' +
        '<div class="row end"><button class="btn ghost" id="d-cancel">Cancel</button><button class="btn" id="d-save">Register</button></div>');
      $("#d-cancel").addEventListener("click", closeModal);
      $("#d-save").addEventListener("click", async () => {
        try {
          const res = await api("/devices", { method: "POST", body: {
            name: $("#d-name").value, kind: $("#d-kind").value, protocol: $("#d-proto").value,
            room: $("#d-room").value, capabilities: JSON.parse($("#d-caps").value || "[]"),
          } });
          modal('<h3>Device token</h3><p class="small muted">Copy it now — shown once.</p>' +
            '<pre class="mono" style="white-space:pre-wrap;background:#0c1320;padding:12px;border-radius:8px">' +
            esc(res.token) + '</pre><div class="row end"><button class="btn" id="tok-close">Done</button></div>');
          $("#tok-close").addEventListener("click", () => { closeModal(); A.navigate("devices"); });
        } catch (e) { toast(e.message, "bad"); }
      });
    });
    $$("[data-cmd]").forEach((el) => el.addEventListener("click", async () => {
      const dev = await api("/devices/" + el.dataset.cmd);
      const caps = (dev.capabilities || []).map((c) => '<option>' + esc(c.name) + "</option>").join("");
      modal("<h3>" + esc(dev.name) + '</h3><div class="field"><label>Capability</label><select id="c-cap">' + caps +
        '</select></div><div class="field"><label>Value</label><input class="input" id="c-val"></div>' +
        '<div class="row end"><button class="btn ghost" id="c-cancel">Cancel</button><button class="btn" id="c-send">Send</button></div>');
      $("#c-cancel").addEventListener("click", closeModal);
      $("#c-send").addEventListener("click", async () => {
        let v = $("#c-val").value;
        if (v === "true") v = true; else if (v === "false") v = false; else if (v !== "" && !isNaN(Number(v))) v = Number(v);
        try {
          const res = await api("/devices/" + dev.id + "/command", { method: "POST", body: { capability: $("#c-cap").value, value: v } });
          closeModal(); toast(res.result.ok ? "Command delivered" : "Command failed", res.result.ok ? "ok" : "bad");
        } catch (e) { toast(e.message, "bad"); }
      });
    }));
  };
})();
