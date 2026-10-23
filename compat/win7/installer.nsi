; Maintained Win7 installer source. Payload lists are generated from verified hashes.
Unicode true
RequestExecutionLevel user
; Zlib keeps extraction responsive on older CPUs and emulated Win7 guests.
SetCompressor zlib
CRCCheck force
AllowSkipFiles off
ManifestSupportedOS all

!include MUI2.nsh
!include LogicLib.nsh
!include WinVer.nsh
!include x64.nsh
!include "${GENERATED}/check-running.nsh"

!ifndef PRODUCT_ID
!define PRODUCT_ID "OpenCSG.CSGClaw.Win7"
!endif
!ifndef PRODUCT_NAME
!define PRODUCT_NAME "CSGClaw Win7"
!endif
!if "${PRODUCT_ID}" == "OpenCSG.CSGClaw.Win7"
!define PRODUCT_KEY "Software\OpenCSG\CSGClawWin7"
!else
!define PRODUCT_KEY "Software\OpenCSG\${PRODUCT_ID}"
!endif
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_ID}"

Var PayloadDir
Var ConfirmedDirectory
Var CurrentBuild
Var TransactionStarted
Var PreviousDisplayVersion

Name "${PRODUCT_NAME}"
OutFile "${SETUP_OUTPUT}"
BrandingText "OpenCSG"
VIProductVersion "${NUMERIC_VERSION}"
VIAddVersionKey ProductName "${PRODUCT_NAME}"
VIAddVersionKey CompanyName "OpenCSG"
VIAddVersionKey FileDescription "CSGClaw Win7 Setup"
VIAddVersionKey FileVersion "${APP_VERSION}"
VIAddVersionKey LegalCopyright "OpenCSG contributors"
!define MUI_ICON "${APP_ICON}"
!define MUI_UNICON "${APP_ICON}"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TEXT "安装或升级 CSGClaw Win7 及其全部运行依赖。$\r$\n$\r$\n构建：${APP_VERSION}$\r$\n$\r$\n自动沿用原安装目录，保留登录、配置、聊天记录和工作区。安装前请从托盘菜单退出 CSGClaw。"
!insertmacro MUI_PAGE_WELCOME
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE CheckInstallDirectory
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\CSGClaw.exe"
!define MUI_FINISHPAGE_RUN_TEXT "启动 CSGClaw Win7"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "此安装包需要 64 位 Windows 7 SP1 或更高版本。" /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  ${IfNot} ${AtLeastWin7}
    MessageBox MB_ICONSTOP "此安装包需要 Windows 7 SP1 或更高版本。" /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  ${If} ${IsWin7}
  ${AndIfNot} ${AtLeastServicePack} 1
    MessageBox MB_ICONSTOP "请先安装 Windows 7 Service Pack 1。" /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  SetShellVarContext current
  SetRegView 64
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "Local\${PRODUCT_ID}.Setup") p .r0 ?e'
  Pop $1
  ${If} $0 == 0
    MessageBox MB_ICONSTOP "无法取得安装锁，请关闭其他安装器后重试。" /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  ${If} $1 == 183
    MessageBox MB_ICONSTOP "另一个 CSGClaw 安装器正在运行，请等待它完成。" /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=upgrade.exe "${UPGRADE_HELPER}"
  ; NSIS has already honored a command-line /D= directory when supplied.
  ${If} $INSTDIR == ""
    ReadRegStr $INSTDIR HKCU "${PRODUCT_KEY}" "InstallDir"
    ${If} $INSTDIR == ""
      StrCpy $INSTDIR "$LOCALAPPDATA\Programs\${PRODUCT_NAME}"
    ${EndIf}
  ${EndIf}
FunctionEnd

!macro DefineClosedFileCheck PREFIX
Function ${PREFIX}CheckFileClosed
  Exch $R0
  Push $R1
  IfFileExists "$R0" 0 closed
  ClearErrors
  FileOpen $R1 "$R0" a
  IfErrors locked
  FileClose $R1
  Goto closed
locked:
  MessageBox MB_ICONSTOP "程序文件正在使用或无法写入。请从托盘菜单退出 CSGClaw 后重试。$\r$\n$R0" /SD IDOK
  SetErrorLevel 2
  Quit
