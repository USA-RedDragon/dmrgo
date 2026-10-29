"use strict";

const $ = (id) => document.getElementById(id);
let fileBytes = null;
let result = null;
let current = null;

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

function setStatus(text, isErr) {
  const s = $("status");
  s.textContent = text;
  s.className = isErr ? "err" : "";
}

window.dmrgoReady = () => {
  $("go").disabled = false;
  setStatus("Ready. Paste hex or drop a capture.");
  loadFromURL();
};

async function loadFromURL() {
  const q = new URLSearchParams(location.search);
  if (q.get("format")) $("format").value = q.get("format");
  if (q.get("protocol")) $("proto").value = q.get("protocol");
  const src = q.get("src");
  if (!src) return;
  try {
    const r = await fetch(src);
    if (!r.ok) throw new Error(r.status + " " + r.statusText);
    fileBytes = new Uint8Array(await r.arrayBuffer());
    $("hex").placeholder = `${src}: ${fileBytes.length} octets loaded`;
    decode();
    const sel = q.get("select");
    if (sel !== null) {
      select(Number(sel));
      const path = q.get("field");
      if (path) clickField(path.split("/"));
    }
  } catch (e) {
    setStatus("Could not load " + src + ": " + e, true);
  }
  document.body.dataset.done = "1";
}

function clickField(path) {
  let scope = document.querySelector("#detail > ul.tree");
  let label = null;
  for (const name of path) {
    label = null;
    for (const li of scope ? scope.children : []) {
      const l = li.children[1];
      if (l && l.firstChild && l.firstChild.textContent === name) {
        label = l;
        scope = li.querySelector(":scope > ul.tree");
        break;
      }
    }
    if (!label) return;
  }
  label.click();
}

async function boot() {
  const go = new Go();
  try {
    const r = await WebAssembly.instantiateStreaming(fetch("inspector.wasm"), go.importObject);
    go.run(r.instance);
  } catch (e) {
    setStatus("Could not load inspector.wasm: " + e, true);
  }
}

function decode() {
  let input;
  if (fileBytes) {
    input = fileBytes;
  } else {
    input = new TextEncoder().encode($("hex").value);
  }
  const json = window.dmrgoInspect(input, $("format").value, $("proto").value);
  result = JSON.parse(json);
  render();
  return result;
}
window.dmrgoDecode = decode;

function render() {
  const list = $("list");
  list.replaceChildren();
  $("detail").replaceChildren();
  const n = (result.packets || []).length;
  const errs = result.errors || [];
  setStatus(`${result.input || ""}: ${n} packets` + (errs.length ? `; ${errs.length} input errors: ${errs.slice(0, 3).join("; ")}` : ""), errs.length > 0 && n === 0);
  (result.packets || []).forEach((p, i) => {
    const d = el("div");
    d.appendChild(el("span", "", `${p.index} `));
    d.appendChild(el("span", "proto", p.protocol + " "));
    d.appendChild(el("span", p.errors ? "err" : "", p.summary));
    d.title = (p.meta || "") + "\n" + p.summary;
    d.onclick = () => select(i);
    list.appendChild(d);
  });
  if (n > 0) select(0);
}

function select(i) {
  const rows = $("list").children;
  for (const r of rows) r.classList.remove("sel");
  if (rows[i]) rows[i].classList.add("sel");
  current = result.packets[i];
  const det = $("detail");
  det.replaceChildren();
  det.appendChild(el("div", "meta", `#${current.index} ${current.protocol} ${current.meta || ""}`));
  for (const e of current.errors || []) det.appendChild(el("div", "err", "Error: " + e));
  const ul = el("ul", "tree");
  for (const n of current.tree || []) ul.appendChild(treeNode(n, 0));
  det.appendChild(ul);
  const hexes = el("div");
  hexes.id = "hexes";
  current.buffers.forEach((b, bi) => hexes.appendChild(hexView(b, bi)));
  det.appendChild(hexes);
}

function bitText(rs) {
  return rs.map((r) => {
    const bytes = r.e - r.s;
    if (r.s % 8 === 0 && bytes % 8 === 0) return `buf ${r.b} octets ${r.s / 8}-${r.e / 8 - 1}`;
    return `buf ${r.b} bits ${r.s}-${r.e - 1}`;
  }).join(", ");
}

