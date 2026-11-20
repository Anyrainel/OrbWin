import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { createHash } from "node:crypto";
const read = (name) =>
  fs.readFileSync(new URL("../src-tauri/" + name, import.meta.url), "utf8");
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
test("NSIS template is exactly three reviewed adaptations of the pinned upstream source", () => {
  const provenance = JSON.parse(read("installer-template.provenance.json"));
  const upstream = read(provenance.upstream.path);
  assert.equal(
    hash(upstream),
    "d00089d3b1a2c4a30d7b6e098079100cb8f25e980f84d4515aca4597b95e967d",
  );
  assert.equal(
    provenance.cliVersion,
    JSON.parse(
      fs.readFileSync(
        new URL(
          "../node_modules/@tauri-apps/cli/package.json",
          import.meta.url,
        ),
      ),
    ).version,
  );
  let expected = upstream
    .replace(
      '      ${IfThen} $UpdateMode = 1 ${|} StrCpy $R1 "$R1 /UPDATE" ${|} ; append /UPDATE',
      '      ; OrbWin: stable-path upgrade must preserve the sign-in entry, including\n      ; when invoking a prior NSIS uninstaller. Explicit uninstall is unchanged.\n      ${If} $UpdateMode = 1\n      ${OrIf} $R0 = 1\n        StrCpy $R1 "$R1 /UPDATE"\n      ${EndIf}',
    )
    .replace(
      '  !insertmacro CheckIfAppIsRunning "$INSTDIR\\${MAINBINARYNAME}.exe" "${PRODUCTNAME}"\n\n  ; Delete the app directory and its content from disk',
      '  !insertmacro CheckIfAppIsRunning "$INSTDIR\\${MAINBINARYNAME}.exe" "${PRODUCTNAME}"\n\n  ; OrbWin: after confirmation/app close, before deleting the native owner.\n  !insertmacro ORBWIN_OWNED_SIGNIN_UNINSTALL\n\n  ; Delete the app directory and its content from disk',
    )
    .replace(
      '  ; Removes the Autostart entry for ${PRODUCTNAME} from the HKCU Run key if it exists.\n  ; This ensures the program does not launch automatically after uninstallation if it exists.\n  ; If it doesn\'t exist, it does nothing.\n  ; We do this when not updating (to preserve the registry value on updates)\n  ${If} $UpdateMode <> 1\n    DeleteRegValue HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Run" "${PRODUCTNAME}"\n  ${EndIf}',
      "  ; OrbWin: exact owned startup cleanup was performed transactionally before\n  ; executable deletion. Never coerce or blindly delete a foreign Run value here.",
    );
  assert.notEqual(expected, upstream);
  assert.equal(read(provenance.adapted.path), expected);
  assert.equal(hash(expected), provenance.adapted.sha256);
  for (const license of provenance.licenses) {
    const bytes = read(license.path);
    assert.ok(bytes.length > 1000);
    assert.equal(hash(bytes), license.sha256);
  }
});
test("uninstall cleanup is closed, runs before deletion, and errors abort without a registry fallback", () => {
  const hooks = read("installer-hooks.nsh");
  const macro = hooks.slice(
    hooks.indexOf("!macro ORBWIN_OWNED_SIGNIN_UNINSTALL"),
  );
  assert.match(macro, /\$UpdateMode <> 1/);
  assert.match(
    macro,
    /ExecWait '[^\n]*\$INSTDIR\\\$\{MAINBINARYNAME\}\.exe[^\n]* --uninstall-owned-startup' \$0/,
  );
  assert.match(macro, /\$\{Errors\}[\s\S]*?Abort/);
  assert.match(
    macro,
    /\$0 = 10[\s\S]*?foreign or modified sign-in entry was preserved/,
  );
  assert.match(macro, /\$0 <> 0[\s\S]*?Abort/);
  assert.doesNotMatch(
    macro,
    /ReadReg|DeleteReg|WriteReg|--background|taskkill|rdd\.exe/,
  );
  const template = read("installer-template.nsi"),
    section = template.slice(template.indexOf("Section Uninstall"));
  assert.ok(
    section.indexOf("CheckIfAppIsRunning") <
      section.indexOf("ORBWIN_OWNED_SIGNIN_UNINSTALL"),
  );
  assert.ok(
    section.indexOf("ORBWIN_OWNED_SIGNIN_UNINSTALL") <
      section.indexOf('Delete "$INSTDIR\\${MAINBINARYNAME}.exe"'),
  );
  assert.doesNotMatch(
    template,
    /DeleteRegValue HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Run"/,
  );
  const config = JSON.parse(read("tauri.package.conf.json"));
  assert.equal(config.bundle.windows.nsis.template, "installer-template.nsi");
  assert.equal(config.bundle.windows.nsis.installMode, "currentUser");
  assert.match(
    read(config.bundle.windows.nsis.customLanguageFiles.English),
    /^LangString noPermissionToCloseApp \$\{LANG_ENGLISH\} .*\{\{product_name\}\}/m,
  );
  assert.match(
    hooks,
    /!macro NSIS_HOOK_PREINSTALL\s+!insertmacro ORBWIN_REQUIRE_QUIESCENCE/,
  );
  assert.match(
    hooks,
    /!macro NSIS_HOOK_PREUNINSTALL\s+!insertmacro ORBWIN_REQUIRE_QUIESCENCE/,
  );
});
test("maintenance dispatch precedes desktop initialization and contains no UI or backend launch", () => {
  const main = read("src/main.rs"),
    body = main.slice(main.indexOf("fn main()"));
  assert.ok(
    body.indexOf("early_maintenance()") < body.indexOf("tauri::Builder"),
  );
  const maintenance = read("src/desktop.rs").split(
    "pub(crate) fn early_maintenance()",
  )[1];
  assert.match(maintenance, /policy::uninstall_mode\(&args\)/);
  assert.match(maintenance, /startup::cleanup\(&own_value\(\)\?\)/);
  assert.doesNotMatch(
    maintenance,
    /Builder|Webview|TrayIconBuilder|engine::|process::Command|create_subkey/,
  );
  assert.match(maintenance, /ForeignPreserved\) => 10/);
  assert.match(maintenance, /Err\(_\) => 21/);
});
