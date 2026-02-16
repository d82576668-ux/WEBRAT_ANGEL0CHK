@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"
title WEBRAT Client Builder
color 0A
cls

echo ==================================================
echo           WEBRAT CLIENT BUILDER (Windows)
echo ==================================================
echo.
echo  ====  WEBRAT CLIENT BUILDER  ====
echo.

set "DEFAULT_SITE=localhost"
set "DEFAULT_PORT=8080"

powershell -NoProfile -Command "Write-Host 'Enter API host [localhost]: ' -ForegroundColor Cyan -NoNewline"
set /p SITE=
if "%SITE%"=="" set "SITE=%DEFAULT_SITE%"

powershell -NoProfile -Command "Write-Host 'Enter API port [8080]: ' -ForegroundColor Cyan -NoNewline"
set /p PORT=
if "%PORT%"=="" set "PORT=%DEFAULT_PORT%"

set "BASE=%SITE%"
if /I not "%BASE:~0,4%"=="http" set "BASE=http://%SITE%:%PORT%"

echo.
echo Using API_BASE_URL: %BASE%
echo.

rem read STREAM_SECRET from project .env if present
set "STREAM_SECRET=webrat-secret"
if exist "%~dp0..\.env" (
  for /f "usebackq tokens=1,* delims==" %%A in ("%~dp0..\.env") do (
    if /I "%%A"=="STREAM_SECRET" set "STREAM_SECRET=%%B"
  )
)

if not exist bin mkdir bin

echo [1/3] Build Windows amd64...
go build -o bin\webrat-client-windows-amd64.exe .
if errorlevel 1 (
  echo Build failed for Windows
  goto :end
)

echo [2/3] Build Linux amd64...
set GOOS=linux
set GOARCH=amd64
go build -o bin\webrat-client-linux-amd64 .
if errorlevel 1 (
  echo Build failed for Linux (likely due to Windows-only deps). Skipping.
)
set GOOS=
set GOARCH=

echo [3/3] Generate run scripts...
> bin\run-client-windows.bat echo @echo off
>> bin\run-client-windows.bat echo setlocal
>> bin\run-client-windows.bat echo set "API_BASE_URL=%BASE%"
>> bin\run-client-windows.bat echo set "DEVICE_NAME=%%COMPUTERNAME%%"
>> bin\run-client-windows.bat echo set "STREAM_SECRET=%STREAM_SECRET%"
>> bin\run-client-windows.bat echo echo Starting WEBRAT client %%DEVICE_NAME%% to %%API_BASE_URL%%
>> bin\run-client-windows.bat echo start "" "%%~dp0webrat-client-windows-amd64.exe"
>> bin\run-client-windows.bat echo endlocal

> bin\run-client-linux.sh echo #!/usr/bin/env bash
>> bin\run-client-linux.sh echo export API_BASE_URL=%BASE%
>> bin\run-client-linux.sh echo export DEVICE_NAME=\$HOSTNAME
>> bin\run-client-linux.sh echo echo "Starting WEBRAT client \$DEVICE_NAME to \$API_BASE_URL"
>> bin\run-client-linux.sh echo ./webrat-client-linux-amd64

echo.
echo Done. Binaries and scripts: .\bin
echo Run Windows: bin\run-client-windows.bat
echo Run Linux:   ./bin/run-client-linux.sh
echo.
pause

:end
endlocal
