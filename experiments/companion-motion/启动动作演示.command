#!/bin/sh
cd "$(dirname "$0")" || exit 1
if ! command -v node >/dev/null 2>&1; then
  echo '需要 Node.js 22 或更新版本，请先阅读 README.md。'
  exit 1
fi
if [ ! -f node_modules/three/package.json ]; then
  echo '请先在本目录执行 npm ci --ignore-scripts，详见 README.md。'
  exit 1
fi
exec node server.mjs
