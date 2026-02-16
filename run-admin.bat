@echo off
setlocal
cd /d "%~dp0\admin-go"
if not defined API_BASE_URL set "API_BASE_URL=https://webrat-angel0chk.onrender.com"
if not defined ADMIN_PORT set "ADMIN_PORT=4444"
echo [run-admin] API_BASE_URL=%API_BASE_URL%
echo [run-admin] ADMIN_PORT=%ADMIN_PORT%
go run .
endlocal
