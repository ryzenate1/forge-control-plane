#!/usr/bin/env python3
"""For each duplicate route, print both registration lines with their middleware chain."""
import json, os, re, sys
HTTP="forge/api/internal/http"
METHODS=("Get","Post","Put","Patch","Delete","All","Head","Options")
routes=json.load(open("forge/web/.scratch/routes.json"))["routes"]
dupes={k:v for k,v in routes.items() if len(v)>1}

# index: file -> src lines
src={}
for f in os.listdir(HTTP):
    if f.endswith(".go") and not f.endswith("_test.go"):
        src[f]=open(os.path.join(HTTP,f),encoding="utf-8",errors="replace").read().split("\n")

MW=re.compile(r"require\w+\([^)]*\)|adminIPAccess|mutationLimiter|methodLimiter|authLimiter|\w+Limiter")
def chain(line):
    # take the arg list up to the handler
    m=re.search(r"\.(?:"+"|".join(METHODS)+r")\(\s*\"[^\"]*\",(.*)$",line)
    if not m: return ""
    return ", ".join(MW.findall(m.group(1)))

out=[]
for key,sites in sorted(dupes.items()):
    method,path=key.split(" ",1)
    # last path segment literal used at the call: find by suffix match
    rows=[]
    for s in sites:
        f,fn=s.split(":",1)
        for i,line in enumerate(src[f]):
            mm=re.search(r"\.("+"|".join(METHODS)+r")\(\s*\"([^\"]*)\"",line)
            if not mm: continue
            if mm.group(1).upper()!=method: continue
            lit=mm.group(2)
            if path.endswith(lit.rstrip("/")) or (lit in ("","/") and True):
                rows.append((f,i+1,lit,chain(line)))
    out.append((key,rows))

for key,rows in out:
    print(key)
    seen=set()
    for f,ln,lit,ch in rows:
        k=(f,ln)
        if k in seen: continue
        seen.add(k)
        print(f"    {f}:{ln}  lit={lit!r}  mw=[{ch}]")
