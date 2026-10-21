importScripts("wasm_exec.js");

const go = new Go();
const ready = WebAssembly.instantiateStreaming(fetch("decoder.wasm"), go.importObject).then((r) => {
  go.run(r.instance);
});

onmessage = async (e) => {
  await ready;
  const m = e.data;
  const t0 = performance.now();
  let r;
  switch (m.cmd) {
    case "open":
      r = dmrgo.open(m.opt);
      break;
    case "write":
      r = dmrgo.write(new Uint8Array(m.buf));
      break;
    case "flush":
      r = dmrgo.flush();
      break;
    case "stats":
      r = dmrgo.stats();
      break;
  }
  postMessage({ id: m.id, cmd: m.cmd, events: r.events || "", error: r.error, seconds: r.seconds || 0, ms: performance.now() - t0 });
};
