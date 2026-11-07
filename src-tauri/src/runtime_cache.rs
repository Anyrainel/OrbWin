//! Fixed-role executable cache, independent of authority/engine lifecycle.
//!
//! Origin is the application's BUILD-TIME binary pin, never a colocated dynamic
//! manifest or renderer hash. Unsafe source ancestry is permitted only for a
//! held local regular file whose complete bytes match that independent pin.
//! Publication is private-at-CREATE and non-replacing. Unknown/collision retains
//! evidence; no ACL repair, existing overwrite, cleanup sweep or engine launch.
//! CachedExecutable must remain owned through the complete child lifetime.
use std::path::{Path, PathBuf};

#[path = "runtime_candidate_stage_model.rs"]
mod candidate_stage;
#[path = "runtime_candidate_custody_model.rs"]
mod custody_model;

#[derive(Clone, Copy)]
enum Role {
    Rdd,
    Session,
    Docker,
    LinuxDocker,
    Credentials,
    Project,
    #[cfg(test)]
    Fixture,
}
impl Role {
    fn cache_stem(self) -> &'static str {
        match self {
            Self::LinuxDocker => "linux-docker",
            _ => self.filename().trim_end_matches(".exe"),
        }
    }
    fn filename(self) -> &'static str {
        match self {
            Self::Rdd => "rdd.exe",
            Self::Session => "orbwin-session.exe",
            Self::Docker => "docker.exe",
            Self::LinuxDocker => "docker",
            Self::Credentials => "docker-credential-orbwin.exe",
            Self::Project => "orbwin-project",
            #[cfg(test)]
            Self::Fixture => "fixture.exe",
        }
    }
    fn limit(self) -> u64 {
        match self {
            Self::Rdd => 512 << 20,
            Self::Session => 256 << 20,
            Self::Docker | Self::LinuxDocker => 128 << 20,
            Self::Credentials | Self::Project => 16 << 20,
            #[cfg(test)]
            Self::Fixture => 128 << 20,
        }
    }
}
fn digest(value: &str) -> Result<[u8; 32], String> {
    if value.len() != 64
        || !value
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    {
        return Err("Executable cache pin is not canonical.".into());
    }
    let mut out = [0; 32];
    for (i, pair) in value.as_bytes().chunks_exact(2).enumerate() {
        out[i] = u8::from_str_radix(
            std::str::from_utf8(pair).map_err(|_| "Invalid cache pin")?,
            16,
        )
        .map_err(|_| "Invalid cache pin")?
    }
    if out == [0; 32] {
        return Err("Executable cache pin is empty.".into());
    }
    Ok(out)
}
fn hex(value: &[u8]) -> String {
    value.iter().map(|v| format!("{v:02x}")).collect()
}
fn component(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 192
        && value != "."
        && value != ".."
        && !value.ends_with(['.', ' '])
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"-_.".contains(&b))
}
fn native_component(value: &str) -> bool {
    if value.is_empty()
        || value == "."
        || value == ".."
        || value.encode_utf16().count() > 255
        || value.ends_with(['.', ' '])
        || value
            .chars()
            .any(|c| c.is_control() || r#"\/:<>"|?*"#.contains(c))
    {
        return false;
    }
    let base = value.split('.').next().unwrap_or("").to_ascii_uppercase();
    !(matches!(base.as_str(), "CON" | "PRN" | "AUX" | "NUL")
        || (base.len() == 4
            && (base.starts_with("COM") || base.starts_with("LPT"))
            && matches!(base.as_bytes()[3], b'1'..=b'9')))
}
fn copy_pinned<R: std::io::Read, W: std::io::Write>(
    input: &mut R,
    output: &mut W,
    expected: [u8; 32],
    size: u64,
    limit: u64,
) -> Result<(), String> {
    use sha2::{Digest, Sha256};
    if size == 0 || size > limit {
        return Err("Executable cache copy size refused.".into());
    }
    let mut hash = Sha256::new();
    let mut count = 0u64;
    let mut buffer = [0u8; 65536];
    loop {
        let n = input
            .read(&mut buffer)
            .map_err(|_| "Executable cache source read failed.")?;
        if n == 0 {
            break;
        }
        count = count
            .checked_add(n as u64)
            .ok_or("Executable cache copy grew.")?;
        if count > size || count > limit {
            return Err("Executable cache copy grew.".into());
        }
        output
            .write_all(&buffer[..n])
            .map_err(|_| "Executable cache stage write failed.")?;
        hash.update(&buffer[..n]);
    }
    if count != size || <[u8; 32]>::from(hash.finalize()) != expected {
        return Err("Executable cache copied bytes differ from the application pin.".into());
    }
    Ok(())
}

/// Sealed Rust-only record. Only prior qualified-identity/source-manifest callers
/// construct one; both paths must also equal the build-time embedded app pin.
pub(crate) struct VerifiedCacheSource {
    role: Role,
    source: PathBuf,
    expected: [u8; 32],
    fixture_root: Option<String>,
    bundle: Vec<BundleSource>,
    cache_key: [u8; 32],
}
struct BundleSource {
    filename: &'static str,
    source: PathBuf,
    expected: [u8; 32],
    limit: u64,
    directory: Option<&'static str>,
}
impl std::fmt::Debug for VerifiedCacheSource {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("VerifiedCacheSource(private origin)")
    }
}
impl VerifiedCacheSource {
    fn production(role: Role, expected: &str) -> Result<Self, String> {
        let pin = digest(expected)?;
        if matches!(role, Role::Rdd) {
            // Pure deferred selector: only the retained ordinary operation may
            // select/open source paths. This also covers engine::cached_backend.
            if digest(&crate::runtime::component_binary_pin("rdd.exe")?)? != pin {
                return Err("Rdd source differs from this desktop build.".into());
            }
            return Ok(Self {
                role,
                source: PathBuf::new(),
                expected: pin,
                fixture_root: None,
                bundle: Vec::new(),
                cache_key: pin,
            });
        }
        let (pin_name, path) = match role {
            Role::Docker => (
                "tools/bin/docker.exe",
                crate::runtime::tools_dir().join("bin/docker.exe"),
            ),
            Role::LinuxDocker => (
                "tools/linux/docker",
                crate::runtime::tools_dir().join("linux/docker"),
            ),
            _ => (
                role.filename(),
                crate::runtime::backend_dir().join(role.filename()),
            ),
        };
        if digest(&crate::runtime::component_binary_pin(pin_name)?)? != pin {
            return Err(
                "Executable cache origin differs from the embedded application pin.".into(),
            );
        }
        let mut bundle = Vec::new();
        let mut cache_key = pin;
        if matches!(role, Role::Rdd) {
            let filename = "rdd-forwarder.exe";
            bundle.push(BundleSource {
                filename,
                source: crate::runtime::backend_dir().join(filename),
                expected: digest(&crate::runtime::component_binary_pin(filename)?)?,
                limit: 4 << 20,
                directory: None,
            });
        }
        if matches!(role, Role::Session) {
            for (filename, limit) in [
                ("assets.json", 4096),
                ("assets.sig", 88),
                ("orbwin-lan-helper.exe", 64 << 20),
                ("orbwin-lan-worker.exe", 64 << 20),
            ] {
                let name = format!("lan/{filename}");
                let expected = digest(&crate::runtime::component_binary_pin(&name)?)?;
                if filename == "assets.json" {
                    // This canonical manifest commits all three executable
                    // hashes/sizes. Updating siblings cannot reuse a cache
                    // generation merely because the SDK bytes stayed equal.
                    cache_key = expected;
                }
                bundle.push(BundleSource {
                    filename,
                    source: crate::runtime::backend_dir().join(name),
                    expected,
                    limit,
                    directory: Some("lan"),
                });
            }
        }
        if matches!(role, Role::Docker | Role::LinuxDocker) {
            use sha2::{Digest, Sha256};
            cache_key = Sha256::digest(include_bytes!("../../runtime-tools.lock.json")).into();
            for (filename, relative, directory) in match role {
                Role::Docker => [
                    (
                        "docker-compose.exe",
                        "docker-cli-plugins/docker-compose.exe",
                        Some("docker-cli-plugins"),
                    ),
                    (
                        "docker-buildx.exe",
                        "docker-cli-plugins/docker-buildx.exe",
                        Some("docker-cli-plugins"),
                    ),
                ],
                _ => [
                    ("docker-compose", "linux/docker-compose", None),
                    ("docker-buildx", "linux/docker-buildx", None),
                ],
            } {
                let expected = digest(&crate::runtime::tool_pin(relative)?)?;
                if digest(&crate::runtime::component_binary_pin(&format!(
                    "tools/{relative}"
                ))?)?
                    != expected
                {
                    return Err("Toolset differs from this desktop build.".into());
                }
                bundle.push(BundleSource {
                    filename,
                    source: crate::runtime::tools_dir().join(relative),
                    expected,
                    limit: 256 << 20,
                    directory,
                });
            }
        }
        Ok(Self {
            role,
            source: path,
            expected: pin,
            fixture_root: None,
            bundle,
            cache_key,
        })
    }
    pub(crate) fn for_rdd_qualified_identity(expected: &str) -> Result<Self, String> {
        Self::production(Role::Rdd, expected)
    }
    fn for_modern_candidate(expected: &str) -> Result<Self, String> {
        let mut source = Self::production(Role::Rdd, expected)?;
        for role in 2..candidate_stage::ROLE_NAMES.len() {
            let filename = candidate_stage::ROLE_NAMES[role];
            source.bundle.push(BundleSource {
                filename,
                source: crate::runtime::backend_dir().join(filename),
                expected: digest(&crate::runtime::component_binary_pin(filename)?)?,
                limit: candidate_stage::ROLE_LIMITS[role],
                directory: None,
            });
        }
        Ok(source)
    }
    pub(crate) fn for_session_verified_manifest(expected: &str) -> Result<Self, String> {
        Self::production(Role::Session, expected)
    }
    pub(crate) fn for_credentials_verified_manifest(expected: &str) -> Result<Self, String> {
        Self::production(Role::Credentials, expected)
    }
    pub(crate) fn for_project_verified_manifest(expected: &str) -> Result<Self, String> {
        Self::production(Role::Project, expected)
    }
    pub(crate) fn for_windows_docker() -> Result<Self, String> {
        Self::production(Role::Docker, &crate::runtime::tool_pin("bin/docker.exe")?)
    }
    pub(crate) fn for_linux_docker() -> Result<Self, String> {
        Self::production(
            Role::LinuxDocker,
            &crate::runtime::tool_pin("linux/docker")?,
        )
    }
}

