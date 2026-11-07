# Coherent successor build inputs

The build pipeline owns immutable component bytes and provenance as build DATA.
Manifests, executionOrigin and source hashes never construct native authority.
Native Root, execution/cache leases and retirement participants remain separate.
The selected backend/tool outputs, installed executable and upstream checkout are
not candidate build destinations. No fallback accepts an older CLI schema or a
source-only CLI as a distributable successor.

Freeze the reviewed SDK and Rdd source milestones first. Canonicalize the actual
successor diff against the verified seventeen-patch stack, update complete LF
source hashes, and verify a clean ordered replay. A changed patch count alone
does not establish this source. Build in a separate replay checkout selected by
ORBWIN_UPSTREAM_DIR. Backend origin DATA is emitted only when its actual native
source matches this canonical lock; SDK pins come from the verified signed bundle.

All build output selectors require an explicit fresh directory below artifacts.
Default manager/credential builds still execute required tests. The explicit
--build-only selector omits test execution and records testsNotRun=true in its
manifest; it never claims execution qualification. SDK manager, Windows helper,
worker, generated cold payload overlay, signed assets and recipes must share one
source epoch. The Rust source catalog consumes the same folder file as JavaScript
and requires exact manifest inventory, including cold payload/guest entry source.

After source acceptance, the intended order is:

1. build-session-bundle --output-dir <fresh-sdk> [--build-only].
2. build-backend --output-dir <fresh-rdd> --with-distro, with absolute
   ORBWIN_UPGRADE_SDK_BUNDLE=<fresh-sdk> and an isolated canonical replay checkout.
3. build-credentials --output-dir <fresh-credentials> [--build-only]. Verify the
   six real locked Docker/Compose/Buildx inputs with prepare-tool-input
   --output-dir <fresh-tools>. It uses the verified current Docker source
   candidate and matching Compose/Buildx bytes, copies their provenance, and
   never downloads, executes tools or replaces the selected tool cache.
4. build-cli --output <fresh-absolute-cli> --backend-input <absolute-rdd>
   --credentials-input <absolute-credentials> --tools-input <absolute-tools>.
   The existing explicit candidate-input layout remains a separate input shape;
   the shapes cannot be mixed. Both require actual schema2 component pins.
5. stage-package --backend-input <absolute-rdd> --session-input <absolute-sdk>
   --credentials-input <absolute-credentials> --tools-input <absolute-tools>
   --cli-input <absolute-cli> --output-dir <fresh-assembly>.
6. build-package --input-dir <absolute-assembly> --output-dir <fresh-build>.
   It revalidates the assembly before generating a complete Tauri package config;
   all resource paths and desktop component/tool pins use that same assembly.
   It sets fresh frontend and Cargo output directories. Its fixed TypeScript/Vite
   compilation does not launch the UI or rerun package preparation; the Tauri
   before-build hook is disabled in this generated configuration.
   Use `--no-bundle` for the full application compile while workflow qualification
   remains open. It skips installer generation; it does not skip input validation
   or compilation. Installer generation remains after the required acceptance.

Preparation validates before exclusive output creation. Interrupted copies leave
an incomplete candidate, without a package manifest; reruns refuse that directory.
No recursive replacement or cleanup exists. Notices inspect those exact input
binaries and the same replay checkout. Copy-time hashes and final input rechecks
refuse changed components. Rdd's compiled SDK five-file pins, verifier key and
bundle-build hash must exactly match the signed SDK packaged beside it. The CLI
must match Rdd, forwarder, credentials, Windows tools, source closure and tool lock.

These are build/source acceptance steps, not authorization to execute product
roles. Broad Go tests may contain native fixtures; use reviewed pure selectors or
explicit build-only mode for compile-only work. The backend guest-capable recipe
runs host-side archive/build utilities but never starts a guest. Real cold upgrade
and whole retirement, issued guest/old engine closure, elevated LAN operations,
guest networking, installation and workload/data preservation still require their
separate qualification. Missing producers remain refusals in the product source.
