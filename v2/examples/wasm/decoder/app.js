"use strict";

const $ = (id) => document.getElementById(id);
const worker = new Worker("worker.js");
const pending = new Map();
let nextID = 1;
let busyMS = 0;
let inputSeconds = 0;
let shown = 0;
const calls = {};
const slotStats = {};

worker.onmessage = (e) => {
  const r = e.data;
  busyMS += r.ms;
  if (r.seconds) inputSeconds = r.seconds;
  if (r.events) r.events.split("\n").forEach((l) => l && render(JSON.parse(l)));
  if (r.error) status("Error: " + r.error);
  const p = pending.get(r.id);
  pending.delete(r.id);
  if (p) p(r);
};

function call(cmd, extra, transfer) {
  const id = nextID++;
  return new Promise((resolve) => {
    pending.set(id, resolve);
    worker.postMessage({ id, cmd, ...extra }, transfer || []);
  });
}

function status(s) {
  $("status").textContent = s;
}

function rtf() {
  return inputSeconds > 0 ? busyMS / 1000 / inputSeconds : 0;
}

const extFormats = { wav: "wav", cu8: "cu8", cs8: "cs8", cs16: "cs16", cf32: "cf32", fc32: "cf32", cfile: "cf32", raw: "cu8", bin: "cu8" };

function formatOf(name) {
  const ext = name.toLowerCase().split(".").pop();
  return extFormats[ext] || "cu8";
}

async function open(format, rate) {
  busyMS = 0;
  inputSeconds = 0;
  const opt = { format, rate, audioTier: Number($("audioTier").value), iqTier: Number($("iqTier").value) };
  const r = await call("open", { opt });
  if (r.error) throw new Error(r.error);
}

async function decodeBlob(blob, format, rate) {
  await open(format, rate);
  const chunk = 1 << 20;
  const t0 = performance.now();
  for (let off = 0; off < blob.size; off += chunk) {
    const buf = await blob.slice(off, off + chunk).arrayBuffer();
    const r = await call("write", { buf }, [buf]);
    if (r.error) throw new Error(r.error);
    status(`Decoding: ${Math.round((100 * (off + buf.byteLength)) / blob.size)}%, ${inputSeconds.toFixed(1)} s of input, real-time factor ${rtf().toFixed(4)}`);
  }
  const r = await call("flush", {});
  const wall = (performance.now() - t0) / 1000;
  const res = { seconds: inputSeconds, rtf: rtf(), wallRTF: wall / Math.max(inputSeconds, 1e-9), stats: lastStats, calls: { ...calls } };
  status(`Done: ${inputSeconds.toFixed(2)} s of input, decoder real-time factor ${res.rtf.toFixed(4)} (with file reads ${res.wallRTF.toFixed(4)}), ${lastStats ? lastStats.bursts : 0} bursts.`);
  return res;
}

let lastStats = null;

function fmtLC(lc) {
  let s = lc.flco;
  if (lc.call) s = `${lc.call} call ${lc.src} to ${lc.dst}`;
  if (lc.fid) s += ` (FID 0x${lc.fid.toString(16).padStart(2, "0")})`;
  if (lc.privacy) s += ", privacy";
  if (lc.emergency) s += ", emergency";
  return `${lc.from} LC: ${s}`;
}

function details(e) {
  const d = [];
  if (e.lc) d.push(fmtLC(e.lc));
  if (e.voice) d.push(`voice ${e.voice}, ${e.frames} frames`);
  if (e.csbk) d.push(e.csbk);
  if (e.header) d.push(e.header);
  if (e.packet) d.push("packet " + e.packet);
  if (e.sms) d.push(`SMS (${e.sms.format}) ${e.sms.src} to ${e.sms.dst}: "${e.sms.text}"`);
  if (e.gps) d.push(`GPS (${e.gps.from}) ${e.gps.lat.toFixed(5)}, ${e.gps.lon.toFixed(5)}${e.gps.detail ? ", " + e.gps.detail : ""}`);
  if (e.alias) d.push(`talker alias "${e.alias}"`);
  if (e.other) d.push(e.other);
  if (e.error) d.push(e.error);
  return d.join("; ");
}

