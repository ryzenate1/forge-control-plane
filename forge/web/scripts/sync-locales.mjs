#!/usr/bin/env node
/**
 * Locale sync utility.
 *
 * Diffs every locale against lang/en.json (the reference) and completes
 * missing keys by copying the English value. Keys present in a locale but
 * absent from en.json (orphans) are removed. Existing translations are
 * preserved and key order follows en.json for readability.
 *
 * Also verifies that the `lang/*.json` catalog matches the
 * `supportedLocales` declared in `packages/shared-types/src/i18n.ts` and
 * consumed by the `/api/i18n/[locale]` route: a locale file without a
 * declaration (or vice versa) is a mismatch.
 *
 * Usage: node scripts/sync-locales.mjs [--dry-run]
 * Exit code is non-zero in --dry-run when keys are missing/orphaned or the
 * locale list mismatches, so `sync:locales:check` fails CI on drift.
 */
import { readFileSync, writeFileSync, readdirSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const SCRIPTS_DIR = dirname(fileURLToPath(import.meta.url));
const LANG_DIR = join(SCRIPTS_DIR, "..", "..", "..", "lang");
const SHARED_I18N = join(SCRIPTS_DIR, "..", "..", "..", "packages", "shared-types", "src", "i18n.ts");
const REFERENCE = "en";
const DRY_RUN = process.argv.includes("--dry-run");

function load(file) {
  return JSON.parse(readFileSync(join(LANG_DIR, file), "utf8"));
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

function readSupportedLocales() {
  try {
    if (!existsSync(SHARED_I18N)) return null;
    const src = readFileSync(SHARED_I18N, "utf8");
    const match = src.match(/supportedLocales\s*:\s*\[([^\]]*)\]/);
    if (!match) return null;
    return match[1]
      .split(",")
      .map((s) => s.trim().replace(/^['"]|['"]$/g, ""))
      .filter(Boolean);
  } catch {
    return null;
  }
}

const reference = load(`${REFERENCE}.json`);
const files = readdirSync(LANG_DIR).filter((f) => f.endsWith(".json") && f !== `${REFERENCE}.json`);

let totalBefore = 0;
let totalAfter = 0;
let drift = false;

for (const file of files.sort()) {
  const locale = file.replace(/\.json$/, "");
  const messages = load(file);
  const { missing, orphans } = diff(reference, messages);

  const merged = mergeInto(reference, messages);
  const { missing: stillMissing, orphans: stillOrphans } = diff(reference, merged);

  totalBefore += missing.length;
  totalAfter += stillMissing.length;
  if (missing.length || orphans.length) drift = true;

  const summary = [
    `${locale}: ${missing.length} missing -> ${stillMissing.length}`,
    orphans.length ? `${orphans.length} orphans removed` : "",
    stillOrphans.length ? `${stillOrphans.length} orphans remaining` : "",
  ].filter(Boolean).join(", ");

  console.log(`[${DRY_RUN ? "dry-run" : "synced"}] ${summary}`);

  // Missing-key report: list the absent paths so the owner knows exactly
  // which English keys need translation (capped per locale for readability).
  if (DRY_RUN && missing.length) {
    for (const key of missing.slice(0, 20)) {
      console.log(`    missing: ${key}`);
    }
    if (missing.length > 20) {
      console.log(`    ... and ${missing.length - 20} more`);
    }
  }
  if (DRY_RUN && orphans.length) {
    for (const key of orphans.slice(0, 20)) {
      console.log(`    orphan: ${key}`);
    }
    if (orphans.length > 20) {
      console.log(`    ... and ${orphans.length - 20} more`);
    }
  }

  if (!DRY_RUN && (missing.length || orphans.length)) {
    writeFileSync(join(LANG_DIR, file), `${JSON.stringify(merged, null, 2)}\n`);
  }
}

console.log(`\nTotal missing keys: ${totalBefore} -> ${totalAfter}`);

// ---- Locale list check: lang/*.json vs supportedLocales -------------------
const langLocales = readdirSync(LANG_DIR)
  .filter((f) => f.endsWith(".json"))
  .map((f) => f.replace(/\.json$/, ""))
  .sort();
const declared = readSupportedLocales();
if (declared) {
  const declaredSorted = [...declared].sort();
  const missingDecl = langLocales.filter((l) => !declared.includes(l));
  const extraDecl = declared.filter((l) => !langLocales.includes(l));
  console.log(`\nLocales on disk: ${langLocales.join(", ")}`);
  console.log(`Locales declared: ${declaredSorted.join(", ")}`);
  if (missingDecl.length || extraDecl.length) {
    drift = true;
    if (missingDecl.length) console.error(`[mismatch] lang files without supportedLocales entry: ${missingDecl.join(", ")}`);
    if (extraDecl.length) console.error(`[mismatch] supportedLocales without lang file: ${extraDecl.join(", ")}`);
  } else {
    console.log("[ok] lang/ catalog matches supportedLocales");
  }
} else {
  console.log("\n[warn] could not parse supportedLocales from packages/shared-types/src/i18n.ts; skipping list check");
}

if (DRY_RUN && drift) {
  console.error("\n[dry-run] locale drift detected (missing/orphan keys or locale list mismatch)");
  process.exit(1);
}
