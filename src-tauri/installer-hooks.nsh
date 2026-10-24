; Probe only mapped application resources. FileOpen append does not write any
; bytes; a mapped Windows executable refuses this write open. Never force-kill
; rdd, another engine, or schedule replacement over an unknown live client.
!macro ORBWIN_REQUIRE_QUIESCENCE
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