#[derive(Clone)]
pub(crate) struct CachedExecutable {
    path: PathBuf,
    #[cfg(windows)]
    lease: NativeLease,
}
#[cfg(windows)]
#[derive(Clone)]
enum NativeLease {
    Legacy(std::sync::Arc<native::Lease>),
    Rdd(native::RddLease),
}
#[cfg(windows)]
impl NativeLease {
    fn recheck(&self) -> Result<(), String> {
        match self {
            Self::Legacy(v) => v.recheck(),
            Self::Rdd(v) => v.recheck(true),
        }
    }
    fn recheck_metadata(&self) -> Result<(), String> {
        match self {
            Self::Legacy(v) => v.recheck_metadata(),
            Self::Rdd(v) => v.recheck(false),
        }
    }
    fn verify_original_process(
        &self,
        process: std::os::windows::io::BorrowedHandle<'_>,
    ) -> Result<(), String> {
        use std::os::windows::io::AsRawHandle;
        match self {
            Self::Legacy(v) => v.verify_original_process(process.as_raw_handle()),
            Self::Rdd(v) => v.verify_original_process(process, None),
        }
    }
    fn verify_process(
        &self,
        pid: u32,
        creation: u64,
        session: u32,
        child: Option<&std::process::Child>,
    ) -> Result<(), String> {
        use std::os::windows::io::AsHandle;
        match self {
            Self::Legacy(v) => v.verify_process(pid, creation, session, child),
            Self::Rdd(v) => {
                let child = child
                    .ok_or("Rdd process verification requires its original process handle.")?;
                v.verify_original_process(child.as_handle(), Some((pid, creation, session)))
            }
        }
    }
    fn manifest(&self) -> Result<serde_json::Value, String> {
        match self {
            Self::Rdd(v) => v.data().map(|(_, manifest)| manifest),
            Self::Legacy(_) => Err("Manifest requested from a non-Rdd role.".into()),
        }
    }
}

impl std::fmt::Debug for CachedExecutable {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("CachedExecutable(retained native lease)")
    }
}
impl CachedExecutable {
    pub(crate) fn path(&self) -> &Path {
        &self.path
    }
    pub(crate) fn recheck(&self) -> Result<(), String> {
        #[cfg(windows)]
        {
            self.lease.recheck()
        }
        #[cfg(not(windows))]
        {
            Err("Executable cache requires Windows.".into())
        }
    }
    // Every constructor hashes the complete pinned bundle before admission and
    // continuously holds native handles denying WRITE/DELETE. Normal dispatch
    // revalidates that unchanged ownership graph; it need not rehash hundreds
    // of MiB of immutable plugins for each inventory read.
    pub(crate) fn check_held_origin(&self) -> Result<(), String> {
        #[cfg(windows)]
        {
            self.lease.recheck_metadata()
        }
        #[cfg(not(windows))]
        {
            Err("Executable cache requires Windows.".into())
        }
    }
    #[cfg(windows)]
    pub(crate) fn verify_original_process(
        &self,
        process: std::os::windows::io::BorrowedHandle<'_>,
    ) -> Result<(), String> {
        self.lease.verify_original_process(process)
    }
    pub(crate) fn verify_process(
        &self,
        pid: u32,
        creation: u64,
        session: u32,
        child: Option<&std::process::Child>,
    ) -> Result<(), String> {
        #[cfg(windows)]
        {
            self.lease.verify_process(pid, creation, session, child)
        }
        #[cfg(not(windows))]
        {
            let _ = (pid, creation, session, child);
            Err("Process identity requires Windows.".into())
        }
    }
}
impl AsRef<Path> for CachedExecutable {
    fn as_ref(&self) -> &Path {
        &self.path
    }
}
impl AsRef<std::ffi::OsStr> for CachedExecutable {
    fn as_ref(&self) -> &std::ffi::OsStr {
        self.path.as_os_str()
    }
}
impl std::ops::Deref for CachedExecutable {
    type Target = Path;
    fn deref(&self) -> &Path {
        self.path()
    }
}
pub(crate) fn prepare(source: VerifiedCacheSource) -> Result<CachedExecutable, String> {
    #[cfg(windows)]
    {
        if source.fixture_root.is_some() {
            return native::prepare(source);
        }
        // Six closed production roles, each immutable for this desktop build.
        // Retaining a strong lease also covers plugin/helper paths handed to
        // subprocesses. No role is evicted while the desktop may launch it.
        static CACHE: std::sync::OnceLock<
            std::sync::Mutex<std::collections::BTreeMap<String, CachedExecutable>>,
        > = std::sync::OnceLock::new();
        let key = format!(
            "{}:{}:{}",
            source.role.cache_stem(),
            hex(&source.expected),
            hex(&source.cache_key)
        );
        let mut cache = CACHE
            .get_or_init(Default::default)
            .lock()
            .map_err(|_| "Runtime cache ownership unavailable")?;
        if let Some(executable) = cache.get(&key) {
            // Bytes were hashed before admission and are still held without
            // WRITE/DELETE sharing. Recheck identities/descriptors, without
            // repeatedly hashing hundreds of MiB on passive inventory polls.
            executable.lease.recheck_metadata()?;
            return Ok(executable.clone());
        }
        if cache.len() >= 6 {
            return Err("Runtime cache role capacity exceeded.".into());
        }
        let executable = native::prepare(source)?;
        cache.insert(key, executable.clone());
        Ok(executable)
    }
    #[cfg(not(windows))]
    {
        let _ = source;
        Err("Executable cache requires Windows.".into())
    }
}

/// Existing ordinary result shape, but source/provenance effects belong to the
/// published ordinary Rdd operation rather than temporary std::fs::File readers.
pub(crate) fn ordinary_backend() -> Result<(CachedExecutable, serde_json::Value), String> {
    let pin = crate::runtime::component_binary_pin("rdd.exe")?;
    let binary = prepare(VerifiedCacheSource::for_rdd_qualified_identity(&pin)?)?;
    #[cfg(windows)]
    {
        let manifest = binary.lease.manifest()?;
        Ok((binary, manifest))
    }
    #[cfg(not(windows))]
    {
        let _ = binary;
        Err("Rdd cache requires Windows.".into())
    }
}

/// Private operation-backed staging result; no serde, executable/path/raw-handle
/// accessor, launch permission or Go authority is provided by this owner.
#[derive(Clone)]
pub(crate) struct PreparedModernBackendCandidate {
    #[cfg(windows)]
    lease: native::CandidateLease,
}
impl std::fmt::Debug for PreparedModernBackendCandidate {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("PreparedModernBackendCandidate(retained original operation)")
    }
}
/// Independent bounded-one memo. It holds the actual original operation before
/// provenance/native entry, including initial failure and poisoned preparation.
pub(crate) fn prepare_modern_backend_candidate() -> Result<PreparedModernBackendCandidate, String> {
    #[cfg(windows)]
    {
        static CANDIDATE: std::sync::OnceLock<
            std::sync::Mutex<custody_model::RetainedAttempt<PreparedModernBackendCandidate>>,
        > = std::sync::OnceLock::new();
        let mut memo = CANDIDATE
            .get_or_init(Default::default)
            .lock()
            .map_err(|_| "Candidate staging ownership is unknown.".to_string())?;
        let owner = if let Some(owner) = memo.captured() {
            owner
        } else {
            // Construction is allocation-only. No provenance/provider call is
            // reachable before this exact owner has been strongly published.
            let owner = std::sync::Arc::new(PreparedModernBackendCandidate {
                lease: native::CandidateLease::new(),
            });
            if !memo.publish(owner.clone()) {
                return Err("Candidate owner publication refused.".into());
            }
            owner
        };
        owner.lease.prepare_or_recheck()?;
        Ok(owner.as_ref().clone())
    }
    #[cfg(not(windows))]
    {
        Err("Candidate staging requires Windows.".into())
    }
}

