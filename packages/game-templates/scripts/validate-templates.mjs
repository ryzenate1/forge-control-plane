import { readFileSync, readdirSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const templatesDir = join(__dirname, '..', 'templates');
const registryPath = join(__dirname, '..', 'index.json');

const REQUIRED = ['id', 'name', 'description', 'version', 'game', 'image', 'startup', 'config', 'ports', 'env', 'resources', 'install_script', 'supported_platforms', 'categories'];

const errors = [];

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
