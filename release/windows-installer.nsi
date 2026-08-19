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
; AN UPGRADE LANDS WHERE THE INSTALL IT IS UPGRADING IS. The Install section
; below writes InstallLocation into this key; reading it back here is what stops
; a second setup run from putting a whole second installation at the default
; path beside one somebody had put somewhere else - two application folders, two
; sets of shortcuts, and one *Installed apps* entry pointing at whichever ran
; last. With no such key this stays the InstallDir above, which is the path every
; installation this setup has ever made is at.
InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "InstallLocation"
BrandingText "Bibites Multiverse"

VIProductVersion "${PRODUCT_VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "Bibites Multiverse"
VIAddVersionKey /LANG=1033 "ProductVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "FileDescription" "Bibites Multiverse Setup"
VIAddVersionKey /LANG=1033 "FileVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "CompanyName" "Bibites Multiverse"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Apache-2.0 project components"

Section "Install"
  ; Use a fixed temporary folder that never depends on InitPluginsDir timing.
  StrCpy $R9 "$TEMP\bibites-multiverse-setup"
  Delete "$TEMP\bibites-multiverse-setup.log"
  CreateDirectory "$R9\package"
  SetOutPath "$R9\package"
  File /r "${PACKAGE_DIR}\*.*"

  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/PROBE" $R1
  ${IfNot} ${Errors}
    ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File "$R9\package\Install-BibitesMultiverse-Gui.ps1" -Probe' $R2
    SetErrorLevel $R2
    ; The payload is the whole package, and for the complete edition that is the
    ; game. Leave nothing of it in %TEMP% on any exit. SetOutPath first: Windows
    ; will not remove the directory this process is sitting in.
    SetOutPath "$TEMP"
    RMDir /r "$R9"
    Quit
  ${EndIf}

  CreateDirectory "$INSTDIR"
  ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File "$R9\package\Install-BibitesMultiverse-Gui.ps1" -InstallRoot "$INSTDIR"' $R2
  ${If} $R2 == 2
    SetErrorLevel 0
    SetOutPath "$TEMP"
    RMDir /r "$R9"
    Quit
  ${EndIf}
  ${If} $R2 != 0
    MessageBox MB_OK|MB_ICONSTOP "Bibites Multiverse Setup could not open or complete the installer (PowerShell exit code $R2).$\n$\nA diagnostic log may be available at:$\n$TEMP\\bibites-multiverse-setup.log$\nIf it is missing, the setup did not start the installer script.$\n$\nIf the problem persists, include this log when reporting the issue."
    SetErrorLevel $R2
    ; The diagnostic log is $TEMP\bibites-multiverse-setup.log, which is not
    ; under $R9, so nothing a report needs is thrown away here.
    SetOutPath "$TEMP"
    RMDir /r "$R9"
    Quit
  ${EndIf}

  WriteUninstaller "$INSTDIR\Uninstall.exe"

  ; Every shortcut opens the launcher, and the working directory is the
  ; installed application - not the temporary payload folder SetOutPath was
  ; last pointed at.
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  CreateDirectory "$SMPROGRAMS\Bibites Multiverse"
  CreateShortCut "$SMPROGRAMS\Bibites Multiverse\Bibites Multiverse.lnk" "$INSTDIR\BibitesMultiverseLauncher.exe" "" "$INSTDIR\bibites-multiverse.ico" 0 SW_SHOWNORMAL
  CreateShortCut "$DESKTOP\Bibites Multiverse.lnk" "$INSTDIR\BibitesMultiverseLauncher.exe" "" "$INSTDIR\bibites-multiverse.ico" 0 SW_SHOWNORMAL
  CreateShortCut "$SMPROGRAMS\Bibites Multiverse\Uninstall Bibites Multiverse.lnk" "$INSTDIR\Uninstall.exe" "" "$INSTDIR\Uninstall.exe" 0 SW_SHOWNORMAL
  SetShellVarContext all
  ClearErrors
  CreateDirectory "$SMPROGRAMS\Bibites Multiverse"
  IfErrors doneCommonShortcuts
  CreateShortCut "$SMPROGRAMS\Bibites Multiverse\Bibites Multiverse.lnk" "$INSTDIR\BibitesMultiverseLauncher.exe" "" "$INSTDIR\bibites-multiverse.ico" 0 SW_SHOWNORMAL
