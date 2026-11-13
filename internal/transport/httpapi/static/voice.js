/* AIOS dashboard: realtime voice loop (mic -> VAD/STT -> brain -> TTS playback). */
(function () {
  "use strict";
  const A = window.AIOS;
  let ws = null, stream = null, ctx = null, processor = null, source = null, active = false;
  let playQueue = [], playing = false;
  let handlers = {};

  function floatToPCM16(f32) {
    const out = new Int16Array(f32.length);
    for (let i = 0; i < f32.length; i++) {
      let s = Math.max(-1, Math.min(1, f32[i]));
      out[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
    }
    return out;
  }
  function playNext() {
    if (playing || !playQueue.length) return;
    playing = true;
    const blob = playQueue.shift();
    const url = URL.createObjectURL(blob);
    const audio = new Audio(url);
    audio.onended = audio.onerror = function () {
      URL.revokeObjectURL(url);
      playing = false;
      playNext();
    };
    audio.play().catch(function () { playing = false; });
  }
  function stopPlayback() {
    playQueue = [];
    playing = false;
  }

  async function start() {
    if (active) return;
    try {
      stream = await navigator.mediaDevices.getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
      });
    } catch (e) {
      handlers.onError && handlers.onError("Microphone permission denied");
      return;
    }
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const url = proto + "//" + location.host + "/api/v1/ws/voice?token=" + encodeURIComponent(A.state.token);
    ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    ws.onmessage = function (ev) {
      if (typeof ev.data !== "string") {
        playQueue.push(new Blob([ev.data]));
        playNext();
        return;
      }
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      switch (msg.type) {
        case "ready": startCapture(); break;
        case "transcript": handlers.onTranscript && handlers.onTranscript(msg.text); break;
        case "reply":
          handlers.onReply && handlers.onReply(msg.text);
          if (msg.session_id) handlers.onSession && handlers.onSession(msg.session_id);
          break;
        case "error": handlers.onError && handlers.onError(msg.stage + ": " + msg.error); break;
        case "wake_needed": handlers.onError && handlers.onError("Wake word required: “hey aios …”"); break;
      }
    };
    ws.onerror = function () { handlers.onError && handlers.onError("Voice connection failed"); };
    ws.onclose = function () { stop(); };
    handlers.onState && handlers.onState(true);
  }

  function startCapture() {
    const rate = 16000;
    try { ctx = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: rate }); }
    catch (e) { ctx = new (window.AudioContext || window.webkitAudioContext)(); }
    source = ctx.createMediaStreamSource(stream);
    processor = ctx.createScriptProcessor(4096, 1, 1);
    processor.onaudioprocess = function (e) {
      if (!ws || ws.readyState !== 1) return;
      const data = e.inputBuffer.getChannelData(0);
      ws.send(floatToPCM16(data).buffer);
    };
    source.connect(processor);
    processor.connect(ctx.destination);
  }

  function stop() {
    active = false;
    if (processor) { processor.disconnect(); processor = null; }
    if (source) { source.disconnect(); source = null; }
    if (ctx) { ctx.close().catch(function () {}); ctx = null; }
    if (stream) { stream.getTracks().forEach(function (t) { t.stop(); }); stream = null; }
    if (ws) { try { ws.send(JSON.stringify({ type: "interrupt" })); } catch (e) {} ws.close(); ws = null; }
    stopPlayback();
    handlers.onState && handlers.onState(false);
  }

  function toggle(opts) {
    handlers = opts || {};
    if (active) { stop(); return; }
    active = true;
    start();
  }

  window.AIOSVoice = { toggle, stop, interrupt: function () { if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: "interrupt" })); stopPlayback(); } };
})();
