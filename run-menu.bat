@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0"
title WEBRAT V2.0 Launcher
chcp 65001 > nul

if /I "%~1"=="server" goto run_api_once
if /I "%~1"=="admin" goto run_admin_once
if /I "%~1"=="builder" goto run_builder_once
if /I "%~1"=="all" goto run_all_once
if /I "%~1"=="env" goto config_env_once

:menu
cls
powershell -NoProfile -Command "Write-Host '============================================' -ForegroundColor Green"
powershell -NoProfile -Command "Write-Host '           WEBRAT V2.0 LAUNCHER           ' -ForegroundColor Green"
powershell -NoProfile -Command "Write-Host '============================================' -ForegroundColor Green"
echo.
powershell -NoProfile -Command "Write-Host '1) Запустить API сервер'"
powershell -NoProfile -Command "Write-Host '2) Запустить админ-панель'"
powershell -NoProfile -Command "Write-Host '3) Открыть клиент билдер'"
powershell -NoProfile -Command "Write-Host '4) Запустить все (1+2+3)'"
powershell -NoProfile -Command "Write-Host '5) Настроить .env (DATABASE_URL, STREAM_SECRET, PORT)'"
powershell -NoProfile -Command "Write-Host '6) Применить миграции (DATABASE_URL .env)'"
powershell -NoProfile -Command "Write-Host '0) Выход'"
echo.
powershell -NoProfile -Command "Write-Host 'Выберите пункты (напр. 1,2) [0-5]: ' -ForegroundColor Cyan -NoNewline"
set /p choice=
if "%choice%"=="" goto menu
set "choice=%choice:,= %"
for %%C in (%choice%) do (
  if "%%C"=="1" call :start_api
  if "%%C"=="2" call :start_admin
  if "%%C"=="3" call :start_builder
  if "%%C"=="4" (
    call :start_api
    call :start_admin
    call :start_builder
  )
  if "%%C"=="5" call :config_env
  if "%%C"=="6" call :run_migrate
  if "%%C"=="0" goto end
)
goto menu

:start_api
start "WEBRAT API" cmd /k ""%~dp0\run-api.bat""
goto :eof

:start_admin
start "WEBRAT Admin" cmd /k ""%~dp0\run-admin.bat""
goto :eof

:start_builder
start "WEBRAT CS Client Builder" powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0\cs-client\build.ps1"
goto :eof

:run_all
call :start_api
call :start_admin
call :start_builder
goto menu

:config_env
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0\edit-env.ps1"
goto :eof
 
:run_migrate
start "WEBRAT DB Migrate" cmd /k "npm run migrate"
goto :eof

:run_api_once
call :start_api
goto end

:run_admin_once
call :start_admin
goto end

:run_builder_once
call :start_builder
goto end

:run_all_once
call :start_api
call :start_admin
call :start_builder
goto end

:config_env_once
call :config_env
goto end

:end
endlocal