// Selection only: used to reject stale public plugin paths BEFORE cache
// creation. This does not attest a directory; prepare still verifies/holds it.
pub(crate) fn expected_path(source: &VerifiedCacheSource) -> Result<PathBuf, String> {
    #[cfg(windows)]
    {
        native::expected_path(source)
    }
    #[cfg(not(windows))]
    {
        let _ = source;
        Err("Runtime cache requires Windows.".into())
    }
}

// Admission DATA for an already owned log File only. No path is selected or
// created, no ACL repaired, and no service/native ownership is issued.
pub(crate) fn check_private_setup_log(file: &std::fs::File) -> Result<(), String> {
    #[cfg(windows)]
    {
        native::check_private_setup_log(file)
    }
    #[cfg(not(windows))]
    {
        let _ = file;
        Err("Private setup logs require Windows.".into())
    }
}

/// A diagnostic sink outside the engine namespace. Its fixed native-profile
/// route is independent of HOME/USERPROFILE and no caller path is accepted.
/// Existing private directories are checked, never repaired. Each attempt
/// creates a new file without replacing or truncating previous diagnostics.
pub(crate) fn create_private_setup_log() -> Result<std::fs::File, String> {
    #[cfg(windows)]
    {
        native::create_private_setup_log()
    }
    #[cfg(not(windows))]
    {
        Err("Private setup logs require Windows.".into())
    }
}

// Fixed existing-directory lease, never a caller path or engine capability.
// No creator, repair or fallback is reachable from this diagnostic action.
pub(crate) struct PrivateSetupLogDirectory {
    #[cfg(windows)]
    inner: native::SetupLogDirectory,
}
impl PrivateSetupLogDirectory {
    pub(crate) fn path(&self) -> Result<&std::path::Path, String> {
        #[cfg(windows)]
        {
            self.inner.path()
        }
        #[cfg(not(windows))]
        {
            Err("Private setup logs require Windows.".into())
        }
    }
    pub(crate) fn close(self) -> Result<(), String> {
        #[cfg(windows)]
        {
            self.inner.close()
        }
        #[cfg(not(windows))]
        {
            Err("Private setup logs require Windows.".into())
        }
    }
}
pub(crate) fn existing_private_setup_logs() -> Result<Option<PrivateSetupLogDirectory>, String> {
    #[cfg(windows)]
    {
        native::existing_setup_logs().map(|v| v.map(|inner| PrivateSetupLogDirectory { inner }))
    }
    #[cfg(not(windows))]
    {
        Err("Private setup logs require Windows.".into())
    }
}

// Path DATA for the existing desktop reader only. The actual Go owner still
// selects and opens all product authority/storage; this creates no directory.
pub(crate) fn native_local_appdata() -> Result<PathBuf, String> {
    #[cfg(windows)]
    {
        native::local_appdata()
    }
    #[cfg(not(windows))]
    {
        Err("Native product state selection requires Windows.".into())
    }
}

/// Test-only KnownFolder path DATA, not a cache, parent lease or HOME grant.
/// One existing User resolves both paths; its token and each CoTaskMem allocation
/// retain the existing RAII contracts, not a new checked-native-close guarantee.
#[cfg(all(test, windows))]
pub(crate) fn qualification_native_folders() -> Result<(PathBuf, PathBuf), String> {
    native::qualification_native_folders()
}

