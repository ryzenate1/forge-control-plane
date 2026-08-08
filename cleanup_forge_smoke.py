#!/usr/bin/env python3
"""Clean up all forge-smoke-* resources from the Forge stack API."""

import json, sys, time
import urllib.request
import http.cookiejar

BASE = "http://localhost:8080/api/v1"
ORIGIN = "http://localhost:3000"
COOKIE_FILE = "/tmp/agent19_cookies.txt"

cj = http.cookiejar.MozillaCookieJar(COOKIE_FILE)
try: cj.load(ignore_discard=True)
except: pass

def req(method, path, body=None):
    url = BASE + path
    data = json.dumps(body).encode() if body else None
    r = urllib.request.Request(url, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    r.add_header("Origin", ORIGIN)
    if body:
        r.add_header("X-CSRF-Token", csrf_token)
    c