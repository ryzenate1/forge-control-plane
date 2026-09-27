import { readFileSync, readdirSync, statSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const templatesDir = join(__dirname, '..', 'templates');
const registryPath = join(__dirname, '..', 'index.json');

const REQUIRED = [
  'id',
  'name',
  'description',
  'version',
  'game',
  'image',
  'startup',
  'config',
  'ports',
  'env',
  'resources',
  'install_script',
  'supported_platforms',
  'categories',
];

// Runtime providers accepted by Forge's ValidateProvider
// (forge/api/internal/runtime): docker/containerd/podman/firecracker/kubernetes
// supported, kvm/lxc behind ENABLE_EXPERIMENTAL_RUNTIMES.
const VALID_PLATFORMS = new Set([
  'docker',
  'containerd',
  'podman',
  'firecracker',
  'kubernetes',
  'kvm',
  'lxc',
]);

// Built-in server variables injected/resolved by the Forge runtime (wings protocol):
// - SERVER_PORT / SERVER_IP / SERVER_MEMORY are injected as environment by forge/api
// - server.build.default.{port,ip,ip_alias} are resolved by the daemon from the
//   server's default allocation
// - any {{VAR}} matching an env variable defined in the template's env[] is
//   replaced with that variable's value at start time
const BUILTIN_VARIABLES = new Set([
  'SERVER_PORT',
  'SERVER_IP',
  'SERVER_MEMORY',
  'SERVER_UUID',
  'P_SERVER_UUID',
  'STARTUP',
  'server.build.default.port',
  'server.build.default.ip',
  'server.build.default.ip_alias',
]);

const VARIABLE_PATTERN = /\{\{([^{}]+)\}\}/g;

// Validation-rule grammar: `required`/`nullable` plus typed constraints.
// Unknown tokens are rejected so a typo (e.g. `requierd`) cannot silently
// disable validation of a variable.
const RULE_TOKENS = [
  { re: /^(required|nullable)$/, name: 'presence' },
  { re: /^(string|integer|boolean|numeric|array)$/, name: 'type' },
  { re: /^min:\d+$/, name: 'min' },
  { re: /^max:\d+$/, name: 'max' },
  { re: /^in:[^|]+$/, name: 'in' },
  { re: /^regex:\/.+\/[a-z]*$/, name: 'regex' },
  { re: /^size:\d+$/, name: 'size' },
];

function parseRules(rules) {
  if (typeof rules !== 'string' || rules.length === 0) {
    return { ok: false, error: 'rules must be a non-empty string' };
  }
  const tokens = rules.split('|');
  for (const token of tokens) {
    if (!RULE_TOKENS.some((t) => t.re.test(token))) {
      return { ok: false, error: `unknown rule token "${token}"` };
    }
  }
  const min = tokens.find((t) => t.startsWith('min:'));
  const max = tokens.find((t) => t.startsWith('max:'));
  if (min && max && parseInt(min.slice(4), 10) > parseInt(max.slice(4), 10)) {
    return { ok: false, error: `min exceeds max in "${rules}"` };
  }
  return { ok: true, tokens };
}

function ruleHas(tokens, name) {
  return tokens.includes(name);
}

function ruleValues(tokens, prefix) {
  const token = tokens.find((t) => t.startsWith(prefix));
  return token ? token.slice(prefix.length).split(',') : null;
}

function ruleNumber(tokens, prefix) {
  const token = tokens.find((t) => t.startsWith(prefix));
  return token ? parseInt(token.slice(prefix.length), 10) : null;
}

// Secret variables are generated randomly server-side at deploy time when left
// empty, so an empty default with `required` rules is valid for them. A
// non-empty weak default (changeme, gamepanel, password, …) is rejected:
// it would ship every deployment with the same guessable credential.
const SECRET_VAR_RE = /(PASSWORD|PASSWD|PASS|SECRET|TOKEN|PRIVATE_KEY)$/;
const WEAK_DEFAULTS = new Set([
  'changeme',
  'change-me',
  'gamepanel',
  'password',
  'passw0rd',
  'admin',
  'administrator',
  'test',
  'testing',
  'default',
  'qwerty',
  '1234',
  '12345',
  '123456',
  'letmein',
]);

function isWeakSecretDefault(value) {
  return WEAK_DEFAULTS.has(String(value).toLowerCase());
}

const errors = [];

function collectPlaceholders(value, out) {
  if (typeof value === 'string') {
    for (const match of value.matchAll(VARIABLE_PATTERN)) {
      out.push(match[1].trim());
    }
    return;
  }
  if (Array.isArray(value)) {
    for (const item of value) collectPlaceholders(item, out);
    return;
  }
  if (value && typeof value === 'object') {
    for (const key of Object.keys(value)) {
      collectPlaceholders(value[key], out);
    }
  }
}

/** Collect {{...}} placeholders only from the startup command and config (the
 * only places the runtime performs substitution). Placeholders anywhere else
 * (install scripts, descriptions, …) never resolve and are rejected. */
function collectAllowedPlaceholders(content) {
  const placeholders = [];
  collectPlaceholders(content.startup, placeholders);
  if (content.config && content.config.files) {
    collectPlaceholders(content.config.files, placeholders);
  }
  return placeholders;
}

/** Find {{...}} placeholders in every string field EXCEPT startup/config. */
function collectDisallowedPlaceholders(content) {
  const found = [];
  function walk(value, path) {
    if (typeof value === 'string') {
      for (const match of value.matchAll(VARIABLE_PATTERN)) {
        found.push({ placeholder: match[1].trim(), path });
      }
      return;
    }
    if (Array.isArray(value)) {
      value.forEach((item, i) => walk(item, `${path}[${i}]`));
      return;
    }
    if (value && typeof value === 'object') {
      for (const key of Object.keys(value)) {
        if (path === '' && (key === 'startup' || key === 'config')) continue;
        walk(value[key], path ? `${path}.${key}` : key);
      }
    }
  }
  walk(content, '');
  return found;
}

/**
 * An interpolation in the startup command is safe only when quoted, so a
 * value containing spaces or shell metacharacters cannot break out into a
 * second command. Numeric (integer-ruled) variables and built-in numeric
 * allocations are exempt; everything else must appear inside a quoted token.
 */
function checkStartupQuoting(file, content, envByName) {
  const startup = content.startup;
  if (typeof startup !== 'string') return;
  const tokens = startup.split(/\s+/);
  for (const token of tokens) {
    for (const match of token.matchAll(VARIABLE_PATTERN)) {
      const name = match[1].trim();
      if (BUILTIN_VARIABLES.has(name)) continue;
      const env = envByName.get(name);
      if (!env) continue; // undefined-variable error is reported elsewhere
      const parsed = parseRules(env.rules);
      if (parsed.ok && ruleHas(parsed.tokens, 'integer')) continue;
      if (!/["']/.test(token)) {
        errors.push(
          `${file}: startup interpolation "{{${name}}}" must be quoted (e.g. '"{{${name}}}"') so values with spaces or shell metacharacters cannot inject commands`,
        );
      }
    }
  }
}

function isValidHttpUri(value) {
  try {
    const url = new URL(value);
    return url.protocol === 'http:' || url.protocol === 'https:';
  } catch {
    return false;
  }
}

const registry = JSON.parse(readFileSync(registryPath, 'utf-8'));
const registeredIds = new Set(Object.keys(registry.registry ?? {}));
const registryCategories = new Set(Object.keys(registry.categories ?? {}));
const templateFiles = new Set(
  readdirSync(templatesDir).filter((f) => f.endsWith('.json') && f !== 'template-schema.json'),
);

// Every registry entry must have a matching template file.
for (const id of registeredIds) {
  if (!templateFiles.has(`${id}.json`)) {
    errors.push(`index.json: id "${id}" has no matching templates/${id}.json file`);
  }
}

for (const file of templateFiles) {
  const filePath = join(templatesDir, file);
  const content = JSON.parse(readFileSync(filePath, 'utf-8'));

  for (const key of REQUIRED) {
    if (!(key in content)) {
      errors.push(`${file}: missing required field "${key}"`);
    }
  }

  const templateId = content.id;
  if (!registeredIds.has(templateId)) {
    errors.push(`${file}: id "${templateId}" not registered in index.json`);
  }

  const expectedFile = `${content.id}.json`;
  if (file !== expectedFile) {
    errors.push(`file "${file}" should be named "${expectedFile}" to match id "${content.id}"`);
  }

  // --- env[] checks: name shape, duplicates, rules syntax, default vs rules ---
  const envByName = new Map();
  if (content.env) {
    const seen = new Set();
    for (const [i, v] of content.env.entries()) {
      if (!/^[A-Z][A-Z0-9_]*$/.test(v.env_variable)) {
        errors.push(
          `${file}: env[${i}] env_variable "${v.env_variable}" must match ^[A-Z][A-Z0-9_]*$`,
        );
      }
      if (seen.has(v.env_variable)) {
        errors.push(`${file}: env[${i}] duplicate env_variable "${v.env_variable}"`);
      }
      seen.add(v.env_variable);
      envByName.set(v.env_variable, v);

      const parsed = parseRules(v.rules);
      if (!parsed.ok) {
        errors.push(
          `${file}: env[${i}] "${v.env_variable}" has invalid rules "${v.rules}": ${parsed.error}`,
        );
        continue;
      }
      const tokens = parsed.tokens;
      const def = v.default_value ?? '';

      // Weak non-empty secrets are rejected; empty secrets are server-generated.
      if (SECRET_VAR_RE.test(v.env_variable)) {
        if (def !== '' && isWeakSecretDefault(def)) {
          errors.push(
            `${file}: env[${i}] "${v.env_variable}" uses weak default "${def}" — leave it empty so the server generates a random credential at deploy time`,
          );
        }
      } else if (def === '' && ruleHas(tokens, 'required') && !ruleHas(tokens, 'nullable')) {
        errors.push(
          `${file}: env[${i}] "${v.env_variable}" has empty default but rules "${v.rules}" require a value — provide a real default or mark it nullable`,
        );
      }

      if (def !== '') {
        if (ruleHas(tokens, 'integer') && !/^-?\d+$/.test(def)) {
          errors.push(
            `${file}: env[${i}] "${v.env_variable}" default "${def}" violates integer rule "${v.rules}"`,
          );
        }
        const allowed = ruleValues(tokens, 'in:');
        if (allowed && !allowed.includes(def)) {
          errors.push(
            `${file}: env[${i}] "${v.env_variable}" default "${def}" is not in [${allowed.join(', ')}] per "${v.rules}"`,
          );
        }
        const maxLen = ruleNumber(tokens, 'max:');
        if (maxLen !== null && ruleHas(tokens, 'string') && def.length > maxLen) {
          errors.push(
            `${file}: env[${i}] "${v.env_variable}" default exceeds max:${maxLen} per "${v.rules}"`,
          );
        }
        const minLen = ruleNumber(tokens, 'min:');
        if (minLen !== null && ruleHas(tokens, 'string') && def.length < minLen) {
          errors.push(
            `${file}: env[${i}] "${v.env_variable}" default shorter than min:${minLen} per "${v.rules}"`,
          );
        }
      }
    }
  }

  // --- placeholders: defined, and only in startup+config ---
  const envVariables = new Set(envByName.keys());
  for (const placeholder of new Set(collectAllowedPlaceholders(content))) {
    if (!BUILTIN_VARIABLES.has(placeholder) && !envVariables.has(placeholder)) {
      errors.push(
        `${file}: placeholder "{{${placeholder}}}" in startup/config is not a defined env variable or built-in server variable`,
      );
    }
  }
  for (const { placeholder, path } of collectDisallowedPlaceholders(content)) {
    errors.push(
      `${file}: placeholder "{{${placeholder}}}" in "${path}" will never resolve — {{...}} substitution runs only in startup and config`,
    );
  }
  checkStartupQuoting(file, content, envByName);

  // --- ports ---
  if (content.ports) {
    for (const [i, p] of content.ports.entries()) {
      if (p.port < 1 || p.port > 65535) {
        errors.push(`${file}: ports[${i}] port ${p.port} out of range`);
      }
      if (!['tcp', 'udp'].includes(p.protocol)) {
        errors.push(`${file}: ports[${i}] protocol must be tcp or udp`);
      }
    }
  }

  // --- resources ranges ---
  const res = content.resources;
  if (res && typeof res === 'object') {
    if (!(res.cpu > 0)) errors.push(`${file}: resources.cpu must be a positive number`);
    if (!(res.memory_mb >= 128)) errors.push(`${file}: resources.memory_mb must be >= 128`);
    if (!(res.disk_mb >= 256)) errors.push(`${file}: resources.disk_mb must be >= 256`);
    if (res.memory_max_mb !== undefined && !(res.memory_max_mb >= res.memory_mb)) {
      errors.push(`${file}: resources.memory_max_mb must be >= memory_mb`);
    }
    if (res.cpu_shares !== undefined && !(res.cpu_shares >= 0 && res.cpu_shares <= 262144)) {
      errors.push(`${file}: resources.cpu_shares must be within 0..262144`);
    }
    if (res.io_weight !== undefined && !(res.io_weight >= 10 && res.io_weight <= 1000)) {
      errors.push(`${file}: resources.io_weight must be within 10..1000`);
    }
    if (res.swap_mb !== undefined && !(res.swap_mb >= 0)) {
      errors.push(`${file}: resources.swap_mb must be >= 0`);
    }
  }

  // --- install_script: present, non-empty, eval-free ---
  const inst = content.install_script;
  if (inst && typeof inst === 'object') {
    for (const key of ['container', 'entrypoint', 'script']) {
      if (typeof inst[key] !== 'string' || inst[key].length === 0) {
        errors.push(`${file}: install_script.${key} must be a non-empty string`);
      }
    }
    if (typeof inst.script === 'string' && /(^|[^A-Za-z0-9_])eval(\s|$)/.test(inst.script)) {
      errors.push(
        `${file}: install_script.script must not invoke eval — expand only allow-listed {{VAR}} placeholders explicitly (see minecraft-paper)`,
      );
    }
  }

  // --- update_url must be an http(s) URI when present ---
  if (
    content.update_url !== undefined &&
    content.update_url !== null &&
    content.update_url !== ''
  ) {
    if (typeof content.update_url !== 'string' || !isValidHttpUri(content.update_url)) {
      errors.push(`${file}: update_url must be a valid http(s) URI`);
    }
  }

  // --- file_denylist must not allow path traversal ---
  if (content.file_denylist) {
    for (const [i, entry] of content.file_denylist.entries()) {
      if (typeof entry !== 'string' || entry.length === 0) {
        errors.push(`${file}: file_denylist[${i}] must be a non-empty string`);
      } else if (entry.startsWith('/') || entry.includes('..')) {
        errors.push(
          `${file}: file_denylist[${i}] "${entry}" must be a relative path without ".." traversal`,
        );
      }
    }
  }

  // --- images: default image real, choices map non-empty ---
  if (typeof content.image !== 'string' || content.image.length === 0) {
    errors.push(`${file}: image must be a non-empty string`);
  }
  if (content.images !== undefined) {
    const entries = Object.entries(content.images);
    if (entries.length === 0) {
      errors.push(`${file}: images must not be an empty map`);
    } else {
      for (const [label, img] of entries) {
        if (typeof img !== 'string' || img.length === 0) {
          errors.push(`${file}: images["${label}"] must be a non-empty string`);
        }
      }
      if (!entries.some(([, img]) => img === content.image)) {
        errors.push(`${file}: image "${content.image}" should be one of the images map values`);
      }
    }
  }

  // --- supported_platforms must be known providers ---
  if (content.supported_platforms) {
    for (const platform of content.supported_platforms) {
      if (!VALID_PLATFORMS.has(platform)) {
        errors.push(
          `${file}: supported_platforms "${platform}" is not a known Forge runtime provider (${[...VALID_PLATFORMS].join(', ')})`,
        );
      }
    }
  }

  // --- categories must exist in index.json ---
  if (content.categories) {
    for (const category of content.categories) {
      if (!registryCategories.has(category)) {
        errors.push(`${file}: category "${category}" is not defined in index.json categories`);
      }
    }
  }
}

if (errors.length > 0) {
  console.error('Template validation failed:\n');
  for (const e of errors) console.error(`  - ${e}`);
  process.exit(1);
}

console.log('All templates validated successfully.');