function render(e) {
  if (e.type === "stats") {
    lastStats = e;
    for (const [k, v] of Object.entries(e.slots)) slotStats[k] = v;
    drawSlots();
    return;
  }
  if (e.type === "burst") {
    const k = String(e.slot || "-");
    const st = (slotStats[k] = slotStats[k] || { bursts: 0, voiceFrames: 0, ambeBER: 0 });
    st.bursts++;
    if (e.frames) st.voiceFrames = e.frames;
    if (e.ambeBits) {
      st.ambeBits = e.ambeBits;
      st.ambeBER = e.ambeBER || 0;
    }
    if (e.lc && e.lc.call) calls[k] = fmtLC(e.lc);
    if (e.alias) calls[k + "alias"] = e.alias;
    if (e.gps) calls[k + "gps"] = `${e.gps.lat.toFixed(5)}, ${e.gps.lon.toFixed(5)}`;
    if (e.sms) calls[k + "sms"] = e.sms.text;
    drawSlots();
    if (e.dataType === "Idle" && !$("idle").checked) return;
  }
  const tr = document.createElement("tr");
  const cells = e.type === "slc"
    ? [e.t.toFixed(3), "", "", "CACH", "Short LC", e.slc, e.fec, ""]
    : [e.t.toFixed(3), e.slot || "-", e.cc >= 0 ? e.cc : "", e.sync || "timed", e.dataType || e.kind, details(e), e.fec, e.ber ? e.ber.toFixed(3) : "0"];
  cells.forEach((c, i) => {
    const td = document.createElement("td");
    td.textContent = c;
    if (i === 5) td.className = "d";
    if (i === 6) td.className = c === "bad" ? "bad" : c === "ok" ? "ok" : "";
    tr.appendChild(td);
  });
  const tb = $("events");
  tb.appendChild(tr);
  if (++shown > 2000) tb.removeChild(tb.firstChild);
}

function drawSlots() {
  const el = $("slots");
  el.textContent = "";
  for (const k of Object.keys(slotStats).sort()) {
    const s = slotStats[k];
    const div = document.createElement("div");
    div.className = "slot";
    const rows = [
      ["bursts", s.bursts],
      ["voice frames", s.voiceFrames],
      ["AMBE BER", s.ambeBits ? `${(100 * s.ambeBER).toFixed(2)} % of ${s.ambeBits} bits` : "-"],
      ["call", calls[k] || "-"],
    ];
    if (calls[k + "alias"]) rows.push(["talker alias", calls[k + "alias"]]);
    if (calls[k + "gps"]) rows.push(["GPS", calls[k + "gps"]]);
    if (calls[k + "sms"]) rows.push(["last SMS", calls[k + "sms"]]);
    const h = document.createElement("h2");
    h.textContent = k === "-" ? "No slot (MS or direct mode)" : "Time slot " + k;
    div.appendChild(h);
    const kv = document.createElement("div");
    kv.className = "kv";
    for (const [a, b] of rows) {
      const x = document.createElement("span");
      x.textContent = a;
      const y = document.createElement("span");
      y.textContent = b;
      kv.append(x, y);
    }
    div.appendChild(kv);
    el.appendChild(div);
  }
}

function clearAll() {
  $("events").textContent = "";
  shown = 0;
  for (const k of Object.keys(slotStats)) delete slotStats[k];
  for (const k of Object.keys(calls)) delete calls[k];
  drawSlots();
}

async function decodeFile(file) {
  clearAll();
  const format = $("format").value || formatOf(file.name);
  try {
    await decodeBlob(file, format, Number($("rate").value));
  } catch (err) {
    status("Error: " + err.message);
  }
}

const drop = $("drop");
drop.addEventListener("dragover", (e) => {
  e.preventDefault();
  drop.classList.add("over");
});
drop.addEventListener("dragleave", () => drop.classList.remove("over"));
drop.addEventListener("drop", (e) => {
  e.preventDefault();
  drop.classList.remove("over");
  if (e.dataTransfer.files[0]) decodeFile(e.dataTransfer.files[0]);
});
$("file").addEventListener("change", (e) => e.target.files[0] && decodeFile(e.target.files[0]));
$("clear").addEventListener("click", clearAll);

let live = null;

