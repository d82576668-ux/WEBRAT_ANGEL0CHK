@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0"
if exist ".env" (
  for /f "usebackq eol=# tokens=1,* delims==" %%A in (".env") do (
    if not "%%A"=="" set "%%A=%%B"
  )
)
if not defined PORT set "PORT=8080"
if not defined DATABASE_URL (
  echo [run-api] DATABASE_URL is not set, using fallback storage
  set "DB_FALLBACK=1"
)
if not defined STREAM_SECRET set "STREAM_SECRET=webrat-secret"
powershell -NoProfile -Command "$p='.env'; if(-not (Test-Path $p)) { New-Item -ItemType File -Path $p -Force | Out-Null }; if($env:DATABASE_URL -and -not (Select-String -Path $p -Pattern '^DATABASE_URL=' -Quiet)) { Add-Content -Path $p -Value ('DATABASE_URL='+$env:DATABASE_URL) }; if(-not (Select-String -Path $p -Pattern '^STREAM_SECRET=' -Quiet)) { Add-Content -Path $p -Value ('STREAM_SECRET='+$env:STREAM_SECRET) }; if(-not (Select-String -Path $p -Pattern '^PORT=' -Quiet)) { Add-Content -Path $p -Value ('PORT='+$env:PORT) }"
echo [run-api] PORT=%PORT%
if defined DATABASE_URL ( echo [run-api] DATABASE_URL set ) else ( echo [run-api] DB_FALLBACK enabled )
echo [run-api] STREAM_SECRET %STREAM_SECRET%
if defined DATABASE_URL (
  echo [run-api] Running migrations...
  npm run migrate
)
node node_modules\typescript\bin\tsc
if errorlevel 1 (
  echo [run-api] TypeScript build failed
  pause
  exit /b 1
)
node dist\src\index.js
endlocal
