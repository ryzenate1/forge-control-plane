import fs from "node:fs";
import path from "node:path";

const ROOT = "forge/web";
const reg = fs.readFileSync(`${ROOT}/components/admin/admin-registry.ts`, "utf8");

// href -> { label, description }
const entries = new Map();
for (const m of reg.matchAll(/\{\s*label:\s*"((?:[^"\\]|\\.)*)",[\s\S]{0,400}?href:\s*"([^"]+)"/g)) {
  const label = m[1].replace(/\\"/g, '"');
  const href = m[2];
  const dMatch = reg.slice(m.index, m.index + 600).match(/description:\s*"((?:[^"\\]|\\.)*)"/);
  entries.set(href, { label, description: dMatch ? dMatch[1] : "" });
}

function routeFor(file) {
  const rel = path.relative(`${ROOT}/app`, path.dirname(file));
  return "/" + rel.split(path.sep).join("/").replace(/\/\[[^\]]+\]/g, "");
}

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p, out);
    else if (e.name.endsWith(".tsx")) out.push(p);
  }
  return out;
}

const files = [...walk(`${ROOT}/app/admin`), ...walk(`${ROOT}/components/admin`)];
const HDR = /<(SectionHeader|AdminPageHeader)\b([\s\S]{0,900}?)\/>/g;
const rows = [];

for (const file of files) {
  const src = fs.readFileSync(file, "utf8");
  const route = file.startsWith(`${ROOT}/app`) ? routeFor(file) : null;
  let m;
  while ((m = HDR.exec(src))) {
    const body = m[2];
    const line = src.slice(0, m.index).split("\n").length;
    const t = body.match(/\btitle=\{?"((?:[^"\\]|\\.)*)"/);
    const tn = body.match(/\btitle=\{([^}]{1,60})\}/);
    const s = body.match(/\b(?:sub|description)=\{?"((?:[^"\\]|\\.)*)"/);
    const hasInfo = /\binfo=/.test(body);
    const hasIcon = /\bicon=\{?(?:null|"[^"]*")|\bicon=\{[A-Za-z]/.test(body);
    rows.push({
      file: file.replace(`${ROOT}/`, ""),
      line,
      route,
      title: t ? t[1] : tn ? `<expr:${tn[1].trim()}>` : null,
      sub: s ? s[1] : null,
      hasInfo,
      hasIcon,
    });
  }
  HDR.lastIndex = 0;
}

let matched = 0, mismatched = 0, derived = 0, unrouted = 0;
const drift = [];
for (const r of rows) {
  const entry = r.route ? entries.get(r.route) : null;
  if (!r.title) { derived++; continue; }
  if (!entry) { unrouted++; continue; }
  if (r.title === entry.label) matched++;
  else { mismatched++; drift.push({ ...r, expected: entry.label }); }
}

console.log(`header call sites: ${rows.length}`);
console.log(`  title omitted (now derived)  : ${derived}`);
console.log(`  title == registry label      : ${matched}`);
console.log(`  title != registry label DRIFT: ${mismatched}`);
console.log(`  route not in registry        : ${unrouted} (detail/wizard, expected)`);
console.log(`\npassing info= : ${rows.filter(r => r.hasInfo).length}`);
console.log(`passing icon= : ${rows.filter(r => r.hasIcon).length}`);
console.log("\n### DRIFT (hand title != sidebar label)");
for (const d of drift) console.log(`  ${d.file}:${d.line}  route=${d.route}  passes "${d.title}"  sidebar says "${d.expected}"`);