async function startMic() {
  const id = $("device").value;
  const stream = await navigator.mediaDevices.getUserMedia({
    audio: { deviceId: id ? { exact: id } : undefined, echoCancellation: false, noiseSuppression: false, autoGainControl: false, channelCount: 1 },
  });
  const ctx = new AudioContext();
  await ctx.audioWorklet.addModule("worklet.js");
  const src = ctx.createMediaStreamSource(stream);
  const tap = new AudioWorkletNode(ctx, "tap");
  clearAll();
  await open("f32", ctx.sampleRate);
  let inflight = 0;
  tap.port.onmessage = (e) => {
    if (inflight > 8) return;
    inflight++;
    call("write", { buf: e.data }, [e.data]).then(() => {
      inflight--;
      status(`Live at ${ctx.sampleRate} Hz: ${inputSeconds.toFixed(1)} s, real-time factor ${rtf().toFixed(4)}`);
    });
  };
  src.connect(tap);
  live = { stream, ctx };
  $("mic").textContent = "Stop";
  $("mic").classList.add("on");
  const devs = await navigator.mediaDevices.enumerateDevices();
  const sel = $("device");
  if (sel.options.length === 1) {
    for (const d of devs.filter((d) => d.kind === "audioinput")) {
      const o = document.createElement("option");
      o.value = d.deviceId;
      o.textContent = d.label || d.deviceId;
      sel.appendChild(o);
    }
  }
}

async function stopMic() {
  live.stream.getTracks().forEach((t) => t.stop());
  await live.ctx.close();
  live = null;
  await call("flush", {});
  $("mic").textContent = "Start microphone or line-in";
  $("mic").classList.remove("on");
}

$("mic").addEventListener("click", async () => {
  try {
    if (live) await stopMic();
    else await startMic();
  } catch (err) {
    status("Error: " + err.message);
  }
});

let rtl = null;

async function startRTL() {
  if (typeof navigator.usb === "undefined") throw new Error("this browser has no WebUSB; use Chrome or Edge on localhost or HTTPS");
  const dev = (await RTL.granted()) || (await RTL.request());
  status("Opening the RTL-SDR.");
  await dev.open();
  const rate = await dev.setRate(Number($("rtlRate").value));
  const hz = Math.round(Number($("freq").value) * 1e6);
  await dev.setFrequency(hz);
  const gain = await dev.setGain(Math.round(Number($("gain").value) * 10));
  clearAll();
  await open("cu8", rate);
  let inflight = 0, dropped = 0, bytes = 0;
  rtl = { dev, rate, hz };
  $("rtl").textContent = "Stop RTL-SDR";
  $("rtl").classList.add("on");
  const t0 = performance.now();
  const desc = `RTL-SDR at ${(hz / 1e6).toFixed(4)} MHz, ${(rate / 1e6).toFixed(4)} Msps, gain ${gain < 0 ? "auto" : (gain / 10).toFixed(1) + " dB"}`;
  status(desc);
  dev.start((buf) => {
    bytes += buf.byteLength;
    if (inflight > 16) {
      dropped += buf.byteLength;
      return;
    }
    inflight++;
    call("write", { buf }, [buf]).then(() => {
      inflight--;
      const wall = (performance.now() - t0) / 1000;
      status(`${desc}: ${wall.toFixed(1)} s, ${(bytes / 2 / rate).toFixed(1)} s of samples, real-time factor ${rtf().toFixed(4)}, ${dropped} bytes dropped`);
      window.rtlStats = { wall, samplesSeconds: bytes / 2 / rate, rtf: rtf(), dropped, calls: { ...calls }, stats: lastStats };
    });
  }).catch((err) => {
    if (rtl) status("Error: " + err.message);
  });
}

async function stopRTL() {
  const d = rtl.dev;
  rtl = null;
  await d.stop();
  await call("flush", {});
  $("rtl").textContent = "Start RTL-SDR (WebUSB)";
  $("rtl").classList.remove("on");
}

$("rtl").addEventListener("click", async () => {
  try {
    if (rtl) await stopRTL();
    else await startRTL();
  } catch (err) {
    status("Error: " + err.message);
  }
});

window.decodeURL = async (url, format, rate) => {
  clearAll();
  const blob = await (await fetch(url)).blob();
  return decodeBlob(blob, format || formatOf(url), rate);
};

call("stats", {}).then(() => {
  status("Ready. Start the microphone or drop a file.");
  const q = new URLSearchParams(location.search);
  if (q.get("src")) {
    if (q.get("audioTier")) $("audioTier").value = q.get("audioTier");
    if (q.get("iqTier")) $("iqTier").value = q.get("iqTier");
    window.result = window.decodeURL(q.get("src"), q.get("format"), Number(q.get("rate")) || 0);
  }
});
