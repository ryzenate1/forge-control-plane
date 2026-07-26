#!/bin/sh
cd "$(dirname "$0")" && node check_types.js && cat /tmp/ts_audit.txt
