@echo off
setlocal
cd /d "%~dp0"

where pwsh.exe >nul 2>&1
if errorlevel 1 goto windows_powershell

pwsh.exe -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File "%~dp0Install-BibitesMultiverse-Gui.ps1"
set "install_exit=%errorlevel%"
goto finished

:windows_powershell
powershell.exe -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File "%~dp0Install-BibitesMultiverse-Gui.ps1"
set "install_exit=%errorlevel%"

:finished
exit /b %install_exit%
