#!/usr/bin/env python3
"""Extract the Forge API route table by resolving Fiber group prefixes.

Two passes:
  1. In server.go, find which router variable each register*() call receives,
     and the prefix that variable carries.
  2. For each register* function body, resolve local .Group() chains and emit
     "METHOD /full/path" for every .Get/.Post/.Put/.Patch/.Delete/.All call.
"""
import os
import re
import sys
import json
from collections import defaultdict

HTTP_DIR = sys.argv[1] if len(sys.argv) > 1 else "forge/api/internal/http"

METHODS = ("Get", "Post", "Put", "Patch", "Delete", "All", "Head", "Options")

# Router variables defined at server.go top level and their prefixes.
ROOT_PREFIX = {
    "app": "",
    "v1": "/api/v1",
    "protected": "/api/v1",
    "remote": "/api/remote",
    "api": "/api/v1",
}

func_re = re.compile(r"^func\s+(\w+)\s*\(([^)]*)\)", re.M)
group_re = re.compile(r"\b(\w+)\s*:?=\s*(\w+)\.Group\(\s*\"([^\"]*)\"")
route_re = re.compile(r"\b(\w+)\.(" + "|".join(METHODS) + r")\(\s*\"([^\"]*)\"")
# call site: registerFoo(varname
call_re = re.compile(r"\b(register\w+)\s*\(\s*(\w+)")


def read(p):
    with open(p, encoding="utf-8", errors="replace") as f:
        return f.read()


def split_funcs(src):
    """Yield (name, params, body) for each top-level func."""
    marks = [(m.start(), m.group(1), m.group(2)) for m in func_re.finditer(src)]
    for i, (start, name, params) in enumerate(marks):
        end = marks[i + 1][0] if i + 1 < len(marks) else len(src)
        yield name, params, src[start:end]


def first_router_param(params):
    """Name of the first param whose type is a fiber router/app."""
    for part in params.split(","):
        part = part.strip()
        if not part:
            continue
        if "fiber.Router" in part or "fiber.App" in part:
            return part.split()[0]
    return None


def resolve(body, base, seed_var):
    """Resolve group prefixes within a function body; return list of (method, path)."""
    prefix = {}
    if seed_var:
        prefix[seed_var] = base
    for k, v in ROOT_PREFIX.items():
        prefix.setdefault(k, v)

    # Resolve groups in source order, repeating so forward refs settle.
    groups = list(group_re.finditer(body))
    for _ in range(4):
        for m in groups:
            child, parent, seg = m.group(1), m.group(2), m.group(3)
            if parent in prefix:
                prefix[child] = prefix[parent] + seg

    out = []
    for m in route_re.finditer(body):
        var, method, path = m.group(1), m.group(2), m.group(3)
        if var not in prefix:
            continue  # not a router we can resolve (e.g. a local http mux)
        full = prefix[var] + path
        full = re.sub(r"//+", "/", full) or "/"
        out.append((method.upper(), full))
    return out


def main():
    files = sorted(
        os.path.join(HTTP_DIR, f)
        for f in os.listdir(HTTP_DIR)
        if f.endswith(".go") and not f.endswith("_test.go")
    )

    # Pass 1: call sites in every non-test file (server.go plus any nested callers).
    callsite = {}
    for path in files:
        src = read(path)
        for name, params, body in split_funcs(src):
            seed = first_router_param(params)
            local = dict(ROOT_PREFIX)
            for _ in range(4):
                for m in group_re.finditer(body):
                    c, p, s = m.group(1), m.group(2), m.group(3)
                    if p in local:
                        local[c] = local[p] + s
            for m in call_re.finditer(body):
                fn, var = m.group(1), m.group(2)
                if var in local:
                    callsite.setdefault(fn, local[var])

    # Pass 2: emit routes per function.
    routes = defaultdict(list)  # (method, path) -> [file:func]
    for path in files:
        src = read(path)
        rel = os.path.basename(path)
        for name, params, body in split_funcs(src):
            seed = first_router_param(params)
            if name in callsite:
                base = callsite[name]
            elif name == "NewServer":
                base = ""
            elif seed:
                # Unknown caller: assume /api/v1 for protected-style params.
                base = ROOT_PREFIX.get(seed, "/api/v1")
            else:
                base = ""
            for method, full in resolve(body, base, seed):
                routes[(method, full)].append(f"{rel}:{name}")

    result = {
        "routes": {f"{m} {p}": sorted(set(v)) for (m, p), v in sorted(routes.items())},
    }
    json.dump(result, sys.stdout, indent=1)
    print(file=sys.stderr)
    print(f"routes: {len(routes)}", file=sys.stderr)
    print(f"register fns with known callsite: {len(callsite)}", file=sys.stderr)
    dupes = {k: v for k, v in routes.items() if len(set(v)) > 1}
    print(f"duplicate registrations: {len(dupes)}", file=sys.stderr)


if __name__ == "__main__":
    main()
