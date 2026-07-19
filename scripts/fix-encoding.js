const fs = require('fs');
const path = require('path');

const dir = path.join(__dirname, 'apps/frontend/components/admin');
const files = fs.readdirSync(dir).filter(f => f.endsWith('.tsx')).map(f => path.join(dir, f));
let total = 0;

for (const f of files) {
  let c = fs.readFileSync(f, 'utf8');
  const orig = c;

  // Fix broken em-dash that became triple-quote: '""" ' -> '"-"'
  // Pattern: ?? """} or || """}
  c = c.replace(/\?\? """}/g, '?? "-"}');
  c = c.replace(/\|\| """}/g, '|| "-"}');

  // Remove junk trailing comment lines like: // """ ADMIN SOMETHING """""""
  const lines = c.split('\n');
  const cleaned = lines.filter(line => !line.trim().startsWith('// """'));
  if (cleaned.length !== lines.length) {
    c = cleaned.join('\n');
  }

  if (c !== orig) {
    fs.writeFileSync(f, c, 'utf8');
    total++;
    console.log('Fixed: ' + path.basename(f));
  }
}
console.log('Total fixed: ' + total);