function treeNode(n, depth) {
  const li = el("li");
  const kids = n.c || [];
  const t = el("span", "t", kids.length ? "-" : "");
  t.onclick = () => {
    li.classList.toggle("closed");
    t.textContent = li.classList.contains("closed") ? "+" : "-";
  };
  li.appendChild(t);
  const label = el("span");
  label.appendChild(document.createTextNode(n.n));
  if (n.v) {
    label.appendChild(document.createTextNode(" = "));
    label.appendChild(el("span", "v", n.v.length > 140 ? n.v.slice(0, 140) + "..." : n.v));
  }
  if (n.r) label.appendChild(el("span", "b", "  [" + bitText(n.r) + "]"));
  if (n.d) {
    li.classList.add("derived");
    const tag = el("span", "tag", "derived");
    tag.title = "Computed from other bits, not sent as its own field";
    label.appendChild(tag);
  }
  if (n.err) label.appendChild(el("span", "err", "  error: " + n.err));
  label.title = n.v || "";
  label.onclick = () => {
    for (const s of document.querySelectorAll("ul.tree span.sel")) s.classList.remove("sel");
    label.classList.add("sel");
    highlight(n.r || []);
  };
  li.appendChild(label);
  if (kids.length) {
    const ul = el("ul", "tree");
    for (const c of kids) ul.appendChild(treeNode(c, depth + 1));
    li.appendChild(ul);
    if (depth >= 2) {
      li.classList.add("closed");
      t.textContent = "+";
    }
  }
  return li;
}

function hexView(b, bi) {
  const box = el("div", "hex");
  box.dataset.buf = bi;
  const len = b.hex.length / 2;
  box.appendChild(el("div", "name", `buffer ${bi}: ${b.name}, ${len} octets` + (b.map ? `, from buffer ${b.parent}` : "")));
  for (let off = 0; off < len; off += 16) {
    const row = el("div", "row");
    row.appendChild(el("span", "o", off.toString(16).padStart(4, "0") + "  "));
    for (let i = off; i < Math.min(off + 16, len); i++) {
      const s = el("span", "", b.hex.substr(i * 2, 2));
      s.dataset.i = i;
      row.appendChild(s);
      row.appendChild(document.createTextNode(i % 8 === 7 ? "  " : " "));
    }
    box.appendChild(row);
  }
  return box;
}

function highlight(ranges) {
  for (const s of document.querySelectorAll(".hex span.h, .hex span.h2")) s.classList.remove("h", "h2");
  const mark = (bi, i, cls) => {
    const box = document.querySelector(`#hexes .hex[data-buf="${bi}"]`);
    const s = box && box.querySelector(`span[data-i="${i}"]`);
    if (s) s.classList.add(cls);
  };
  let first = null;
  for (const r of ranges) {
    const b = current.buffers[r.b];
    for (let i = Math.floor(r.s / 8); i < Math.ceil(r.e / 8); i++) {
      mark(r.b, i, "h");
      if (b && b.map && b.map[i] !== undefined) mark(b.parent, b.map[i], "h2");
    }
    if (!first) first = document.querySelector(`#hexes .hex[data-buf="${r.b}"]`);
  }
  if (first) first.scrollIntoView({ block: "nearest" });
}

$("go").onclick = decode;
$("hex").oninput = () => { fileBytes = null; };
$("file").onchange = async (e) => {
  const f = e.target.files[0];
  if (!f) return;
  await loadFile(f);
};

async function loadFile(f) {
  fileBytes = new Uint8Array(await f.arrayBuffer());
  $("hex").value = "";
  $("hex").placeholder = `${f.name}: ${fileBytes.length} octets loaded`;
  if (!$("go").disabled) decode();
}

const drop = $("drop");
drop.addEventListener("dragover", (e) => { e.preventDefault(); drop.classList.add("over"); });
drop.addEventListener("dragleave", () => drop.classList.remove("over"));
drop.addEventListener("drop", (e) => {
  e.preventDefault();
  drop.classList.remove("over");
  if (e.dataTransfer.files.length) loadFile(e.dataTransfer.files[0]);
});

boot();
