"use strict";

const fs = require("fs");
const path = require("path");

async function main() {
  const [execJS, wasmPath, file, format, rate, audioTier, iqTier] = process.argv.slice(2);
  if (!file) {
    console.error("usage: node node_run.js wasm_exec.js decoder.wasm file format [rate] [audioTier] [iqTier]");
    process.exit(2);
  }
  require(path.resolve(execJS));
  const go = new Go();
  const { instance } = await WebAssembly.instantiate(fs.readFileSync(wasmPath), go.importObject);
  go.run(instance);
  const tier = (v) => (v === undefined ? -1 : Number(v));
  let r = dmrgo.open({ format, rate: Number(rate) || 0, audioTier: tier(audioTier), iqTier: tier(iqTier) });
  if (r.error) throw new Error(r.error);
  const data = fs.readFileSync(file);
  const chunk = 1 << 20;
  let out = "";
  let busy = 0;
  for (let off = 0; off < data.length; off += chunk) {
    const t0 = performance.now();
    r = dmrgo.write(new Uint8Array(data.buffer, data.byteOffset + off, Math.min(chunk, data.length - off)));
    busy += performance.now() - t0;
    if (r.error) throw new Error(r.error);
    out += r.events;
  }
  const t0 = performance.now();
  r = dmrgo.flush();
  busy += performance.now() - t0;
  out += r.events;
  process.stdout.write(out);
  process.stderr.write(`${r.seconds.toFixed(3)} s of input in ${(busy / 1000).toFixed(3)} s, real-time factor ${(busy / 1000 / r.seconds).toFixed(4)}\n`);
  process.exit(0);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
