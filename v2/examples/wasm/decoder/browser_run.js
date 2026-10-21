"use strict";

const fs = require("fs");

async function main() {
  const [port, url, expr, shot, timeoutS] = process.argv.slice(2);
  if (!url) {
    console.error("usage: node browser_run.js debugPort pageURL [expression] [screenshot.png] [timeout s]");
    process.exit(2);
  }
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent(url)}`, { method: "PUT" })).json();
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((r, j) => {
    ws.onopen = r;
    ws.onerror = j;
  });
  let id = 0;
  const waiting = new Map();
  ws.onmessage = (m) => {
    const msg = JSON.parse(m.data);
    if (msg.method === "Runtime.consoleAPICalled") console.error("console:", msg.params.args.map((a) => a.value ?? a.description).join(" "));
    if (msg.method === "Runtime.exceptionThrown") console.error("exception:", JSON.stringify(msg.params.exceptionDetails).slice(0, 500));
    const p = waiting.get(msg.id);
    if (p) {
      waiting.delete(msg.id);
      p(msg);
    }
  };
  const send = (method, params = {}) =>
    new Promise((r) => {
      const n = ++id;
      waiting.set(n, r);
      ws.send(JSON.stringify({ id: n, method, params }));
    });
  await send("Runtime.enable");
  await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 1000, deviceScaleFactor: 1, mobile: false });
  const wait = expr || "new Promise((r) => { const f = () => (window.result ? window.result.then((x) => r(JSON.stringify(x)), (e) => r('error: ' + e.message)) : setTimeout(f, 100)); f(); })";
  const res = await Promise.race([
    send("Runtime.evaluate", { expression: wait, awaitPromise: true, returnByValue: true }),
    new Promise((r) => setTimeout(() => r({ timeout: true }), 1000 * (Number(timeoutS) || 120))),
  ]);
  if (res.timeout) console.error("timed out");
  else if (res.result.exceptionDetails) console.error(JSON.stringify(res.result.exceptionDetails));
  else console.log(res.result.result.value);
  if (shot) {
    const s = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: false });
    fs.writeFileSync(shot, Buffer.from(s.result.data, "base64"));
  }
  await fetch(`http://127.0.0.1:${port}/json/close/${target.id}`);
  ws.close();
  process.exit(res.timeout ? 1 : 0);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