#[cfg(windows)]
mod native {
    use super::*;
    use sha2::{Digest, Sha256};
    use std::{
        ffi::c_void,
        fs::File,
        io::{Read, Seek, SeekFrom, Write},
        mem::{size_of, zeroed},
        os::windows::{
            ffi::OsStrExt,
            io::{AsRawHandle, FromRawHandle},
        },
        ptr::{null, null_mut},
        sync::atomic::{AtomicU64, Ordering},
    };
    use windows_sys::Win32::{
        Foundation::*,
        Security::{Authorization::*, *},
        Storage::FileSystem::*,
        System::RemoteDesktop::ProcessIdToSessionId,
        System::Threading::*,
        UI::Shell::{FOLDERID_LocalAppData, FOLDERID_Profile, SHGetKnownFolderPath},
    };
    const READ_CONTROL_MASK: u32 = 0x20000;
    const SYNCHRONIZE_MASK: u32 = 0x100000;
    const DELETE_MASK: u32 = 0x10000;
    const FILE_OPEN: u32 = 1;
    const FILE_CREATE: u32 = 2;
    const FILE_OPEN_IF: u32 = 3;
    const OBJ_CASE_INSENSITIVE: u32 = 0x40;
    const OBJ_DONT_REPARSE: u32 = 0x1000;
    const FILE_DIRECTORY_FILE: u32 = 1;
    const FILE_NON_DIRECTORY_FILE: u32 = 0x40;
    const FILE_SYNCHRONOUS_IO_NONALERT: u32 = 0x20;
    const NT_OPEN_REPARSE: u32 = 0x200000;
    const FILE_PERSISTENT_ACLS: u32 = 0x8;
    #[repr(C)]
    struct Unicode {
        length: u16,
        maximum: u16,
        buffer: *mut u16,
    }
    #[repr(C)]
    struct Attributes {
        length: u32,
        root: HANDLE,
        name: *mut Unicode,
        flags: u32,
        descriptor: *mut c_void,
        qos: *mut c_void,
    }
    #[repr(C)]
    struct IoStatus {
        status: usize,
        information: usize,
    }
    #[repr(C)]
    struct Rename {
        replace: u8,
        root: HANDLE,
        length: u32,
        name: [u16; 1],
    }
    #[link(name = "ntdll")]
    unsafe extern "system" {
        fn NtCreateFile(
            out: *mut HANDLE,
            access: u32,
            attributes: *mut Attributes,
            status: *mut IoStatus,
            allocation: *const i64,
            file_attributes: u32,
            share: u32,
            disposition: u32,
            options: u32,
            ea: *const c_void,
            ea_length: u32,
        ) -> i32;
        fn NtSetInformationFile(
            file: HANDLE,
            status: *mut IoStatus,
            information: *mut c_void,
            length: u32,
            class: u32,
        ) -> i32;
    }
    #[link(name = "ole32")]
    unsafe extern "system" {
        fn CoTaskMemFree(p: *const c_void);
    }
    fn bad(stage: &str) -> String {
        format!("Private executable cache {stage} refused; existing data retained.")
    }
    fn raw(file: &File) -> HANDLE {
        file.as_raw_handle()
    }
    fn own(h: HANDLE) -> Result<File, String> {
        if h.is_null() || h == INVALID_HANDLE_VALUE {
            Err(bad("native handle"))
        } else {
            Ok(unsafe { File::from_raw_handle(h) })
        }
    }
    fn wide(path: &Path) -> Result<Vec<u16>, String> {
        let v: Vec<u16> = path.as_os_str().encode_wide().collect();
        if v.len() > 32760 || v.contains(&0) {
            return Err(bad("path encoding"));
        }
        Ok(v.into_iter().chain(Some(0)).collect())
    }
    fn sid_at(base: *const u8, length: usize, p: *const c_void) -> Result<Vec<u8>, String> {
        let start = base as usize;
        let address = p as usize;
        if p.is_null()
            || address < start
            || address - start > length
            || length - (address - start) < 8
        {
            return Err(bad("SID bounds"));
        }
        let count = unsafe { *(p.cast::<u8>().add(1)) } as usize;
        let len = 8 + count * 4;
        if count > 15
            || len > length - (address - start)
            || unsafe { IsValidSid(p.cast_mut()) } == 0
            || unsafe { GetLengthSid(p.cast_mut()) } as usize != len
        {
            return Err(bad("SID shape"));
        }
        Ok(unsafe { std::slice::from_raw_parts(p.cast::<u8>(), len) }.to_vec())
    }
    fn sid_text(sid: &[u8]) -> Result<String, String> {
        let mut p = null_mut();
        if unsafe { ConvertSidToStringSidW(sid.as_ptr().cast_mut().cast(), &mut p) } == 0 {
            return Err(bad("SID encoding"));
        }
        let mut n = 0;
        while n < 184 && unsafe { *p.add(n) } != 0 {
            n += 1
        }
        let result = if n < 184 {
            String::from_utf16(unsafe { std::slice::from_raw_parts(p, n) })
                .map_err(|_| bad("SID encoding"))
        } else {
            Err(bad("SID bounds"))
        };
        unsafe {
            LocalFree(p.cast());
        }
        result
    }
    struct User {
        sid: Vec<u8>,
        text: String,
        token: File,
        session: u32,
    }
    enum KnownFolder {
        Profile,
        LocalAppData,
    }
    pub(super) fn local_appdata() -> Result<PathBuf, String> {
        User::current()?.known_folder(KnownFolder::LocalAppData)
    }
    #[cfg(test)]
    pub(super) fn qualification_native_folders() -> Result<(PathBuf, PathBuf), String> {
        let user = User::current()?;
        let local = user.known_folder(KnownFolder::LocalAppData)?;
        let profile = user.profile()?;
        Ok((local, profile))
    }
    impl User {
        fn current() -> Result<Self, String> {
            let mut thread = null_mut();
            if unsafe { OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, 1, &mut thread) } != 0 {
                unsafe {
                    CloseHandle(thread);
                }
                return Err(bad("impersonated caller"));
            }
            if unsafe { GetLastError() } != ERROR_NO_TOKEN {
                return Err(bad("effective token"));
            }
            let mut h = null_mut();
            // SHGetKnownFolderPath's explicit-user token contract requires
            // QUERY|IMPERSONATE. This does not impersonate or enable privileges.
            if unsafe {
                OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY | TOKEN_IMPERSONATE, &mut h)
            } == 0
            {
                return Err(bad("process token"));
            }
            let token = own(h)?;
            let mut elevated = 0u32;
            let mut needed = 0;
            if unsafe {
                GetTokenInformation(
                    raw(&token),
                    TokenElevation,
                    (&mut elevated as *mut u32).cast(),
                    4,
                    &mut needed,
                )
            } == 0
                || needed != 4
                || elevated != 0
            {
                return Err(bad("ordinary caller"));
            }
            let mut session = 0u32;
            if unsafe {
                GetTokenInformation(
                    raw(&token),
                    TokenSessionId,
                    (&mut session as *mut u32).cast(),
                    4,
                    &mut needed,
                )
            } == 0
                || needed != 4
                || session == 0
            {
                return Err(bad("interactive token session"));
            }
            let mut buffer = [0u64; 256];
            if unsafe {
                GetTokenInformation(
                    raw(&token),
                    TokenUser,
                    buffer.as_mut_ptr().cast(),
                    size_of::<[u64; 256]>() as u32,
                    &mut needed,
                )
            } == 0
                || needed as usize > size_of::<[u64; 256]>()
                || needed < size_of::<TOKEN_USER>() as u32
            {
                return Err(bad("token user"));
            }
            let user = unsafe { &*buffer.as_ptr().cast::<TOKEN_USER>() };
            let sid = sid_at(buffer.as_ptr().cast(), needed as usize, user.User.Sid)?;
            let text = sid_text(&sid)?;
            if !(text.starts_with("S-1-5-21-") || text.starts_with("S-1-12-1-")) {
                return Err(bad("ordinary account"));
            }
            Ok(Self {
                sid,
                text,
                token,
                session,
            })
        }
        fn profile(&self) -> Result<PathBuf, String> {
            self.known_folder(KnownFolder::Profile)
        }
        fn known_folder(&self, folder: KnownFolder) -> Result<PathBuf, String> {
            let mut p = null_mut();
            let id = match folder {
                KnownFolder::Profile => &FOLDERID_Profile,
                KnownFolder::LocalAppData => &FOLDERID_LocalAppData,
            };
            let status = unsafe { SHGetKnownFolderPath(id, 0, raw(&self.token), &mut p) };
            // Microsoft requires CoTaskMemFree on a returned allocation even
            // when the call fails. Never inspect output bytes on failure.
            if p.is_null() {
                return Err(bad("native known folder"));
            }
            struct FolderAllocation(*mut u16);
            impl Drop for FolderAllocation {
                fn drop(&mut self) {
                    unsafe { CoTaskMemFree(self.0.cast()) }
                }
            }
            let allocation = FolderAllocation(p);
            if status < 0 {
                return Err(bad("native known folder"));
            }
            let mut n = 0;
            while n < 1024 && unsafe { *allocation.0.add(n) } != 0 {
                n += 1
            }
            if n > 0 && n < 1024 {
                String::from_utf16(unsafe { std::slice::from_raw_parts(allocation.0, n) })
                    .map(PathBuf::from)
                    .map_err(|_| bad("known folder encoding"))
            } else {
                Err(bad("known folder bounds"))
            }
        }
    }
    struct Descriptor {
        data: *mut c_void,
    }
    impl Drop for Descriptor {
        fn drop(&mut self) {
            unsafe {
                LocalFree(self.data);
            }
        }
    }
    impl Descriptor {
        fn private(user: &User, directory: bool) -> Result<Self, String> {
            let inherit = if directory { "OICI" } else { "" };
            let text: Vec<u16> = format!(
                "O:{}D:P(A;{inherit};FA;;;{})(A;{inherit};FA;;;SY)(A;{inherit};FA;;;BA)",
                user.text, user.text
            )
            .encode_utf16()
            .chain(Some(0))
            .collect();
            let mut p = null_mut();
            if unsafe {
                ConvertStringSecurityDescriptorToSecurityDescriptorW(
                    text.as_ptr(),
                    1,
                    &mut p,
                    null_mut(),
                )
            } == 0
            {
                return Err(bad("private descriptor"));
            }
            Ok(Self { data: p })
        }
    }
    struct Security {
        bytes: Vec<u32>,
        length: usize,
    }
    impl Security {
        fn read(file: &File) -> Result<Self, String> {
            let mut n = 0;
            unsafe {
                GetKernelObjectSecurity(
                    raw(file),
                    OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                    null_mut(),
                    0,
                    &mut n,
                );
            }
            if !(20..=65536).contains(&n) {
                return Err(bad("descriptor bounds"));
            }
            let mut bytes = vec![0u32; (n as usize).div_ceil(4)];
            let capacity = n;
            if unsafe {
                GetKernelObjectSecurity(
                    raw(file),
                    OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                    bytes.as_mut_ptr().cast(),
                    capacity,
                    &mut n,
                )
            } == 0
                || n > capacity
                || n < 20
            {
                return Err(bad("descriptor read"));
            }
            Ok(Self {
                bytes,
                length: n as usize,
            })
        }
        fn descriptor(&self) -> *mut c_void {
            self.bytes.as_ptr().cast_mut().cast()
        }
        fn owner_acl(&self) -> Result<(Vec<u8>, *mut ACL, u16), String> {
            let mut owner = null_mut();
            let mut defaulted = 0;
            let mut present = 0;
            let mut acl = null_mut();
            let mut control = 0;
            let mut revision = 0;
            if unsafe { GetSecurityDescriptorOwner(self.descriptor(), &mut owner, &mut defaulted) }
                == 0
                || unsafe {
                    GetSecurityDescriptorDacl(
                        self.descriptor(),
                        &mut present,
                        &mut acl,
                        &mut defaulted,
                    )
                } == 0
                || present == 0
                || acl.is_null()
                || unsafe {
                    GetSecurityDescriptorControl(self.descriptor(), &mut control, &mut revision)
                } == 0
            {
                return Err(bad("descriptor shape"));
            }
            let sid = sid_at(self.bytes.as_ptr().cast(), self.length, owner)?;
            let base = self.bytes.as_ptr() as usize;
            let offset = (acl as usize)
                .checked_sub(base)
                .ok_or_else(|| bad("ACL bounds"))?;
            if offset > self.length || self.length - offset < size_of::<ACL>() {
                return Err(bad("ACL bounds"));
            }
            let size = unsafe { (*acl).AclSize } as usize;
            if size < size_of::<ACL>()
                || size > self.length - offset
                || unsafe { (*acl).AceCount } > 256
            {
                return Err(bad("ACL bounds"));
            }
            Ok((sid, acl, control))
        }
        fn aces(&self, acl: *mut ACL) -> Result<Vec<(u8, u8, u32, Vec<u8>)>, String> {
            let mut out = Vec::new();
            for i in 0..unsafe { (*acl).AceCount } as u32 {
                let mut p = null_mut();
                if unsafe { GetAce(acl, i, &mut p) } == 0 || p.is_null() {
                    return Err(bad("ACE read"));
                }
                let base = acl as usize;
                let offset = (p as usize)
                    .checked_sub(base)
                    .ok_or_else(|| bad("ACE bounds"))?;
                let total = unsafe { (*acl).AclSize } as usize;
                if offset > total || total - offset < 8 {
                    return Err(bad("ACE bounds"));
                }
                let ace = unsafe { &*p.cast::<ACCESS_ALLOWED_ACE>() };
                let size = ace.Header.AceSize as usize;
                if size < 16
                    || size > total - offset
                    || ace.Header.AceType > 1
                    || ace.Header.AceFlags & !0x1f != 0
                {
                    return Err(bad("ACE shape"));
                }
                let sid = sid_at(p.cast(), size, unsafe { p.cast::<u8>().add(8) }.cast())?;
                out.push((ace.Header.AceType, ace.Header.AceFlags, ace.Mask, sid));
            }
            Ok(out)
        }
        fn trusted(sid: &[u8], user: &User) -> Result<bool, String> {
            if sid == user.sid {
                return Ok(true);
            }
            let text = sid_text(sid)?;
            Ok(matches!(
                text.as_str(),
                "S-1-5-18"
                    | "S-1-5-32-544"
                    | "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
            ))
        }
        // One DATA decision function is shared with the candidate's owned-query
        // adapter. Existing native extraction and raw SD hash remain unchanged.
        fn allowed(&self, user: &User, private: bool, directory: bool) -> Result<(), String> {
            let (owner, acl, control) = self.owner_acl()?;
            // Preserve the original owner/protection refusal before GetAce.
            if !custody_model::descriptor_header_allowed(&owner, &user.sid, control, private) {
                return Err(bad("descriptor owner/protection"));
            }
            let aces = self
                .aces(acl)?
                .into_iter()
                .map(|(kind, flags, mask, sid)| custody_model::Ace {
                    kind,
                    flags,
                    mask,
                    sid,
                })
                .collect::<Vec<_>>();
            if custody_model::descriptor_allowed(
                &owner, &user.sid, control, &aces, private, directory,
            ) {
                Ok(())
            } else {
                Err(bad("descriptor policy"))
            }
        }
        fn integrity(&self, user: &User) -> Result<(), String> {
            self.allowed(user, false, true)
        }
        fn private(&self, user: &User, directory: bool) -> Result<(), String> {
            self.allowed(user, true, directory)
        }
        fn hash(&self) -> [u8; 32] {
            Sha256::digest(unsafe {
                std::slice::from_raw_parts(self.bytes.as_ptr().cast::<u8>(), self.length)
            })
            .into()
        }
    }
    #[derive(Clone, Copy, PartialEq, Eq)]
    struct Identity {
        volume: u32,
        high: u32,
        low: u32,
        attributes: u32,
        links: u32,
        size: u64,
        write: u64,
    }
    pub(super) fn check_private_setup_log(file: &File) -> Result<(), String> {
        let user = User::current()?;
        identity(file, false)?;
        Security::read(file)?.private(&user, false)
    }

    pub(super) struct SetupLogDirectory {
        user: Option<User>,
        ancestors: Vec<Held>,
        path: PathBuf,
    }
    impl SetupLogDirectory {
        pub(super) fn path(&self) -> Result<&Path, String> {
            let user = self.user.as_ref().ok_or_else(|| bad("setup log user"))?;
            for held in &self.ancestors {
                held.recheck(user)?;
            }
            Ok(&self.path)
        }
        // Consuming checked reverse release. A failed close is never retried;
        // ManuallyDrop retains all remaining original resources on error/panic.
        pub(super) fn close(self) -> Result<(), String> {
            use std::{mem::ManuallyDrop, os::windows::io::IntoRawHandle};
            let mut owner = ManuallyDrop::new(self);
            while let Some(held) = owner.ancestors.last() {
                if unsafe { CloseHandle(raw(&held.file)) } == 0 {
                    return Err(bad("setup log release"));
                }
                let held = owner
                    .ancestors
                    .pop()
                    .ok_or_else(|| bad("setup log release order"))?;
                let _ = held.file.into_raw_handle(); // Disarm after positive close.
            }
            let user = owner
                .user
                .as_ref()
                .ok_or_else(|| bad("setup log release user"))?;
            if unsafe { CloseHandle(raw(&user.token)) } == 0 {
                return Err(bad("setup log token release"));
            }
            let user = owner
                .user
                .take()
                .ok_or_else(|| bad("setup log release user"))?;
            let _ = user.token.into_raw_handle();
            drop(ManuallyDrop::into_inner(owner));
            Ok(())
        }
    }
    pub(super) fn existing_setup_logs() -> Result<Option<SetupLogDirectory>, String> {
        let user = User::current()?;
        let (profile, mut ancestors) = profile_chain(&user)?;
        for name in [".orbwin-private", "setup-logs"] {
            let parent = &ancestors
                .last()
                .ok_or_else(|| bad("setup log parent"))?
                .file;
            let file = match relative_status(parent, name, true, FILE_OPEN, None, false) {
                Ok(file) => file,
                Err(RelativeError::Missing) => return Ok(None),
                Err(_) => return Err(bad("existing setup log directory")),
            };
            ancestors.push(Held::capture(file, &user, true, true)?);
        }
        let lease = SetupLogDirectory {
            user: Some(user),
            ancestors,
            path: profile.join(".orbwin-private").join("setup-logs"),
        };
        lease.path()?;
        Ok(Some(lease))
    }

    pub(super) fn create_private_setup_log() -> Result<File, String> {
        let user = User::current()?;
        let (_, mut ancestors) = profile_chain(&user)?;
        let directory_sd = Descriptor::private(&user, true)?;
        // The executable cache must already have initialized this root; this
        // helper cannot precreate the service's OrbWinData or guest home.
        for (name, disposition) in [(".orbwin-private", FILE_OPEN), ("setup-logs", FILE_OPEN_IF)] {
            let parent = &ancestors
                .last()
                .ok_or_else(|| bad("setup log parent"))?
                .file;
            let file = relative(parent, name, true, disposition, Some(&directory_sd), false)?;
            ancestors.push(Held::capture(file, &user, true, true)?);
        }
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_err(|_| bad("setup log time"))?
            .as_nanos();
        let name = format!(
            "setup-{}-{stamp}-{}.log",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        );
        let file_sd = Descriptor::private(&user, false)?;
        let parent = &ancestors
            .last()
            .ok_or_else(|| bad("setup log directory"))?
            .file;
        let file = relative(parent, &name, false, FILE_CREATE, Some(&file_sd), true)?;
        check_private_setup_log(&file)?;
        if identity(&file, false)?.size != 0 {
            return Err(bad("new setup log size"));
        }
        for held in &ancestors {
            held.recheck(&user)?;
        }
        // Only this original owned file is handed to Stdio; no subsequently
        // reopened path can substitute the child log. Directory leases are
        // acquisition custody only, not service/guest authority.
        Ok(file)
    }

    fn identity(file: &File, directory: bool) -> Result<Identity, String> {
        let mut info: BY_HANDLE_FILE_INFORMATION = unsafe { zeroed() };
        let mut path = [0u16; 1024];
        let mut flags = 0;
        if unsafe { GetFileInformationByHandle(raw(file), &mut info) } == 0
            || unsafe { GetFileType(raw(file)) } != FILE_TYPE_DISK
            || info.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT != 0
            || (info.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY != 0) != directory
            || (!directory && info.nNumberOfLinks != 1)
        {
            return Err(bad("object kind/identity"));
        }
        let n = unsafe {
            GetFinalPathNameByHandleW(raw(file), path.as_mut_ptr(), path.len() as u32, 2)
        } as usize;
        if n == 0
            || n >= path.len()
            || !String::from_utf16_lossy(&path[..n]).starts_with(r"\Device\HarddiskVolume")
            || unsafe {
                GetVolumeInformationByHandleW(
                    raw(file),
                    null_mut(),
                    0,
                    null_mut(),
                    null_mut(),
                    &mut flags,
                    null_mut(),
                    0,
                )
            } == 0
            || flags & FILE_PERSISTENT_ACLS == 0
        {
            return Err(bad("local persistent volume"));
        }
        Ok(Identity {
            volume: info.dwVolumeSerialNumber,
            high: info.nFileIndexHigh,
            low: info.nFileIndexLow,
            attributes: info.dwFileAttributes,
            links: info.nNumberOfLinks,
            size: if directory {
                0
            } else {
                (u64::from(info.nFileSizeHigh) << 32) | u64::from(info.nFileSizeLow)
            },
            write: if directory {
                0
            } else {
                (u64::from(info.ftLastWriteTime.dwHighDateTime) << 32)
                    | u64::from(info.ftLastWriteTime.dwLowDateTime)
            },
        })
    }
    struct Held {
        file: File,
        id: Identity,
        security: [u8; 32],
        directory: bool,
        private: bool,
    }
    impl Held {
        fn capture(
            file: File,
            user: &User,
            directory: bool,
            private: bool,
        ) -> Result<Self, String> {
            let id = identity(&file, directory)?;
            let sd = Security::read(&file)?;
            if private {
                sd.private(user, directory)?
            } else {
                sd.integrity(user)?
            }
            Ok(Self {
                file,
                id,
                security: sd.hash(),
                directory,
                private,
            })
        }
        fn recheck(&self, user: &User) -> Result<(), String> {
            if identity(&self.file, self.directory)? != self.id {
                return Err(bad("retained identity changed"));
            }
            let sd = Security::read(&self.file)?;
            if sd.hash() != self.security {
                return Err(bad("retained descriptor changed"));
            }
            if self.private {
                sd.private(user, self.directory)
            } else {
                sd.integrity(user)
            }
        }
    }
    pub(super) struct Lease {
        sid: Vec<u8>,
        ancestors: Vec<Held>,
        image: Held,
        expected: [u8; 32],
        limit: u64,
        gate: std::sync::Mutex<()>,
        bundle: Vec<(Held, [u8; 32], u64)>,
    }
    impl Lease {
        fn recheck_objects(&self) -> Result<(), String> {
            let user = User::current()?;
            if user.sid != self.sid {
                return Err(bad("current owner changed"));
            }
            for h in &self.ancestors {
                h.recheck(&user)?;
            }
            self.image.recheck(&user)?;
            for (file, _, _) in &self.bundle {
                file.recheck(&user)?;
            }
            Ok(())
        }
        pub(super) fn recheck_metadata(&self) -> Result<(), String> {
            let _guard = self
                .gate
                .lock()
                .map_err(|_| bad("concurrent lease recheck"))?;
            self.recheck_objects()
        }
        pub(super) fn verify_process(
            &self,
            pid: u32,
            creation: u64,
            session: u32,
            child: Option<&std::process::Child>,
        ) -> Result<(), String> {
            self.recheck_metadata()?;
            let user = User::current()?;
            if pid == 0 || creation == 0 || session == 0 || session != user.session {
                return Err(bad("service process tuple"));
            }
            let process = own(unsafe {
                OpenProcess(
                    PROCESS_QUERY_LIMITED_INFORMATION | PROCESS_SYNCHRONIZE,
                    0,
                    pid,
                )
            })?;
            self.verify_process_image(raw(&process), pid, creation, session, child)
        }
        // Borrowed original kernel process, never a reopened numeric PID.
        pub(super) fn verify_original_process(&self, process: HANDLE) -> Result<(), String> {
            self.recheck_metadata()?;
            let user = User::current()?;
            let pid = unsafe { GetProcessId(process) };
            let mut created: FILETIME = unsafe { zeroed() };
            let mut exited = created;
            let mut kernel = created;
            let mut used = created;
            let mut session = 0;
            if pid == 0
                || unsafe {
                    GetProcessTimes(process, &mut created, &mut exited, &mut kernel, &mut used)
                } == 0
                || unsafe { ProcessIdToSessionId(pid, &mut session) } == 0
                || session == 0
                || session != user.session
            {
                return Err(bad("original process generation"));
            }
            let creation =
                (u64::from(created.dwHighDateTime) << 32) | u64::from(created.dwLowDateTime);
            if creation == 0 {
                return Err(bad("original process generation"));
            }
            self.verify_process_image(process, pid, creation, session, None)
        }
        fn verify_process_image(
            &self,
            process: HANDLE,
            pid: u32,
            creation: u64,
            session: u32,
            child: Option<&std::process::Child>,
        ) -> Result<(), String> {
            fn times(handle: HANDLE) -> Result<u64, String> {
                let mut created: FILETIME = unsafe { zeroed() };
                let mut exited = created;
                let mut kernel = created;
                let mut user = created;
                if unsafe {
                    GetProcessTimes(handle, &mut created, &mut exited, &mut kernel, &mut user)
                } == 0
                {
                    return Err(bad("process creation time"));
                }
                Ok((u64::from(created.dwHighDateTime) << 32) | u64::from(created.dwLowDateTime))
            }
            let mut actual_session = 0;
            if unsafe { GetProcessId(process) } != pid
                || times(process)? != creation
                || unsafe { ProcessIdToSessionId(pid, &mut actual_session) } == 0
                || actual_session != session
                || unsafe { WaitForSingleObject(process, 0) } != WAIT_TIMEOUT
            {
                return Err(bad("service process generation"));
            }
            if let Some(child) = child {
                if child.id() != pid || times(child.as_raw_handle())? != creation {
                    return Err(bad("spawned service generation"));
                }
            }
            let mut path = vec![0u16; 32768];
            let mut length = path.len() as u32;
            if unsafe { QueryFullProcessImageNameW(process, 0, path.as_mut_ptr(), &mut length) }
                == 0
                || length == 0
                || length as usize >= path.len()
            {
                return Err(bad("actual process image"));
            }
            let path = PathBuf::from(
                String::from_utf16(&path[..length as usize])
                    .map_err(|_| bad("actual image encoding"))?,
            );
            let image = source_open(&path)?;
            // The actual image is the SAME native object as our continuously
            // held, independently hashed cache image. Opening a different file
            // with matching bytes cannot substitute for this identity check.
            if identity(&image, false)? != self.image.id
                || times(process)? != creation
                || unsafe { WaitForSingleObject(process, 0) } != WAIT_TIMEOUT
            {
                return Err(bad("actual service image differs"));
            }
            self.recheck_metadata()
        }
        pub(super) fn recheck(&self) -> Result<(), String> {
            let _guard = self
                .gate
                .lock()
                .map_err(|_| bad("concurrent lease recheck"))?;
            self.recheck_objects()?;
            if hash(&self.image.file, self.limit)? != self.expected {
                return Err(bad("cached hash changed"));
            }
            for (file, expected, limit) in &self.bundle {
                if hash(&file.file, *limit)? != *expected {
                    return Err(bad("cached bundle hash changed"));
                }
            }
            Ok(())
        }
    }
    #[derive(Debug, PartialEq, Eq)]
    enum RelativeError {
        Missing,
        Refused,
    }
    // Metadata-only directory opens do not enter NT share-delete accounting.
    // Request traversal, not enumeration, for the retained provenance chain.
    pub(super) fn retained_directory_access() -> u32 {
        FILE_TRAVERSE
    }
    fn relative_status(
        parent: &File,
        name: &str,
        directory: bool,
        disposition: u32,
        descriptor: Option<&Descriptor>,
        write: bool,
    ) -> Result<File, RelativeError> {
        if !native_component(name) {
            return Err(RelativeError::Refused);
        }
        let mut text: Vec<u16> = name.encode_utf16().collect();
        let length = u16::try_from(text.len() * 2).map_err(|_| RelativeError::Refused)?;
        let mut unicode = Unicode {
            length,
            maximum: length,
            buffer: text.as_mut_ptr(),
        };
        let mut oa = Attributes {
            length: size_of::<Attributes>() as u32,
            root: raw(parent),
            name: &mut unicode,
            flags: OBJ_CASE_INSENSITIVE | OBJ_DONT_REPARSE,
            descriptor: descriptor.map_or(null_mut(), |d| d.data),
            qos: null_mut(),
        };
        let mut io = IoStatus {
            status: 0,
            information: 0,
        };
        let mut h = null_mut();
        let access = READ_CONTROL_MASK
            | FILE_READ_ATTRIBUTES
            | SYNCHRONIZE_MASK
            | if directory {
                retained_directory_access()
            } else {
                FILE_GENERIC_READ
            }
            | if write {
                FILE_GENERIC_WRITE | DELETE_MASK
            } else {
                0
            };
        let options = NT_OPEN_REPARSE
            | FILE_SYNCHRONOUS_IO_NONALERT
            | if directory {
                FILE_DIRECTORY_FILE
            } else {
                FILE_NON_DIRECTORY_FILE
            };
        let status = unsafe {
            NtCreateFile(
                &mut h,
                access,
                &mut oa,
                &mut io,
                null(),
                0,
                FILE_SHARE_READ | if directory { FILE_SHARE_WRITE } else { 0 },
                disposition,
                options,
                null(),
                0,
            )
        };
        if status != 0 {
            return Err(if status as u32 == 0xc0000034 {
                RelativeError::Missing
            } else {
                RelativeError::Refused
            });
        }
        own(h).map_err(|_| RelativeError::Refused)
    }
    fn relative(
        parent: &File,
        name: &str,
        directory: bool,
        disposition: u32,
        descriptor: Option<&Descriptor>,
        write: bool,
    ) -> Result<File, String> {
        relative_status(parent, name, directory, disposition, descriptor, write)
            .map_err(|_| bad("relative open/create"))
    }
    fn hash(file: &File, limit: u64) -> Result<[u8; 32], String> {
        let mut reader = file;
        reader
            .seek(SeekFrom::Start(0))
            .map_err(|_| bad("hash seek"))?;
        let mut sum = Sha256::new();
        let mut buf = [0u8; 65536];
        let mut count = 0u64;
        loop {
            let n = reader.read(&mut buf).map_err(|_| bad("hash read"))?;
            if n == 0 {
                break;
            }
            count += n as u64;
            if count > limit {
                return Err(bad("image size"));
            }
            sum.update(&buf[..n]);
        }
        if count == 0 {
            return Err(bad("empty image"));
        }
        Ok(sum.finalize().into())
    }
    fn source_open(path: &Path) -> Result<File, String> {
        let name = wide(path)?;
        own(unsafe {
            CreateFileW(
                name.as_ptr(),
                FILE_GENERIC_READ | READ_CONTROL_MASK,
                FILE_SHARE_READ,
                null(),
                OPEN_EXISTING,
                FILE_FLAG_OPEN_REPARSE_POINT,
                null_mut(),
            )
        })
    }
    fn profile_chain(user: &User) -> Result<(PathBuf, Vec<Held>), String> {
        let profile = user.profile()?;
        let text = profile.to_str().ok_or_else(|| bad("profile encoding"))?;
        if text.len() < 4
            || text.as_bytes()[1] != b':'
            || text.as_bytes()[2] != b'\\'
            || !text.as_bytes()[0].is_ascii_alphabetic()
        {
            return Err(bad("native local profile shape"));
        }
        let parts: Vec<&str> = text[3..].split('\\').collect();
        if parts.len() > 16 || parts.iter().any(|p| !native_component(p)) {
            return Err(bad("profile components"));
        }
        let root = PathBuf::from(&text[..3]);
        let wide_root = wide(&root)?;
        let file = own(unsafe {
            CreateFileW(
                wide_root.as_ptr(),
                READ_CONTROL_MASK | FILE_READ_ATTRIBUTES | retained_directory_access(),
                FILE_SHARE_READ | FILE_SHARE_WRITE,
                null(),
                OPEN_EXISTING,
                FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT,
                null_mut(),
            )
        })?;
        let mut ancestors = vec![Held::capture(file, user, true, false)?];
        for part in parts {
            let file = relative(
                &ancestors.last().ok_or_else(|| bad("profile parent"))?.file,
                part,
                true,
                FILE_OPEN,
                None,
                false,
            )?;
            ancestors.push(Held::capture(file, user, true, false)?)
        }
        Ok((profile, ancestors))
    }
    fn rename(parent: &File, stage: &File, name: &str) -> Result<(), String> {
        if !component(name) {
            return Err(bad("publish name"));
        }
        let utf16: Vec<u16> = name.encode_utf16().collect();
        let bytes = 20 + utf16.len() * 2;
        let mut buffer = vec![0usize; bytes.div_ceil(size_of::<usize>())];
        let header = buffer.as_mut_ptr().cast::<Rename>();
        unsafe {
            (*header).replace = 0;
            (*header).root = raw(parent);
            (*header).length = (utf16.len() * 2) as u32;
            std::ptr::copy_nonoverlapping(
                utf16.as_ptr(),
                buffer.as_mut_ptr().cast::<u8>().add(20).cast::<u16>(),
                utf16.len(),
            );
        }
        let mut io = IoStatus {
            status: 0,
            information: 0,
        };
        if unsafe {
            NtSetInformationFile(
                raw(stage),
                &mut io,
                buffer.as_mut_ptr().cast(),
                bytes as u32,
                10,
            )
        } < 0
        {
            return Err(bad("non-replacing publish"));
        }
        Ok(())
    }
    static NEXT: AtomicU64 = AtomicU64::new(1);
    pub(super) fn expected_path(source: &VerifiedCacheSource) -> Result<PathBuf, String> {
        let user = User::current()?;
        Ok(user
            .profile()?
            .join(source.fixture_root.as_deref().unwrap_or(".orbwin-private"))
            .join("runtime")
            .join(format!(
                "{}-{}",
                source.role.cache_stem(),
                hex(&source.cache_key)
            ))
            .join(source.role.filename()))
    }
    fn publish_asset(
        parent: &File,
        name: &str,
        input: &mut File,
        before: Identity,
        expected: [u8; 32],
        limit: u64,
        user: &User,
        file_sd: &Descriptor,
    ) -> Result<Held, String> {
        let held = match relative_status(parent, name, false, FILE_OPEN, None, false) {
            Ok(file) => Held::capture(file, user, false, true)?,
            Err(RelativeError::Refused) => return Err(bad("existing bundle asset unavailable")),
            Err(RelativeError::Missing) => {
                let stamp = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .map_err(|_| bad("stage time"))?
                    .as_nanos();
                let stage_name = format!(
                    ".stage-{}-{stamp}-{}",
                    std::process::id(),
                    NEXT.fetch_add(1, Ordering::Relaxed)
                );
                let mut stage =
                    relative(parent, &stage_name, false, FILE_CREATE, Some(file_sd), true)?;
                Security::read(&stage)?.private(user, false)?;
                identity(&stage, false)?;
                input
                    .seek(SeekFrom::Start(0))
                    .map_err(|_| bad("source seek"))?;
                copy_pinned(input, &mut stage, expected, before.size, limit)?;
                stage.flush().map_err(|_| bad("stage flush"))?;
                stage.sync_all().map_err(|_| bad("stage sync"))?;
                if hash(&stage, limit)? != expected || identity(input, false)? != before {
                    return Err(bad("copy identity/hash"));
                }
                let id = identity(&stage, false)?;
                rename(parent, &stage, name)?;
                stage.sync_all().map_err(|_| bad("published sync"))?;
                drop(stage);
                let held = Held::capture(
                    relative(parent, name, false, FILE_OPEN, None, false)?,
                    user,
                    false,
                    true,
                )?;
                if held.id != id {
                    return Err(bad("published identity"));
                }
                held
            }
        };
        if hash(&held.file, limit)? != expected || identity(input, false)? != before {
            return Err(bad("final asset/source hash"));
        }
        Ok(held)
    }
    mod candidate_custody {
        include!("runtime_candidate_custody_windows.rs");
    }
    #[derive(Clone)]
    pub(super) struct CandidateLease {
        inner: candidate_custody::CandidateLease,
    }
    impl CandidateLease {
        pub(super) fn new() -> Self {
            Self {
                inner: candidate_custody::CandidateLease::new(),
            }
        }
        pub(super) fn prepare_or_recheck(&self) -> Result<(), String> {
            self.inner.prepare_or_recheck()
        }
    }

    #[derive(Clone)]
    pub(super) struct RddLease {
        inner: candidate_custody::CandidateLease,
    }
    impl RddLease {
        pub(super) fn new() -> Self {
            Self {
                inner: candidate_custody::CandidateLease::ordinary(),
            }
        }
        pub(super) fn prepare_or_recheck(&self) -> Result<(), String> {
            self.inner.prepare_or_recheck()
        }
        pub(super) fn recheck(&self, full: bool) -> Result<(), String> {
            self.inner.recheck(full)
        }
        pub(super) fn data(&self) -> Result<(PathBuf, serde_json::Value), String> {
            self.inner.ordinary_data()
        }
        pub(super) fn verify_original_process(
            &self,
            process: std::os::windows::io::BorrowedHandle<'_>,
            expected: Option<(u32, u64, u32)>,
        ) -> Result<(), String> {
            self.inner.verify_original_process(process, expected)
        }
    }
    fn prepare_ordinary_rdd(source: VerifiedCacheSource) -> Result<CachedExecutable, String> {
        if source.fixture_root.is_some()
            || source.cache_key != source.expected
            || source.expected != digest(&crate::runtime::component_binary_pin("rdd.exe")?)?
        {
            return Err(bad("ordinary Rdd selector"));
        }
        static RDD: std::sync::OnceLock<
            std::sync::Mutex<custody_model::RetainedAttempt<RddLease>>,
        > = std::sync::OnceLock::new();
        let mut memo = RDD
            .get_or_init(Default::default)
            .lock()
            .map_err(|_| bad("ordinary Rdd owner poisoned"))?;
        let owner = if let Some(owner) = memo.captured() {
            owner
        } else {
            let owner = std::sync::Arc::new(RddLease::new());
            if !memo.publish(owner.clone()) {
                return Err(bad("ordinary Rdd owner publication"));
            }
            owner
        };
        owner.prepare_or_recheck()?;
        let (path, _) = owner.data()?;
        Ok(CachedExecutable {
            path,
            lease: NativeLease::Rdd(owner.as_ref().clone()),
        })
    }

    pub(super) fn prepare(source: VerifiedCacheSource) -> Result<CachedExecutable, String> {
        if matches!(source.role, Role::Rdd) && source.fixture_root.is_none() {
            return prepare_ordinary_rdd(source);
        }
        let user = User::current()?;
        // Source ancestors are deliberately not trusted. The immutable held
        // bytes must equal the independent app pin before any execution/copy.
        let mut input = source_open(&source.source)?;
        let source_before = identity(&input, false)?;
        if source_before.size == 0
            || source_before.size > source.role.limit()
            || hash(&input, source.role.limit())? != source.expected
        {
            return Err(bad("source image pin"));
        }
        // Open and pin the complete fixed bundle before creating cache state.
        // No path or expected digest comes from the mutable JSON manifest.
        let mut bundle_inputs = Vec::new();
        for asset in &source.bundle {
            let file = source_open(&asset.source)?;
            let before = identity(&file, false)?;
            if before.size == 0
                || before.size > asset.limit
                || (asset.filename == "assets.sig" && before.size != 88)
                || hash(&file, asset.limit)? != asset.expected
            {
                return Err(bad("source bundle pin"));
            }
            bundle_inputs.push((asset, file, before));
        }
        let (profile, mut ancestors) = profile_chain(&user)?;
        let root_name = source.fixture_root.as_deref().unwrap_or(".orbwin-private");
        let dir_sd = Descriptor::private(&user, true)?;
        let file_sd = Descriptor::private(&user, false)?;
        let role_name = format!("{}-{}", source.role.cache_stem(), hex(&source.cache_key));
        let mut path = profile;
        for name in [root_name, "runtime", &role_name] {
            let parent = &ancestors.last().ok_or_else(|| bad("private parent"))?.file;
            let disposition = if source.fixture_root.is_some() && name == root_name {
                FILE_CREATE
            } else {
                FILE_OPEN_IF
            };
            let file = relative(parent, name, true, disposition, Some(&dir_sd), false)?;
            ancestors.push(Held::capture(file, &user, true, true)?);
            path.push(name)
        }
        let parent = &ancestors.last().ok_or_else(|| bad("cache parent"))?.file;
        let image = match relative_status(
            parent,
            source.role.filename(),
            false,
            FILE_OPEN,
            None,
            false,
        ) {
            Ok(file) => Held::capture(file, &user, false, true)?,
            Err(RelativeError::Refused) => return Err(bad("existing cached image unavailable")),
            Err(RelativeError::Missing) => {
                let stamp = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .map_err(|_| bad("stage time"))?
                    .as_nanos();
                let name = format!(
                    ".stage-{}-{stamp}-{}",
                    std::process::id(),
                    NEXT.fetch_add(1, Ordering::Relaxed)
                );
                let mut stage = relative(parent, &name, false, FILE_CREATE, Some(&file_sd), true)?;
                // Verify private policy BEFORE copying even public executable bytes.
                Security::read(&stage)?.private(&user, false)?;
                identity(&stage, false)?;
                input
                    .seek(SeekFrom::Start(0))
                    .map_err(|_| bad("source seek"))?;
                copy_pinned(
                    &mut input,
                    &mut stage,
                    source.expected,
                    source_before.size,
                    source.role.limit(),
                )?;
                stage.flush().map_err(|_| bad("stage flush"))?;
                stage.sync_all().map_err(|_| bad("stage sync"))?;
                if hash(&stage, source.role.limit())? != source.expected
                    || identity(&input, false)? != source_before
                {
                    return Err(bad("copy identity/hash"));
                }
                let stage_id = identity(&stage, false)?;
                rename(parent, &stage, source.role.filename())?;
                stage.sync_all().map_err(|_| bad("published sync"))?;
                drop(stage);
                // Reopen after closing the write file object; exact identity,
                // private descriptor and hash must match before admission.
                let result = relative(
                    parent,
                    source.role.filename(),
                    false,
                    FILE_OPEN,
                    None,
                    false,
                )?;
                let held = Held::capture(result, &user, false, true)?;
                if held.id != stage_id {
                    return Err(bad("published identity"));
                }
                held
            }
        };
        if hash(&image.file, source.role.limit())? != source.expected
            || identity(&input, false)? != source_before
        {
            return Err(bad("final image/source hash"));
        }
        let mut bundle = Vec::new();
        if !bundle_inputs.is_empty() {
            let subdirectory_name = bundle_inputs
                .iter()
                .find_map(|(asset, _, _)| asset.directory);
            if bundle_inputs.iter().any(|(asset, _, _)| {
                asset.directory.is_some() && asset.directory != subdirectory_name
            }) {
                return Err(bad("mixed bundle directory roles"));
            }
            let subdirectory = if let Some(name) = subdirectory_name {
                Some(Held::capture(
                    relative(parent, name, true, FILE_OPEN_IF, Some(&dir_sd), false)?,
                    &user,
                    true,
                    true,
                )?)
            } else {
                None
            };
            for (asset, mut file, before) in bundle_inputs {
                let parent = if asset.directory.is_some() {
                    &subdirectory
                        .as_ref()
                        .ok_or_else(|| bad("missing LAN bundle parent"))?
                        .file
                } else {
                    parent
                };
                let cached = publish_asset(
                    parent,
                    asset.filename,
                    &mut file,
                    before,
                    asset.expected,
                    asset.limit,
                    &user,
                    &file_sd,
                )?;
                bundle.push((cached, asset.expected, asset.limit));
            }
            if let Some(held) = subdirectory {
                ancestors.push(held);
            }
        }
        path.push(source.role.filename());
        let lease = Lease {
            sid: user.sid,
            ancestors,
            image,
            expected: source.expected,
            limit: source.role.limit(),
            gate: std::sync::Mutex::new(()),
            bundle,
        };
        lease.recheck()?;
        Ok(CachedExecutable {
            path,
            lease: NativeLease::Legacy(std::sync::Arc::new(lease)),
        })
    }
}

