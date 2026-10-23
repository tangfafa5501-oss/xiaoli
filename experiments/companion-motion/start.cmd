@echo off
setlocal
cd /d "%~dp0"
where node >nul 2>nul
if errorlevel 1 (
  echo Node.js 22 or newer is required. See README.md.
  pause
  exit /b 1
)
if not exist "node_modules\three\package.json" (
  echo Run npm.cmd ci --ignore-scripts in this folder first. See README.md.
  pause
  exit /b 1
)
node server.mjs
if errorlevel 1 pause
