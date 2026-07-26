const ts = require('typescript');
const path = require('path');
const fs = require('fs');

const configPath = ts.findConfigFile('./', ts.sys.fileExists, 'tsconfig.json');
const configFile = ts.readConfigFile(configPath, ts.sys.readFile);
const parsed = ts.parseJsonConfigFileContent(configFile.config, ts.sys, './');
const program = ts.createProgram(parsed.fileNames, parsed.options);
const diags = ts.getPreEmitDiagnostics(program);

const results = [];
diags.forEach(d => {
  if (!d.file) return;
  const file = d.file.fileName;
  if (file.includes('node_modules')) return;
  if (!file.startsWith(path.resolve('./'))) return;
  const rel = path.relative('./', file);
  const pos = d.start !== undefined ? d.file.getLineAndCharacterOfPosition(d.start) : null;
  const line = pos ? pos.line + 1 : '?';
  const col = pos ? pos.character + 1 : '?';
  const msg = ts.flattenDiagnosticMessageText(d.messageText, '\n');
  const severity = d.category === 0 ? 'error' : 'warning';
  results.push(severity + '|' + rel + ':' + line + ':' + col + '|' + msg);
});

results.sort();
fs.writeFileSync('/tmp/ts_audit.txt', results.join('\n'));
process.exit(0);
