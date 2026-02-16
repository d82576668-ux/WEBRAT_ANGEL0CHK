@echo off
setlocal
set "API_BASE_URL=https://webrat-angel0chk.onrender.com"
set "DEVICE_NAME=%COMPUTERNAME%"
set "STREAM_SECRET=webrat-secret"
echo Starting WEBRAT client %DEVICE_NAME% to %API_BASE_URL%
start "" "%~dp0webrat-client-windows-amd64.exe"
endlocal