closed:
  Pop $R1
  Pop $R0
FunctionEnd
!macroend
!insertmacro DefineClosedFileCheck ""
!insertmacro DefineClosedFileCheck "un."

Function CheckInstallDirectory
  ${If} $INSTDIR == ""
    Abort
  ${EndIf}
  ReadRegStr $1 HKCU "${PRODUCT_KEY}" "InstallDir"
  ${If} $1 != ""
  ${AndIf} $1 != $INSTDIR
    IfFileExists "$1\CSGClaw.exe" 0 directory_available
    MessageBox MB_ICONSTOP "检测到原安装目录：$1$\r$\n请使用原目录升级。如需迁移目录，请先卸载旧版，卸载会保留用户数据。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
directory_available:
  nsExec::ExecToStack '"$PLUGINSDIR\upgrade.exe" recover "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "无法恢复上次安装，请保留程序和升级备份目录并重试。$\r$\n$1" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  IfFileExists "$INSTDIR\CSGClaw.exe" 0 checked
  ReadINIStr $0 "$INSTDIR\win7-install.ini" "Product" "ID"
  ${If} $0 != "${PRODUCT_ID}"
    MessageBox MB_ICONSTOP "所选目录已有其他 CSGClaw 文件。请选择一个空目录，保留原有便携版。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
checked:
  !insertmacro CheckPayloadClosed ""
  ${If} $ConfirmedDirectory != $INSTDIR
    nsExec::ExecToStack '"$PLUGINSDIR\upgrade.exe" inspect "$INSTDIR" "${SOURCE_COMMIT}" "${SOURCE_ORDER}"'
    Pop $0
    Pop $CurrentBuild
    ${If} $0 == 12
      MessageBox MB_ICONEXCLAMATION|MB_YESNO|MB_DEFBUTTON2 "检测到较新的版本：$CurrentBuild$\r$\n将安装较旧的构建：${APP_VERSION}$\r$\n$\r$\n旧版可能无法读取新版数据。确定降级吗？" /SD IDNO IDYES confirmed
      SetErrorLevel 2
      Abort
    ${ElseIf} $0 == 11
      MessageBox MB_ICONINFORMATION|MB_OKCANCEL "已安装相同代码版本：$CurrentBuild$\r$\n继续将修复安装 ${APP_VERSION}，并保留用户数据。" /SD IDOK IDOK confirmed
      SetErrorLevel 2
      Abort
    ${ElseIf} $0 == 10
      MessageBox MB_ICONINFORMATION|MB_OKCANCEL "当前构建：$CurrentBuild$\r$\n升级到：${APP_VERSION}$\r$\n$\r$\n保留登录、配置、聊天记录和工作区。" /SD IDOK IDOK confirmed
      SetErrorLevel 2
      Abort
    ${ElseIf} $0 != 0
      MessageBox MB_ICONSTOP "无法识别原安装，请保留原目录并检查。$\r$\n$CurrentBuild" /SD IDOK
      SetErrorLevel 2
      Abort
    ${EndIf}
confirmed:
    StrCpy $ConfirmedDirectory $INSTDIR
  ${EndIf}
FunctionEnd

