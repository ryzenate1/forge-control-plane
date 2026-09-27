#!/usr/bin/env python3
"""Extract frontend API call paths and diff them against the backend route table.

Frontend paths are relative to API_BASE_URL (default "/api/v1").

Path normalisation rules:
  * `${...}` occupying a whole path segment  -> ":p"  (a route parameter)
  * `${...}` appended to a segment           -> path truncated there
    (that form is a query-string / suffix builder, e.g. `/alerts${qs}`)
  * query and fragment are stripped.

Template literals are scanned with brace/backtick awareness so nested
interpolations such as ``${q ? `?${q}` : ''}`` do not truncate the match.
"""
import json
import os
import re
import sys
from collections import defaultdict

WEB = sys.argv[1] if len(sys.argv) > 1 else "forge/web"
ROUTES = sys.argv[2] if len(sys.argv) > 2 else "forge/web/.scratch/routes.json"

CALLERS = (
    "requestJSON", "fetchJSON", "postJSON", "putJSON", "patchJSON",
    "deleteJSON", "requestVoid", "requestBlob", "requestText",
    "postMultipartJSON",
)
CALLER_METHOD = {
    "postJSON": "POST", "postMultipartJSON": "POST", "putJSON": "PUT",
    "patchJSON": "PATCH", "deleteJSON": "DELETE",
}

open_re = re.compile(r"\b(" + "|".join(CALLERS) + r")\s*(?:<[^()<>]*>)?\s*\(\s*")
method_re = re.compile(r"method:\s*[\"'](\w+)[\"']")

SKIP_DIRS = {"node_modules", ".next", "dist", "build", ".scratch", "coverage"}
# Paths served by Next.js route handlers, not the Go API.
NEXT_PREFIXES = ("/api/i18n",)


def scan_string(src, i):
    """Scan a JS string/template literal starting at src[i]. Return (raw, end) or None."""
    if i >= len(src) or src[i] not in "`\"'":
        return None
    q = src[i]
    out = []
    i += 1
    depth = 0  # ${ } nesting inside a template literal
    while i < len(src):
        c = src[i]
        if c == "\\":
            out.append(src[i:i + 2])
            i += 2
            continue
        if q == "`" and c == "$" and i + 1 < len(src) and src[i + 1] == "{":
            depth += 1
            out.append("${")
            i += 2
            continue
        if depth:
            # Inside an interpolation: track nesting, skip nested strings.
            if c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
            elif c in "`\"'":
                inner = scan_string(src, i)
                if inner:
                    out.append("<expr>")
                    i = inner[1]
                    continue
            out.append(c)
            i += 1
            continue
        if c == q:
            return "".join(out), i + 1
        out.append(c)
        i += 1
    return None


INTERP = re.compile(r"\$\{")


def strip_interps(raw):
    """Replace each top-level ${...} with a marker, returning (text, )."""
    out = []
    i = 0
    while i < len(raw):
        if raw.startswith("${", i):
            depth = 1
            j = i + 2
            while j < len(raw) and depth:
                if raw[j] == "{":
                    depth += 1
                elif raw[j] == "}":
                    depth -= 1
                j += 1
            out.append("\x00")  # interpolation marker
            i = j
        else:
            out.append(raw[i])
            i += 1
    return "".join(out)


def norm(raw):
    """Normalise a call path template into a comparable route shape."""
    t = strip_interps(raw).strip()
    t = t.split("?")[0].split("#")[0]
    if not t.startswith("/"):
        return None
    segs = t.strip("/").split("/")
    out = []
    for s in segs:
        if s == "\x00":
            out.append(":p")            # whole segment is a parameter
        elif "\x00" in s:
            # interpolation glued onto a literal: a suffix/query builder.
            head = s.split("\x00")[0]
            if head:
                out.append(head)
            break                        # truncate: rest is not a path segment
        else:
            out.append(s)
    return "/" + "/".join(out)


def route_shape(p):
    segs = []
    for s in p.strip("/").split("/"):
        if s.startswith(":"):
            segs.append(":p")
        elif s == "*" or s.startswith("+"):
            segs.append("*")
        else:
            segs.append(s)
    return "/" + "/".join(segs)


def main():
    routes = json.load(open(ROUTES))["routes"]
    backend = defaultdict(set)
    for key in routes:
        method, path = key.split(" ", 1)
        backend[method].add(route_shape(path))
    all_shapes = set()
    for s in backend.values():
        all_shapes |= s

    calls = defaultdict(list)
    for root, dirs, files in os.walk(WEB):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for fn in files:
            if not fn.endswith((".ts", ".tsx")):
                continue
            if re.search(r"\.(test|spec)\.tsx?$", fn):
                continue
            path = os.path.join(root, fn)
            src = open(path, encoding="utf-8", errors="replace").read()
            for m in open_re.finditer(src):
                caller = m.group(1)
                got = scan_string(src, m.end())
                if not got:
                    continue
                raw, end = got
                p = norm(raw)
                if not p or p == "/":
                    continue
                full = p if p.startswith("/api/") else "/api/v1" + p
                if full.startswith(NEXT_PREFIXES):
                    continue
                method = CALLER_METHOD.get(caller)
                if not method:
                    mm = method_re.search(src[end:end + 400])
                    method = mm.group(1).upper() if mm else "GET"
                line = src[: m.start()].count("\n") + 1
                calls[(method, route_shape(full))].append(
                    f"{os.path.relpath(path, WEB)}:{line}"
                )

    missing = {}
    for (method, shape), sites in sorted(calls.items()):
        if shape in backend.get(method, ()):
            continue
        if shape in backend.get("ALL", ()):
            continue
        if any(bs.endswith("*") and shape.startswith(bs[:-1]) for bs in all_shapes):
            continue
        missing[f"{method} {shape}"] = {
            "callers": sorted(set(sites)),
            "path_exists_for_methods": sorted(m for m in backend if shape in backend[m]),
        }

    print(f"frontend call sites: {sum(len(v) for v in calls.values())}", file=sys.stderr)
    print(f"distinct frontend method+path: {len(calls)}", file=sys.stderr)
    print(f"MISSING on backend: {len(missing)}", file=sys.stderr)
    json.dump(missing, sys.stdout, indent=1)


if __name__ == "__main__":
    main()