doneCommonShortcuts:
  SetShellVarContext current

  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "DisplayName" "Bibites Multiverse"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "DisplayIcon" "$INSTDIR\bibites-multiverse.ico"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "Publisher" "Bibites Multiverse"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "URLInfoAbout" "https://bibitesmultiverse.com/"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "HelpLink" "https://bibitesmultiverse.com/"
  ; What this installation occupies is BOTH the program files here and the
  ; managed game copies under <data root>\runtimes - a whole copy of the game per
  ; build, which is the size somebody clearing disk space came to this entry for.
  ; The graphical installer never passes -DataRoot, so the setup's data root is
  ; always the installer's own default. Reporting only $INSTDIR would tell that
  ; person a gigabyte is 15 MB.
  ;
  ; IT STOPS AT runtimes\ BECAUSE THAT IS WHERE THE UNINSTALL STOPS. This number
  ; is what removing the entry gives back, and the uninstall takes the recorded
  ; runtime by hash while the journal in data\ and the logs beside it SURVIVE it
  ; by design - only -RemoveWorldData takes those. Counting them would promise
  ; back space that uninstalling does not free.
  ;
  ; AND NOTHING MAY WALK data\ OR logs\ HERE WHATEVER THE NUMBER IS FOR.
  ; ${GetSize} enumerates every file below the path it is given, and a world's
  ; journal grows without bound: one measured data root held 145k files and
  ; 14.1 GB, and this line sat on it for 27.5 MINUTES - all of it AFTER the
  ; success dialog, with the setup invisible (SilentInstall), the shortcuts
  ; already written and the unpacked payload still in %TEMP%. A managed runtime
  ; is 216 files, so runtimes\ stays a sub-second walk however many builds have
  ; accumulated in it.
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  ClearErrors
  ; Absent in the add-on edition, which manages no runtime: GetSize sets the
  ; error flag for a path that is not there, and $3 is made 0 below.
  ${GetSize} "$LOCALAPPDATA\BibitesMultiverse\runtimes" "/S=0K" $3 $1 $2
  ${If} ${Errors}
    StrCpy $3 0
  ${EndIf}
  ClearErrors
  IntOp $0 $0 + $3
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "EstimatedSize" "$0"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse" "NoRepair" 1

  ; Nothing needs the extracted payload once the install is written.
  RMDir /r "$R9"
SectionEnd

Section "Uninstall"
  ; QuietUninstallString promises an unattended removal, so a caller that passes
  ; /S must never meet a modal dialog it cannot answer. The recorded uninstaller
  ; refuses whenever a world is running, which makes that path reachable. The
  ; exit code carries the refusal instead. Everything is silent here already
  ; (SilentUnInstall), so the flag on the command line is what tells a scripted
  ; caller apart from somebody clicking Uninstall in Programs and Features.
  StrCpy $R3 "0"
  ${un.GetParameters} $R1
  ClearErrors
  ${un.GetOptions} $R1 "/S" $R2
  ${IfNot} ${Errors}
    StrCpy $R3 "1"
  ${EndIf}
  ClearErrors

  IfFileExists "$INSTDIR\Uninstall-BibitesMultiverse.ps1" 0 cleanup
  ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy RemoteSigned -File "$INSTDIR\Uninstall-BibitesMultiverse.ps1"' $R0
  ${If} $R0 != 0
    SetErrorLevel $R0
    ${If} $R3 == "0"
      MessageBox MB_OK|MB_ICONSTOP "Bibites Multiverse could not be removed. Close the game, and every world the launcher is running, and try again."
    ${EndIf}
    Abort
  ${EndIf}

cleanup:
  SetShellVarContext current
  Delete "$DESKTOP\Bibites Multiverse.lnk"
  Delete "$SMPROGRAMS\Bibites Multiverse\Bibites Multiverse.lnk"
  Delete "$SMPROGRAMS\Bibites Multiverse\Uninstall Bibites Multiverse.lnk"
  RMDir "$SMPROGRAMS\Bibites Multiverse"
  SetShellVarContext all
  Delete "$SMPROGRAMS\Bibites Multiverse\Bibites Multiverse.lnk"
  RMDir "$SMPROGRAMS\Bibites Multiverse"
  SetShellVarContext current
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BibitesMultiverse"
  Delete "$INSTDIR\Uninstall-BibitesMultiverse.ps1"
  Delete "$INSTDIR\Uninstall.exe"
  ; The recorded uninstaller above removes the launcher by hash - a file a later
  ; hand changed is kept - and removes the profiles it wrote unconditionally,
  ; because a profile is meant to change and no recorded hash could survive an
  ; edit. This only takes the directory away once it is empty.
  RMDir "$INSTDIR\profiles"
  RMDir "$INSTDIR"
SectionEnd
