; Probe only mapped application resources. FileOpen append does not write any
; bytes; a mapped Windows executable refuses this write open. Never force-kill
; rdd, another engine, or schedule replacement over an unknown live client.
!macro ORBWIN_REQUIRE_QUIESCENCE
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\orbwin-cli.exe"
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\orbwin-docker.exe"
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\runtime\backend\rdd.exe"
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\runtime\backend\orbwin-session.exe"
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\runtime\backend\docker-credential-orbwin.exe"
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\runtime\tools\bin\docker.exe"
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\runtime\tools\docker-cli-plugins\docker-buildx.exe"
  !insertmacro ORBWIN_PROBE_FILE "$INSTDIR\runtime\tools\docker-cli-plugins\docker-compose.exe"
!macroend

!macro ORBWIN_PROBE_FILE PATH
  Push $0
  ${If} ${FileExists} "${PATH}"
    ClearErrors
    FileOpen $0 "${PATH}" a
    ${If} ${Errors}
      Pop $0
      MessageBox MB_OK|MB_ICONEXCLAMATION "An OrbWin application file is still in use. In OrbWin, open Settings > Docker engine and choose Prepare for application update. When it finishes, close OrbWin and run this installer or uninstaller again. Engine data will be retained." /SD IDOK
      Abort
    ${Else}
      FileClose $0
    ${EndIf}
  ${EndIf}
  Pop $0
!macroend

!macro NSIS_HOOK_PREINSTALL
  !insertmacro ORBWIN_REQUIRE_QUIESCENCE
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  !insertmacro ORBWIN_REQUIRE_QUIESCENCE
!macroend

; Called by the pinned template after user confirmation/application closure and
; before deleting the executable. /UPDATE retains the stable-path opt-in entry.
; The closed native mode never creates Tauri/WebView/tray or touches the backend.
!macro ORBWIN_OWNED_SIGNIN_UNINSTALL
  ${If} $UpdateMode <> 1
    Push $0
    ClearErrors
    ExecWait '$\"$INSTDIR\${MAINBINARYNAME}.exe$\" --uninstall-owned-startup' $0
    ${If} ${Errors}
      Pop $0
      MessageBox MB_OK|MB_ICONEXCLAMATION "OrbWin could not check its sign-in entry. No application files were removed. Repair this installation or read the sign-in preference before retrying." /SD IDOK
      Abort
    ${ElseIf} $0 = 10
      DetailPrint "A foreign or modified sign-in entry was preserved."
    ${ElseIf} $0 <> 0
      Pop $0
      MessageBox MB_OK|MB_ICONEXCLAMATION "OrbWin sign-in cleanup failed or its result is unknown. No application files were removed. Read the sign-in preference before retrying; no automatic replay was made." /SD IDOK
      Abort
    ${EndIf}
    Pop $0
  ${EndIf}
!macroend
