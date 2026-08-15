@echo off
setlocal
cd /d "%~dp0"

where pwsh.exe >nul 2>&1
if errorlevel 1 goto windows_powershell

pwsh.exe -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0Install-BibitesMultiverse.ps1"
set "install_exit=%errorlevel%"
goto finished

:windows_powershell
powershell.exe -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0Install-BibitesMultiverse.ps1"
set "install_exit=%errorlevel%"

:finished
echo.
if "%install_exit%"=="0" (
    echo Installation complete.
) else (
    echo Installation stopped with exit code %install_exit%.
)
echo Press any key to close this window.
pause >nul
exit /b %install_exit%
