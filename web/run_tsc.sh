#!/bin/bash
cd /Users/riyaz/project/gamepanel/web
node -e "
const ts=require('typescript'),p=require('path'),fs=require('fs');
const c=ts.readConfigFile('tsconfig.json',ts.sys.readFile);
const pp=ts.parseJsonConfigFileContent(c.config,ts.sys,'.');
const pg=ts.createProgram(pp.fileNames,pp.options);
const d=ts.getPreEmitDiagnostics(pg);
const r=[];
d.forEach(x=>{
  if(!x.file||x.file.fileName.includes('node_modules'))return;
  const f=p.relative('.',x.file.fileName);
  const s=x.start!==undefined?x.file.getLineAndCharacterOfPosition(x.start):null;
  const l=s?s.line+1:'?';
  const msg=ts.flattenDiagnosticMessageText(x.messageText,'\n').replace(/\n/g,' ');
  r.push((x.category===0?'error':'warning')+' '+f+':'+l+' '+msg);
});
r.sort();
fs.writeFileSync('/tmp/ts_audit.txt',r.join('\n'));
" 2>/dev/null
echo "TSC_DONE"
