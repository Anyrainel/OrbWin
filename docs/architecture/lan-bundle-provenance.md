# LAN asset bundle provenance

Source design, 2026-10-26. The verifier and build-side manifest utility exist;
the complete helper, build/staging integration and native qualification remain
unfinished. Ordinary application setup remains user scoped.

The elevated LAN helper must authenticate a coherent product bundle before
accepting the ordinary manager as a product caller. Observing a process's own
image hash and comparing it with a hash supplied by that process is insufficient.
UAC identifies the executable being elevated; a separate native scope review
authorizes the exact network change.

The bundle contains these fixed roles, in this order:

| Role | Relative path from the held manager image directory | Maximum size |
| --- | --- | --- |
| manager | `orbwin-session.exe` | 256 MiB |
| helper | `lan/orbwin-lan-helper.exe` | 64 MiB |
| worker | `lan/orbwin-lan-worker.exe` | 64 MiB |

`lan/assets.json` is canonical UTF-8 JSON without a final newline:
`{"version":1,"assets":[{"role":"manager","path":"orbwin-session.exe","sha256":"…","size":1},…]}`.
All fields, order, paths, positive integer size bounds and lowercase SHA-256
encodings are closed. The exact three entries are required. The document is
limited to 4096 bytes. Duplicate/extra fields and noncanonical representations
are refused. `lan/assets.sig` contains only the canonical 88-byte base64 encoding
of a 64-byte Ed25519 signature over `orbwin-lan-asset-bundle-v1\n` followed by the
exact manifest bytes. No request supplies an asset path or trusted expected hash.

The helper embeds only the raw 32-byte verifier public key as lowercase hex via
`orbwin.local/session/internal/lanhelper.bundleVerifierPublicKeyHex`. An empty or
invalid key refuses authorization. Native authorization derives the bundle
directory from the held actual manager image, retains the required image/path
objects, verifies the signature and every asset, and compares the actual manager
and helper images with their signed roles. The dedicated worker is staged from
that verified role; the general SDK must never be copied into its allowed
protected worker path.

## Build order and key handling

For local product qualification, the build may generate one ephemeral Ed25519
key pair in memory. It builds the manager/helper with the public key, completes
all three assets, then signs their exact manifest. This avoids reciprocal
embedded manager/helper hashes. The private key is never exported, passed in
child-process environment/arguments, logged, written into artifacts, or used as
a checked-in default. Only the public key, signature, manifest and build inputs
are retained. Rebuilding the binaries uses the recorded public linker input;
the original public manifest/signature can be verified and reused only when
the reproduced asset bytes match exactly. Changed binaries require a new bundle.

The desktop app and package staging must independently attest the complete
bundle, including its public manifest/signature and helper bytes. An unsigned,
partially staged or mismatched bundle must not enable LAN activation. Helper
verification cannot substitute for this application-side selection boundary.
This establishes same-build asset correspondence. It is not code signing,
release-publisher authentication, an update trust chain, or a sandbox against
the trusted Windows account itself.

Privileged provisioning is optional LAN first-use work, after the exact native
scope review. It does not turn ordinary setup into an administrator operation.
Prior protected records remain bound to their immutable worker paths, hashes,
native identities and original review/operation mappings. A newer bundle's key
does not authorize reusing those paths or adopting a present firewall rule.
Explicit removal still requires protected ownership, positive ingress/flow
closure and full current typed rule observation.

The build utility is `scripts/lan-asset-manifest.mjs`. Pure Node tests and the
public-only synthetic Node-to-Go signature vector cover the byte/signature
contract. They do not qualify native file identity, ACLs, process selection,
consent, protected provisioning, provider creation/removal, or installed updates.

## Native dialog resources

`lan-helper/lan.manifest` requests Common Controls v6, Windows10 compatibility
and per-monitor DPI awareness. It explicitly declares `asInvoker` with no UI
access privilege; elevation remains the separate user-initiated `runas` action.
These declarations follow Microsoft's [Common Controls manifest guidance](https://learn.microsoft.com/en-us/windows/win32/controls/cookbook-overview)
and [application manifest schema](https://learn.microsoft.com/en-us/windows/win32/sbscs/application-manifests).
`scripts/lan-helper-resource.mjs` compiles the fixed resource into a new artifact
directory, checks amd64 COFF/resource presence and exact embedded manifest bytes,
and records source/compiler/output hashes. The frozen helper build must include
that object and attest its XML/RC inputs; no generated object belongs in the
live source tree. Compile-only proof is retained at
`artifacts/qualification/p2-lan-helper-resource-20261026/proof.json`.
This does not establish actual activation-context selection or dialog behavior.
