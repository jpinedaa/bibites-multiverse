Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma
SilentInstall silent
SilentUnInstall silent
AutoCloseWindow true

!include "FileFunc.nsh"
!include "LogicLib.nsh"

!ifndef PRODUCT_VERSION
  !error "PRODUCT_VERSION is required"
!endif
!ifndef PACKAGE_DIR
  !error "PACKAGE_DIR is required"
!endif
!ifndef OUTPUT_FILE
  !error "OUTPUT_FILE is required"
!endif
!ifndef PRODUCT_ICON
  !error "PRODUCT_ICON is required"
!endif

Name "Bibites Multiverse ${PRODUCT_VERSION}"
OutFile "${OUTPUT_FILE}"
Icon "${PRODUCT_ICON}"
UninstallIcon "${PRODUCT_ICON}"
InstallDir "$LOCALAPPDATA\Programs\Bibites Multiverse"
BrandingText "Bibites Multiverse"

VIProductVersion "${PRODUCT_VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "Bibites Multiverse"
VIAddVersionKey /LANG=1033 "ProductVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "FileDescription" "Bibites Multiverse Setup"
VIAddVersionKey /LANG=1033 "FileVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "CompanyName" "Bibites Multiverse"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Apache-2.0 project components"

Section "Install"
  ; $PLUGINSDIR is only created by InitPluginsDir or by the first NSIS plugin
  ; call. This script uses no plugins (FileFunc.nsh and LogicLib.nsh are pure
  ; macros), so without this line the variable is empty and extraction targets
  ; the drive root — which fails without admin rights.
  InitPluginsDir
  Delete "$TEMP\bibites-multiverse-setup.log"
  SetOutPath "$PLUGINSDIR\package"
  File /r "${PACKAGE_DIR}\*.*"

  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/PROBE" $R1
  ${IfNot} ${Errors}
    ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File "$PLUGINSDIR\package\Install-BibitesMultiverse-Gui.ps1" -Probe' $R2
    SetErrorLevel $R2
    Quit
  ${EndIf}

  CreateDirectory "$INSTDIR"
  ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File "$PLUGINSDIR\package\Install-BibitesMultiverse-Gui.ps1" -InstallRoot "$INSTDIR"' $R2
  ${If} $R2 == 2
    SetErrorLevel 0
    Quit
  ${EndIf}
  ${If} $R2 != 0
    MessageBox MB_OK|MB_ICONSTOP "Bibites Multiverse Setup could not open or complete the installer (PowerShell exit code $R2).$\n$\nA diagnostic log may be available at:$\n$TEMP\\bibites-multiverse-setup.log$\nIf it is missing, the setup did not start the installer script.$\n$\nIf the problem persists, include this log when reporting the issue."
    SetErrorLevel $R2
    Quit
  ${EndIf}

  WriteUninstaller "$INSTDIR\Uninstall.exe"

  CreateDirectory "$SMPROGRAMS\Bibites Multiverse"
  StrCpy $R3 '-NoLogo -NoProfile -ExecutionPolicy RemoteSigned -File "$INSTDIR\Start-Multiverse.ps1"'
  CreateShortCut "$SMPROGRAMS\Bibites Multiverse\Bibites Multiverse.lnk" "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" "$R3" "$INSTDIR\bibites-multiverse.ico" 0 SW_SHOWNORMAL
  CreateShortCut "$DESKTOP\Bibites Multiverse.lnk" "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" "$R3" "$INSTDIR\bibites-multiverse.ico" 0 SW_SHOWNORMAL
  CreateShortCut "$SMPROGRAMS\Bibites Multiverse\Uninstall Bibites Multiverse.lnk" "$INSTDIR\Uninstall.exe" "" "$INSTDIR\Uninstall.exe" 0 SW_SHOWNORMAL

  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "DisplayName" "Bibites Multiverse"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "DisplayIcon" "$INSTDIR\bibites-multiverse.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "Publisher" "Bibites Multiverse"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "URLInfoAbout" "https://bibitesmultiverse.com/"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "NoRepair" 1
SectionEnd

Section "Uninstall"
  IfFileExists "$INSTDIR\Uninstall-BibitesMultiverse.ps1" 0 cleanup
  ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -File "$INSTDIR\Uninstall-BibitesMultiverse.ps1"' $R0
  ${If} $R0 != 0
    MessageBox MB_OK|MB_ICONSTOP "Bibites Multiverse could not be removed. Close the game and try again."
    Abort
  ${EndIf}

cleanup:
  Delete "$DESKTOP\Bibites Multiverse.lnk"
  Delete "$SMPROGRAMS\Bibites Multiverse\Bibites Multiverse.lnk"
  Delete "$SMPROGRAMS\Bibites Multiverse\Uninstall Bibites Multiverse.lnk"
  RMDir "$SMPROGRAMS\Bibites Multiverse"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse"
  Delete "$INSTDIR\Uninstall-BibitesMultiverse.ps1"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