#[cfg(test)]
mod tests {
    #[cfg(windows)]
    #[test]
    fn retained_cache_directories_participate_in_sharing_without_listing() {
        use windows_sys::Win32::Storage::FileSystem::{FILE_LIST_DIRECTORY, FILE_TRAVERSE};
        assert_ne!(
            super::native::retained_directory_access() & FILE_TRAVERSE,
            0
        );
        assert_eq!(
            super::native::retained_directory_access() & FILE_LIST_DIRECTORY,
            0
        );
    }
    use super::*;
    #[test]
    fn pin_and_component_bounds_are_closed() {
        for value in ["", "00", "A".repeat(64).as_str(), "0".repeat(64).as_str()] {
            assert!(digest(value).is_err())
        }
        assert!(digest(&"a".repeat(64)).is_ok());
        for name in [
            "",
            ".",
            "..",
            "other/path",
            "other\\path",
            "name.",
            "name ",
            "%TEMP%",
        ] {
            assert!(!component(name))
        }
        assert!(component("rdd-0123456789abcdef"));
    }
    #[test]
    fn opaque_records_redact_origin() {
        let source = VerifiedCacheSource {
            role: Role::Rdd,
            source: PathBuf::from("private-source-marker"),
            expected: [1; 32],
            fixture_root: None,
            bundle: Vec::new(),
            cache_key: [1; 32],
        };
        assert!(!format!("{source:?}").contains("private-source-marker"));
    }
    #[test]
    fn native_profile_components_and_lease_thread_safety() {
        for value in ["Jane Doe", "开发者", "Users"] {
            assert!(native_component(value))
        }
        for value in ["..", "A:B", "CON", "lpt1.txt", "x\\y", "x/../y", "name."] {
            assert!(!native_component(value))
        }
        fn send_sync<T: Send + Sync>() {}
        send_sync::<CachedExecutable>();
    }
    #[test]
    fn copy_refuses_growth_truncation_wrong_hash_and_failed_writer() {
        use sha2::{Digest, Sha256};
        let bytes = b"immutable public executable fixture";
        let expected: [u8; 32] = Sha256::digest(bytes).into();
        let mut out = Vec::new();
        copy_pinned(
            &mut bytes.as_slice(),
            &mut out,
            expected,
            bytes.len() as u64,
            1024,
        )
        .unwrap();
        assert_eq!(out, bytes);
        for (size, pin) in [
            (bytes.len() as u64 + 1, expected),
            (bytes.len() as u64 - 1, expected),
            (bytes.len() as u64, [1; 32]),
        ] {
            assert!(copy_pinned(&mut bytes.as_slice(), &mut Vec::new(), pin, size, 1024).is_err())
        }
        struct Failed;
        impl std::io::Write for Failed {
            fn write(&mut self, _: &[u8]) -> std::io::Result<usize> {
                Err(std::io::Error::other("injected"))
            }
            fn flush(&mut self) -> std::io::Result<()> {
                Ok(())
            }
        }
        assert!(copy_pinned(
            &mut bytes.as_slice(),
            &mut Failed,
            expected,
            bytes.len() as u64,
            1024
        )
        .is_err());
    }
    #[cfg(windows)]
    #[test]
    fn reviewed_native_cache_fixture() {
        if std::env::var("ORBWIN_CACHE_FIXTURE_REVIEWED").as_deref() != Ok("yes") {
            return;
        }
        let nonce = std::env::var("ORBWIN_CACHE_FIXTURE_NONCE").expect("closed nonce");
        assert_eq!(nonce.len(), 32);
        assert!(nonce
            .bytes()
            .all(|b| b.is_ascii_hexdigit() && !b.is_ascii_uppercase()));
        let pin = digest(
            &std::env::var("ORBWIN_CACHE_FIXTURE_BINARY_SHA256").expect("frozen test image pin"),
        )
        .expect("closed pin");
        let source = VerifiedCacheSource {
            role: Role::Fixture,
            source: std::env::current_exe().expect("test image"),
            expected: pin,
            fixture_root: Some(format!(".orbwin-cache-proof-{nonce}")),
            bundle: Vec::new(),
            cache_key: pin,
        };
        let cached = prepare(source).expect("reviewed private source/copy/publish/cache recheck");
        cached.recheck().expect("retained cache recheck");
        assert!(cached.path().ends_with("fixture.exe"));
        println!("CACHE_FIXTURE_COPY_AND_RECHECK_PASSED");
        // No cached executable is launched. The exact private nonce fixture is
        // retained as evidence; deletion is a separate reviewed cleanup action.
    }
}
