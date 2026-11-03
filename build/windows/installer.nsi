; FluxDM installer (per-user, no administrator rights needed)
; Build: makensis /DVERSION=0.1.0 /DSRC=..\bin build\windows\installer.nsi

Unicode true
!include "MUI2.nsh"

!ifndef VERSION
  !define VERSION "0.1.0"
!endif
!ifndef SRC
  !define SRC "..\bin"
!endif

Name "FluxDM"
OutFile "..\bin\FluxDM-Setup-${VERSION}.exe"
InstallDir "$LOCALAPPDATA\Programs\FluxDM"
InstallDirRegKey HKCU "Software\FluxDM" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
BrandingText "FluxDM ${VERSION}"

!define MUI_ICON "..\appicon\fluxdm.ico"
!define MUI_UNICON "..\appicon\fluxdm.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\fluxdm.exe"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\extension"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Ouvrir le dossier de l'extension navigateur"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "French"
!insertmacro MUI_LANGUAGE "English"

Section "FluxDM" SecMain
  SetOutPath "$INSTDIR"
  ; stop a running instance so files can be replaced
  nsExec::Exec 'taskkill /IM fluxdm.exe /F'
  File "${SRC}\fluxdm.exe"
  File "${SRC}\fluxdm-host.exe"
  SetOutPath "$INSTDIR\extension"
  File /r "..\..\extension\*.*"
  RMDir /r "$INSTDIR\extension\keys"

  WriteRegStr HKCU "Software\FluxDM" "InstallDir" "$INSTDIR"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add/Remove programs (current user)
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM" "DisplayName" "FluxDM"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM" "Publisher" "FluxDM"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM" "DisplayIcon" "$INSTDIR\fluxdm.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM" "NoRepair" 1

  ; .torrent and magnet: associations (current user)
  WriteRegStr HKCU "Software\Classes\.torrent" "" "FluxDM.torrent"
  WriteRegStr HKCU "Software\Classes\FluxDM.torrent" "" "Fichier torrent"
  WriteRegStr HKCU "Software\Classes\FluxDM.torrent\DefaultIcon" "" '"$INSTDIR\fluxdm.exe",0'
  WriteRegStr HKCU "Software\Classes\FluxDM.torrent\shell\open\command" "" '"$INSTDIR\fluxdm.exe" "%1"'
  WriteRegStr HKCU "Software\Classes\magnet" "" "URL:Magnet Link"
  WriteRegStr HKCU "Software\Classes\magnet" "URL Protocol" ""
  WriteRegStr HKCU "Software\Classes\magnet\shell\open\command" "" '"$INSTDIR\fluxdm.exe" "%1"'

  CreateDirectory "$SMPROGRAMS\FluxDM"
  CreateShortcut "$SMPROGRAMS\FluxDM\FluxDM.lnk" "$INSTDIR\fluxdm.exe"
  CreateShortcut "$SMPROGRAMS\FluxDM\Désinstaller FluxDM.lnk" "$INSTDIR\uninstall.exe"
  CreateShortcut "$DESKTOP\FluxDM.lnk" "$INSTDIR\fluxdm.exe"
SectionEnd

Section "Uninstall"
  nsExec::Exec 'taskkill /IM fluxdm.exe /F'
  Delete "$INSTDIR\fluxdm.exe"
  Delete "$INSTDIR\fluxdm-host.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir /r "$INSTDIR\extension"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\FluxDM\*.lnk"
  RMDir "$SMPROGRAMS\FluxDM"
  Delete "$DESKTOP\FluxDM.lnk"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxDM"
  DeleteRegKey HKCU "Software\FluxDM"
  DeleteRegKey HKCU "Software\Classes\FluxDM.torrent"
  DeleteRegKey HKCU "Software\Classes\fluxdm"
  DeleteRegKey HKCU "Software\Google\Chrome\NativeMessagingHosts\com.fluxdm.host"
  DeleteRegKey HKCU "Software\Microsoft\Edge\NativeMessagingHosts\com.fluxdm.host"
  DeleteRegKey HKCU "Software\Chromium\NativeMessagingHosts\com.fluxdm.host"
  DeleteRegKey HKCU "Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\com.fluxdm.host"
  DeleteRegKey HKCU "Software\Vivaldi\NativeMessagingHosts\com.fluxdm.host"
  DeleteRegKey HKCU "Software\Opera Software\NativeMessagingHosts\com.fluxdm.host"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "FluxDM"
SectionEnd
