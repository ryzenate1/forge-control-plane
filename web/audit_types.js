/* eslint-disable @typescript-eslint/no-require-imports */
const ts = require('typescript');
const p = require('path');
const c = ts.readConfigFile('tsconfig.json', ts.sys.readFile);
const pp = ts.parseJsonConfigFileContent(c.config, ts.sys, '.');
const pg = ts.createProgram(pp.fileNames, pp.options);
const d = ts.getPreEmitDiagnostics(pg);
let e = 0, w = 0;
const lines = [];
d.forEach(x => {
  if (!x.file || x.file.fileName.includes('node_modules')) return;
  const f = p.relative('.', x.file.fileName);
  if (!f.startsWith('app/') && !f.startsWith('components/') && !f.startsWith('lib/') && !f.startsWith('middleware')) return;
  const s = x.start !== undefined ? x.file.getLineAndCharacterOfPosition(x.start) : null;
  const l = s ? s.line + 1 : '?';
  const msg = ts.flattenDiagnosticMessageText(x.messageText, '\n').replace(/\n/g, ' ');
  const sev = x.category === 0 ? 'ERROR' : 'WARN';
  if (x.category === 0) e++; else w++;
  lines.push(sev + ' ' + f + ':' + l + ' ' + msg);
});
lines.sort();
require('fs').writeFileSync('/tmp/ts_web_audit.txt', lines.join('\n') + '\n---\nerrors=' + e + ' warnings=' + w + '\n');
