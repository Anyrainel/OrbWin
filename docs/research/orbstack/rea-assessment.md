# REA capability assessment and execution record

Baseline 2026-10-21 Pacific. Official identity checked against [morluto/rea](https://github.com/morluto/rea) and [rea.tools](https://rea.tools). Registry/package version **rea-agents 6.7.0**, MIT. Source documentation pinned at **dd3497e1338cda86c9ef792754d8a369bd009942**. Read the official reverse-engineer-anything workflow (metadata version 44), installation guide, native investigation guide and Windows Ghidra P0 guidance. Source main and released package capabilities may differ; runtime output wins for this host.

## Actually executed

| Command/check | Observed result | Side effects / artifact |
|---|---|---|
| `Get-Command node,npm,rea,ghidra,java,python` | Node/npm/Java/Python found; rea and ghidra absent from PATH. | Read-only discovery; not a whole-disk proof of absence. |
| `node --version` | v24.18.0; satisfies package runtime range. | Read-only. |
| `npm view rea-agents version dist.tarball --json` | 6.7.0; official registry tarball URL. | Registry read. |
| `npm exec --yes --package=rea-agents@6.7.0 -- rea --help` | CLI executes and advertises commands. | One-off package download/npm cache; no global installation or agent registration. Transitive prebuild-install deprecation warning observed. |
| `npm exec --yes --package=rea-agents@6.7.0 -- rea providers --json` | Provider/capability inventory returned. | [Raw output](support/rea-providers.json). |
| `npm exec --yes --package=rea-agents@6.7.0 -- rea doctor --provider ghidra --json` | Ghidra unavailable/not configured; GHIDRA_INSTALL_DIR not set. Diagnostic command returned nonzero readiness. | [Raw output](support/rea-doctor-ghidra.json). No configuration repair attempted. |

No OrbStack-target REA command was executed. No binary, app bundle or DMG was acquired. No decompilation, LLDB tracing, Swift runtime inspection, native UI scenario or provider-backed Mach-O analysis occurred. Public documentation and screenshot inspection were ordinary research, not REA runtime observations.

## Local capabilities and limits

**Follow-up correction (2026-10-21):** The user supplied `D:\Program Files\ghidra_12.1.4_PUBLIC\ghidraRun.bat` and `D:\BaiduDownload\OrbStack_v2.2.4_21120_amd64.dmg`; both paths were verified to exist. With process-local `GHIDRA_INSTALL_DIR` set to that installation, REA 6.7.0's scoped Ghidra doctor passed: Ghidra 12.1.4, matching Windows x64 decompiler, full JDK 21.0.8 and native authority checks. [Diagnostic evidence](support/rea-doctor-ghidra-configured.json). Earlier “not configured” results describe the earlier invocation, not absence of an installation. The DMG filename/version/architecture are user-supplied names, not yet authenticated bundle metadata. Its contents have not been analyzed. Windows REA Ghidra target-format limits still apply despite the healthy provider.

- **Observed:** native macOS provider reports unsupported host for UI observation, native scenarios/call tracing, Mach-O utilities, Swift demangling and associated native utilities. Artifact graph operations such as some static archive/resource inspection are advertised available; that does not establish complete SwiftUI reconstruction or runtime semantics.
- **Observed:** Ghidra provider is not configured. Diagnostics found an Adoptium JDK 21 path, but this does not establish a usable native analysis stack. Hopper/IDA availability must follow provider diagnostics, not existence of a similarly named executable. No working native disassembler was used.
- **Documented:** Windows Ghidra P0 is experimental Windows x64 on local fixed NTFS, with Ghidra 12.1.x and compatible full JDK. Target admission is native PE or raw NMOS 6502. **Do not claim Windows-native Ghidra Mach-O support.**
- **Documented:** Linux x64/arm64 Ghidra hosts are supported with matching native decompiler and Java; Linux process ownership checks need procps-compatible `ps`. Cross-format fixture coverage includes x86-64 Mach-O. This suggests a conditional static-analysis route, not proof that a particular OrbStack arm64/universal Swift binary will load and analyze correctly.
- **Unknown here:** WSL/Linux provider installation, procps/JDK/Ghidra readiness and target architecture coverage. Existing Ubuntu was not launched merely to probe this optional route. Root-chat WSL inventory was historical context; it was not independently revalidated or modified in this task.

## Conditional future routes

1. **Behavior research:** on an authorized Mac running a named OrbStack build, use live observation and optional REA native UI tools according to [the Mac plan](mac-observation-plan.md). Capture screenshots/accessibility/actions and CLI results; don't use binary internals to invent user-visible behavior.
2. **Static route only if justified and permitted:** establish artifact authorization/terms first. In a task-local Linux environment, inspect exact provider/target support, run scoped doctor, identify artifact architecture/hash, then a bounded inspect query. Preserve provider version, success/failure and all uncertainty. A provider opening a file does not establish correctness or completeness. No proprietary decompiled implementation enters implementation-facing specifications.
3. **No setup workaround required for this research:** CLI diagnostics work without registering MCP. Global setup would modify agent files and potentially require restart; the task forbids global agent configuration changes. No setup approval is being requested because current public research can finish without it.

OrbStack's [terms §2](https://docs.orbstack.dev/legal/terms) explicitly restrict reverse engineering. This report does not declare an exception or legal clean-room status. Use public behavior and permissively licensed upstream sources immediately; resolve any additional authorization before a future binary investigation. Do not bypass licensing, access controls or code-signing protections.
