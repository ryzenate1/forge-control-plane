import { readFileSync, readdirSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const templatesDir = join(__dirname, '..', 'templates');
const registryPath = join(__dirname, '..', 'index.json');

const REQUIRED = ['id', 'name', 'description', 'version', 'game', 'image', 'startup', 'config', 'ports', 'env', 'resources', 'install_script', 'supported_platforms', 'categories'];

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

const registry = JSON.parse(readFileSync(registryPath, 'utf-8'));
const registeredIds = new Set(Object.keys(registry.registry));

for (const file of readdirSync(templatesDir)) {
  if (!file.endsWith('.json') || file === 'template-schema.json') continue;

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

  if (content.env) {
    for (const [i, v] of content.env.entries()) {
      if (!/^[A-Z][A-Z0-9_]*$/.test(v.env_variable)) {
        errors.push(`${file}: env[${i}] env_variable "${v.env_variable}" must match ^[A-Z][A-Z0-9_]*$`);
      }
    }
  }

  const envVariables = new Set((content.env || []).map((v) => v.env_variable));

  // Every {{...}} placeholder used in the startup command and config files must
  // resolve to either a built-in server variable or a template env variable.
  const placeholders = [];
  collectPlaceholders(content.startup, placeholders);
  if (content.config && content.config.files) {
    collectPlaceholders(content.config.files, placeholders);
  }
  for (const placeholder of new Set(placeholders)) {
    if (!BUILTIN_VARIABLES.has(placeholder) && !envVariables.has(placeholder)) {
      errors.push(`${file}: placeholder "{{${placeholder}}}" in startup/config is not a defined env variable or built-in server variable`);
    }
  }

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

  const expectedFile = `${content.id}.json`;
  if (file !== expectedFile) {
    errors.push(`file "${file}" should be named "${expectedFile}" to match id "${content.id}"`);
  }
}

if (errors.length > 0) {
  console.error('Template validation failed:\n');
  for (const e of errors) console.error(`  - ${e}`);
  process.exit(1);
}

console.log('All templates validated successfully.');