Section "CSGClaw Win7"
  Call CheckInstallDirectory
  SetShellVarContext current
  SetRegView 64
  ReadRegStr $PreviousDisplayVersion HKCU "${UNINSTALL_KEY}" "DisplayVersion"
  SetOverwrite on
  nsExec::ExecToStack '"$PLUGINSDIR\upgrade.exe" prepare "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "无法准备升级，原程序文件未被替换。$\r$\n$1" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  StrCpy $TransactionStarted "yes"
  StrCpy $PayloadDir "$INSTDIR.csgclaw-upgrade\stage"
  SetOutPath "$PayloadDir"
  !include "${GENERATED}/install-files.nsh"
  SetOutPath "$PayloadDir"
  ClearErrors
  WriteUninstaller "$PayloadDir\Uninstall.exe"
  WriteINIStr "$PayloadDir\win7-install.ini" "Product" "ID" "${PRODUCT_ID}"
  WriteINIStr "$PayloadDir\win7-install.ini" "Product" "Version" "${APP_VERSION}"
  ${If} ${Errors}
    SetErrorLevel 2
    Abort
  ${EndIf}
  ; Release the extraction working directory before moving staged files.
  SetOutPath "$PLUGINSDIR"
  nsExec::ExecToStack '"$PLUGINSDIR\upgrade.exe" commit "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "升级失败，已尝试恢复原版。若恢复受阻，请保留升级备份目录并退出程序后重试。$\r$\n$1" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  ClearErrors
  WriteRegStr HKCU "${PRODUCT_KEY}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "OpenCSG"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\CSGClaw.exe,0"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "EstimatedSize" ${INSTALLED_KB}
  CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk" "$INSTDIR\CSGClaw.exe" "" "$INSTDIR\resources\csgclaw.ico"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\卸载 ${PRODUCT_NAME}.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortcut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\CSGClaw.exe" "" "$INSTDIR\resources\csgclaw.ico"
  ${If} ${Errors}
    SetErrorLevel 2
    Abort
  ${EndIf}
  nsExec::ExecToStack '"$PLUGINSDIR\upgrade.exe" finish "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "无法完成升级，请保留升级备份目录并重试。$\r$\n$1" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  StrCpy $TransactionStarted ""
SectionEnd

Function .onInstFailed
  ${If} $TransactionStarted == "yes"
    SetOutPath "$PLUGINSDIR"
    nsExec::ExecToStack '"$PLUGINSDIR\upgrade.exe" recover "$INSTDIR"'
    Pop $0
    Pop $1
    ${If} $0 != 0
      MessageBox MB_ICONSTOP "恢复未完成，请保留 $INSTDIR.csgclaw-upgrade 并在退出程序后重新运行安装包。$\r$\n$1" /SD IDOK
    ${ElseIf} $PreviousDisplayVersion != ""
      WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "$PreviousDisplayVersion"
    ${Else}
      ReadRegStr $1 HKCU "${PRODUCT_KEY}" "InstallDir"
      ${If} $1 == $INSTDIR
        DeleteRegKey HKCU "${PRODUCT_KEY}"
        DeleteRegKey HKCU "${UNINSTALL_KEY}"
        Delete "$DESKTOP\${PRODUCT_NAME}.lnk"
        Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk"
        Delete "$SMPROGRAMS\${PRODUCT_NAME}\卸载 ${PRODUCT_NAME}.lnk"
        RMDir "$SMPROGRAMS\${PRODUCT_NAME}"
      ${EndIf}
    ${EndIf}
  ${EndIf}
FunctionEnd

Function un.onInit
  SetShellVarContext current
  SetRegView 64
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "Local\${PRODUCT_ID}.Setup") p .r0 ?e'
  Pop $1
  ${If} $0 == 0
  ${OrIf} $1 == 183
    MessageBox MB_ICONSTOP "另一个 CSGClaw 安装器正在运行，请等待它完成。" /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  ReadINIStr $0 "$INSTDIR\win7-install.ini" "Product" "ID"
  ${If} $0 != "${PRODUCT_ID}"
    MessageBox MB_ICONSTOP "此目录不是由 CSGClaw Win7 安装器创建的，已停止卸载。" /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  !insertmacro CheckPayloadClosed "un."
FunctionEnd

Section "Uninstall"
  ; Remove only files owned by this installer, never profiles or arbitrary trees.
  !include "${GENERATED}/uninstall-files.nsh"
  ReadRegStr $0 HKCU "${PRODUCT_KEY}" "InstallDir"
  ${If} $0 == $INSTDIR
    DeleteRegKey HKCU "${PRODUCT_KEY}"
    DeleteRegKey HKCU "${UNINSTALL_KEY}"
    Delete "$DESKTOP\${PRODUCT_NAME}.lnk"
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk"
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\卸载 ${PRODUCT_NAME}.lnk"
    RMDir "$SMPROGRAMS\${PRODUCT_NAME}"
  ${EndIf}
  Delete "$INSTDIR\win7-install.ini"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
