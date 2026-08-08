#!/usr/bin/env node
/**
 * Locale sync utility.
 *
 * Diffs every locale against lang/en.json (the reference) and completes
 * missing keys by copying the English value. Keys present in a locale but
 * absent from en.json (orphans) are removed. Existing translations are
 * preserved and key order follows en.json for readability.
 *
 * Usage: node scripts/sync-locales.mjs [--dry-run]
 */
import { readFileSync, writeFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const LANG_DIR = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "lang");
const REFERENCE = "en";
const DRY_RUN = process.argv.includes("--dry-run");

function load(file) {
  return JSON.parse(readFileSync(join(LANG_DIR, file), "utf8"));
}

function countKeys(obj) {
  let n = 0;
  const walk = (o) => {
    for (const v of Object.values(o)) {
      if (v && typeof v === "object" && !Array.isArray(v)) walk(v);
      else n += 1;
    }
  };
  walk(obj);
  return n;
}

/**
 * Rebuild `target` to match `template`'s structure and key order, keeping
 * target's own value when it exists, otherwise falling back to template's.
 */
function mergeInto(template, target) {
  const out = {};
  for (const [key, value] of Object.entries(template)) {
    if (value && typeof value === "object" && !Array.isArray(value)) {
      const next = target?.[key];
      out[key] = next && typeof next === "object" && !Array.isArray(next)
        ? mergeInto(value, next)
        : mergeInto(value, {});
    } else {
      out[key] = target?.[key] ?? value;
    }
  }
  return out;
}

function diff(a, b) {
  const missing = [];
  const orphans = [];
  const walkA = (obj, prefix) => {
    for (const [key, value] of Object.entries(obj)) {
      const path = prefix ? `${prefix}.${key}` : key;
      if (value && typeof value === "object" && !Array.isArray(value)) {
        walkA(value, path);
      } else if (!resolve(b, path)) {
        missing.push(path);
      }
    }
  };
  const walkB = (obj, prefix) => {
    for (const [key, value] of Object.entries(obj)) {
      const path = prefix ? `${prefix}.${key}` : key;
      if (value && typeof value === "object" && !Array.isArray(value)) {
        walkB(value, path);
      } else if (!resolve(a, path)) {
        orphans.push(path);
      }
    }
  };
  walkA(a, "");
  walkB(b, "");
  return { missing, orphans };
}

function resolve(obj, path) {
  let current = obj;
  for (const segment of path.split(".")) {
    if (!current || typeof current !== "object" || !(segment in current)) return undefined;
    current = current[segment];
  }
  return current;
}

const reference = load(`${REFERENCE}.json`);
const files = readdirSync(LANG_DIR).filter((f) => f.endsWith(".json") && f !== `${REFERENCE}.json`);

let totalBefore = 0;
let totalAfter = 0;

for (const file of files.sort()) {
  const locale = file.replace(/\.json$/, "");
  const messages = load(file);
  const { missing, orphans } = diff(reference, messages);

  const merged = mergeInto(reference, messages);
  const { missing: stillMissing, orphans: stillOrphans } = diff(reference, merged);

  totalBefore += missing.length;
  totalAfter += stillMissing.length;

  const summary = [
    `${locale}: ${missing.length} missing -> ${stillMissing.length}`,
    orphans.length ? `${orphans.length} orphans removed` : "",
    stillOrphans.length ? `${stillOrphans.length} orphans remaining` : "",
  ].filter(Boolean).join(", ");

  console.log(`[${DRY_RUN ? "dry-run" : "synced"}] ${summary}`);

  if (!DRY_RUN && (missing.length || orphans.length)) {
    writeFileSync(join(LANG_DIR, file), `${JSON.stringify(merged, null, 2)}\n`);
  }
}

console.log(`\nTotal missing keys: ${totalBefore} -> ${totalAfter}`);
