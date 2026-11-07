// Included INSIDE client_config::native: reuse private primitives, never expose
// a File/identity/DTO constructor which grants production admission.
pub(super) mod import_owner {
    use super::super::legacy_import as codec;
    use super::*;
    const BOOT_READ: u32 =
        FILE_READ_DATA | FILE_READ_ATTRIBUTES | READ_CONTROL_MASK | SYNCHRONIZE_MASK;
    const BOOT_WRITE: u32 = BOOT_READ | FILE_WRITE_DATA | FILE_APPEND_DATA | FILE_WRITE_ATTRIBUTES;
    // Bootstrap and shared native-output temporaries use this panic guard.
    // Do not let a local
    // FILE_OBJECT close before the outer strong graph observes unwinding.
    struct BootstrapFile(Option<CheckedFile>);
    impl std::ops::Deref for BootstrapFile {
        type Target = File;
        fn deref(&self) -> &File {
            self.0.as_ref().unwrap()
        }
    }
    impl TakeFile for BootstrapFile {
        fn take_file(mut self) -> File {
            self.0.take().unwrap().take_file()
        }
    }
    impl Drop for BootstrapFile {
        fn drop(&mut self) {
            if std::thread::panicking() || writer_resources_known().is_err() {
                retain_unknown();
                if let Some(f) = self.0.take() {
                    std::mem::forget(f)
                }
            }
        }
    }
    fn capture_output(handle: HANDLE) -> Option<BootstrapFile> {
        if handle.is_null() || handle == INVALID_HANDLE_VALUE {
            None
        } else {
            Some(BootstrapFile(Some(CheckedFile(Some(unsafe {
                File::from_raw_handle(handle)
            })))))
        }
    }
    fn uncertain_output(output: Option<BootstrapFile>) -> ClientConfigError {
        retain_unknown();
        if let Some(cap) = output {
            std::mem::forget(cap)
        }
        err(ErrorKind::RecoveryRequired)
    }
    // Narrow shared-opener hardening: capture before evaluating status so an
    // error with a real output can NEVER establish Missing/Sharing/absence.
    // NT_SUCCESS is signed >=0, including informational NTSTATUS values:
    // https://learn.microsoft.com/en-us/windows-hardware/drivers/kernel/using-ntstatus-values
    pub(super) fn relative_output(status: i32, handle: HANDLE) -> Result<File, RelativeError> {
        let output = capture_output(handle);
        codec::native_output(
            status >= 0,
            output,
            |cap| {
                retain_unknown();
                if let Some(cap) = cap {
                    std::mem::forget(cap)
                };
                RelativeError::Refused(bad("relative native output unknown"))
            },
            || {
                if status as u32 == 0xc0000043 {
                    RelativeError::Sharing
                } else if status as u32 == 0xc0000034 {
                    RelativeError::Missing
                } else {
                    RelativeError::Refused(NativeDiagnostic {
                        stage: "relative native open/create",
                        system_error: Some(status as u32),
                        security: None,
                    })
                }
            },
        )
        .map(TakeFile::take_file)
    }
    #[link(name = "ntdll")]
    extern "system" {
        fn NtQueryObject(
            handle: *mut c_void,
            class: u32,
            buffer: *mut c_void,
            length: u32,
            returned: *mut u32,
        ) -> i32;
    }
    fn mapped(e: RelativeError) -> ClientConfigError {
        match e {
            RelativeError::Sharing => err(ErrorKind::Busy),
            e => explain(ErrorKind::RecoveryRequired, relative_diagnostic(e)),
        }
    }
    fn exact_boot_open(
        parent: &File,
        disposition: u32,
        user: &User,
    ) -> Result<File, ClientConfigError> {
        let descriptor = Descriptor::private(user, false).map_err(fresh_failure)?;
        let name = "client.create-intent";
        let mut chars: Vec<u16> = name.encode_utf16().collect();
        let mut text = Unicode {
            length: (chars.len() * 2) as u16,
            maximum: (chars.len() * 2) as u16,
            buffer: chars.as_mut_ptr(),
        };
        let mut attrs = Attributes {
            length: size_of::<Attributes>() as u32,
            root: raw(parent),
            name: &mut text,
            flags: OBJ_CASE_INSENSITIVE | OBJ_DONT_REPARSE,
            descriptor: if disposition == FILE_CREATE {
                descriptor.data
            } else {
                null_mut()
            },
            qos: null_mut(),
        };
        let mut status = IoStatus {
            status: 0,
            information: 0,
        };
        let mut handle = null_mut();
        let result = unsafe {
            NtCreateFile(
                &mut handle,
                BOOT_WRITE,
                &mut attrs,
                &mut status,
                null(),
                0,
                0,
                disposition,
                NT_OPEN_REPARSE | FILE_SYNCHRONOUS_IO_NONALERT | FILE_NON_DIRECTORY_FILE,
                null(),
                0,
            )
        };
        let output = capture_output(handle);
        codec::native_output(result >= 0, output, uncertain_output, || {
            if result as u32 == 0xc0000043 || result as u32 == 0xc0000035 {
                err(ErrorKind::Busy)
            } else {
                explain(
                    ErrorKind::RecoveryRequired,
                    bad("bootstrap exact native open"),
                )
            }
        })
        .map(TakeFile::take_file)
    }
    fn reduced(file: &File) -> Result<File, ClientConfigError> {
        let mut handle = null_mut();
        let result = unsafe {
            DuplicateHandle(
                GetCurrentProcess(),
                raw(file),
                GetCurrentProcess(),
                &mut handle,
                BOOT_READ,
                0,
                0,
            )
        };
        let output = capture_output(handle);
        let reduced = codec::native_output(result != 0, output, uncertain_output, || {
            err(ErrorKind::RecoveryRequired)
        })?;
        let mut info = [0u32; 14];
        let mut returned = 0;
        let result = unsafe {
            NtQueryObject(
                raw(&reduced),
                0,
                info.as_mut_ptr().cast(),
                std::mem::size_of_val(&info) as u32,
                &mut returned,
            )
        };
        if result != 0
            || returned != std::mem::size_of_val(&info) as u32
            || info[1] != BOOT_READ
            || unsafe { CompareObjectHandles(raw(file), raw(&reduced)) } == 0
        {
            close_file(reduced).map_err(fresh_failure)?;
            return Err(err(ErrorKind::RecoveryRequired));
        };
        Ok(reduced.take_file())
    }
    fn fid(id: Identity) -> codec::Fid {
        codec::Fid {
            volume: id.volume,
            high: id.high,
            low: id.low,
        }
    }
    // Windows may finalize LastWriteTime at the last writable close. Capture the
    // immutable snapshot AFTER that checked close; identity/shape/SD/body must
    // already match the pre-close observations independently.
    fn same_closed_file(a: Identity, b: Identity) -> bool {
        fid(a) == fid(b) && a.attributes == b.attributes && a.links == b.links && a.size == b.size
    }
    fn witness(data: &Data, user: &User) -> Result<codec::Witness, ClientConfigError> {
        data.check(user).map_err(fresh_failure)?;
        let bytes = body(&data.file).map_err(fresh_failure)?;
        Ok(codec::Witness {
            id: fid(data.id),
            sd_sha256: hex(&data.security),
            body_sha256: codec::hash(&bytes),
            bytes: bytes.len() as u64,
        })
    }
    fn same_witness(
        data: &Data,
        expected: &codec::Witness,
        user: &User,
    ) -> Result<(), ClientConfigError> {
        if witness(data, user)? != *expected {
            return Err(err(ErrorKind::RecoveryRequired));
        };
        Ok(())
    }
    pub(super) struct Bootstrap {
        held: Option<CheckedFile>,
        id: Identity,
        security: [u8; 32],
        bytes: Vec<u8>,
        mutation_live: bool,
    }
    impl Bootstrap {
        // Read-reduction and checked writable close occur before client/control effects.
        pub(super) fn claim(
            parent: &File,
            user: &User,
            create: bool,
        ) -> Result<Self, ClientConfigError> {
            let writer = BootstrapFile(Some(CheckedFile(Some(exact_boot_open(
                parent,
                if create { FILE_CREATE } else { FILE_OPEN },
                user,
            )?))));
            let id = identity(&writer, false).map_err(fresh_failure)?;
            let sd = Security::read_file(&writer).map_err(fresh_failure)?;
            sd.private(user, false).map_err(fresh_failure)?;
            let bytes = body(&writer).map_err(fresh_failure)?;
            if codec::marker_kind(&bytes) == codec::MarkerKind::Unsupported {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let read = reduced(&writer)?;
            let mut result = Self {
                held: Some(CheckedFile(Some(read))),
                id,
                security: sd.hash(),
                bytes,
                mutation_live: false,
            };
            if let Err(e) = close_file(writer) {
                retain_unknown();
                result.retain();
                return Err(fresh_failure(e));
            };
            let after = identity(result.held.as_ref().unwrap(), false).map_err(fresh_failure)?;
            if !same_closed_file(after, result.id) {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            result.id = after;
            result.check(user)?;
            Ok(result)
        }
        fn check(&self, user: &User) -> Result<(), ClientConfigError> {
            writer_resources_known()?;
            let file = self
                .held
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            if identity(file, false).map_err(fresh_failure)? != self.id {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            let sd = Security::read_file(file).map_err(fresh_failure)?;
            sd.private(user, false).map_err(fresh_failure)?;
            if sd.hash() != self.security || body(file).map_err(fresh_failure)? != self.bytes {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            Ok(())
        }
        fn retain(&mut self) {
            if let Some(file) = self.held.take() {
                std::mem::forget(file)
            }
        }
        // Caller holds actual exclusive controller. A pre-write sharing refusal is
        // known Busy; unlike an uncertain write/close it does NOT poison the process.
        fn select_import(&mut self, parent: &File, user: &User) -> Result<(), ClientConfigError> {
            self.check(user)?;
            if !codec::MARKER.starts_with(&self.bytes) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            close_file(self.held.take().unwrap()).map_err(fresh_failure)?;
            let opened = exact_boot_open(parent, FILE_OPEN, user)?;
            self.held = Some(CheckedFile(Some(opened)));
            self.check(user)?;
            if self.bytes != codec::MARKER {
                let file = self.held.as_ref().unwrap();
                let prefix = body(file).map_err(fresh_failure)?;
                if prefix != self.bytes {
                    return Err(err(ErrorKind::RecoveryRequired));
                };
                let mut writer: &File = file;
                writer.seek(SeekFrom::End(0)).map_err(fresh_failure)?;
                self.mutation_live = true;
                let result = (|| -> Result<(), ClientConfigError> {
                    writer
                        .write_all(&codec::MARKER[prefix.len()..])
                        .map_err(fresh_failure)?;
                    writer.sync_all().map_err(fresh_failure)?;
                    if body(file).map_err(fresh_failure)? != codec::MARKER {
                        return Err(err(ErrorKind::RecoveryRequired));
                    };
                    let actual = identity(file, false).map_err(fresh_failure)?;
                    if fid(actual) != fid(self.id)
                        || Security::read_file(file).map_err(fresh_failure)?.hash() != self.security
                    {
                        return Err(err(ErrorKind::RecoveryRequired));
                    };
                    self.id = actual;
                    self.bytes = codec::MARKER.to_vec();
                    Ok(())
                })();
                if let Err(e) = result {
                    retain_unknown();
                    self.retain();
                    return Err(e);
                }
            }
            close_file(self.held.take().unwrap()).map_err(fresh_failure)?;
            // No marker mutation can be admitted by another cooperative bootstrap while
            // this same controller is held. Known read-open Busy preserves pending bytes.
            let file = relative_status(
                parent,
                "client.create-intent",
                false,
                FILE_OPEN,
                None,
                false,
            )
            .map_err(mapped)?;
            self.held = Some(CheckedFile(Some(file)));
            let after = identity(self.held.as_ref().unwrap(), false).map_err(fresh_failure)?;
            if !same_closed_file(after, self.id) {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            self.id = after;
            self.check(user)?;
            self.mutation_live = false;
            Ok(())
        }
        fn close(&mut self) -> Result<(), ClientConfigError> {
            if let Some(file) = self.held.take() {
                close_file(file).map_err(fresh_failure)?
            };
            Ok(())
        }
        pub(super) fn into_fresh(
            mut self,
            parent: &File,
            user: &User,
            expected: (Identity, [u8; 32]),
            bytes: &[u8],
        ) -> Result<Data, ClientConfigError> {
            self.check(user)?;
            if (self.id, self.security) != expected || self.bytes != bytes {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            // Actual exclusive controller is already held by the caller. This is a
            // known no-write handoff; an open Busy leaves the marker and tree intact.
            self.close()?;
            let file = relative_status(
                parent,
                "client.create-intent",
                false,
                FILE_OPEN,
                None,
                false,
            )
            .map_err(mapped)?;
            self.held = Some(CheckedFile(Some(file)));
            self.check(user)?;
            Ok(Data {
                file: self.held.take().unwrap(),
                id: self.id,
                security: self.security,
                writer: false,
            })
        }
    }
    impl Drop for Bootstrap {
        fn drop(&mut self) {
            if std::thread::panicking() || self.mutation_live || writer_resources_known().is_err() {
                retain_unknown();
                self.retain()
            }
        }
    }
    // Fresh recovery shares ONLY the early bootstrap capability handoff. Own
    // the real user/roles/ancestry before that first new native close, even
    // before the client root/controller exists. No constructor from DTO facts.
    pub(super) struct FreshPrelude {
        pub(super) fresh: Option<Box<FreshInitialization>>,
        pub(super) bootstrap: Option<Bootstrap>,
    }
    impl FreshPrelude {
        fn retain(&mut self) {
            if let Some(v) = self.fresh.take() {
                std::mem::forget(v)
            }
            if let Some(v) = self.bootstrap.take() {
                std::mem::forget(v)
            }
        }
        pub(super) fn ready(mut self) -> Box<FreshInitialization> {
            assert!(self.bootstrap.is_none());
            self.fresh.take().unwrap()
        }
    }
    impl Drop for FreshPrelude {
        fn drop(&mut self) {
            if std::thread::panicking() || writer_resources_known().is_err() {
                retain_unknown();
                self.retain();
                return;
            }
            if let Some(mut bootstrap) = self.bootstrap.take() {
                if bootstrap.close().is_err() {
                    std::mem::forget(bootstrap);
                    self.retain();
                    return;
                }
            }
            if let Some(mut fresh) = self.fresh.take() {
                if fresh.release().is_err() {
                    std::mem::forget(fresh);
                    self.retain()
                }
            }
        }
    }
    // Captured from the token's fixed historical OrbWin public application path.
    // No caller supplies a source path; the global Docker directory is unreachable.
    include!("client_config_import_route.rs");
    // Only a complete actual held route can perform the first source-body read.
    struct Source {
        route: source_route::Completed,
        bytes: Vec<u8>,
        witness: codec::Witness,
    }
    impl Source {
        fn capture(user: &User) -> Result<Self, ClientConfigError> {
            let route = source_route::Draft::known_local(user)?.source(user)?;
            let bytes = route.read(user)?;
            if bytes.is_empty() {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            let file = route.source();
            let witness = codec::Witness {
                id: fid(file.id),
                sd_sha256: hex(&file.security),
                body_sha256: codec::hash(&bytes),
                bytes: bytes.len() as u64,
            };
            let source = Self {
                route,
                bytes,
                witness,
            };
            source.check(user)?;
            Ok(source)
        }
        fn check(&self, user: &User) -> Result<(), ClientConfigError> {
            if self.route.read(user)? != self.bytes {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            Ok(())
        }
        fn close(self) -> Result<(), ClientConfigError> {
            self.route.close()
        }
    }

    const RECORDS: [&str; 4] = [
        "client.import-intent",
        "client.import-original",
        "client.import-source",
        "client.import-target",
    ];
    fn root_allowed(name: &str) -> bool {
        matches!(
            name,
            "controller.lock"
                | "client.init"
                | "docker"
                | "client.import-intent"
                | "client.import-original"
                | "client.import-source"
                | "client.import-target"
        )
    }
    pub(in crate::client_config) struct Owner {
        user: Option<User>,
        ancestors: Vec<Held>,
        controller: Option<Data>,
        bootstrap: Option<Bootstrap>,
        source: Option<Source>,
        records: std::collections::BTreeMap<&'static str, Data>,
        init: Option<Data>,
        config: Option<Data>,
        stage: Option<Data>,
        docker_dir: Option<Held>,
        pending: Option<CheckedFile>,
        mutation_live: bool,
        profile: PathBuf,
        plugins: PathBuf,
        docker: Option<crate::runtime_cache::CachedExecutable>,
        credentials: Option<crate::runtime_cache::CachedExecutable>,
        intent: Option<codec::Intent>,
        intent_bytes: Vec<u8>,
        original: Vec<u8>,
        scratch: codec::Scratch,
        action: codec::ImportAction,
        restart_uncommitted: bool,
        completed_summary: bool, // Diagnostic only; Apply revalidates actual caps/record.
    }
    impl Owner {
        pub(in crate::client_config) fn prepare(
            docker: crate::runtime_cache::CachedExecutable,
            credentials: crate::runtime_cache::CachedExecutable,
            action: codec::ImportAction,
        ) -> Result<Box<Self>, ClientConfigError> {
            writer_resources_known()?;
            docker.check_held_origin().map_err(fresh_failure)?;
            credentials.check_held_origin().map_err(fresh_failure)?;
            if docker.path().file_name() != Some(std::ffi::OsStr::new("docker.exe"))
                || credentials.path().file_name()
                    != Some(std::ffi::OsStr::new("docker-credential-orbwin.exe"))
            {
                return Err(err(ErrorKind::Unavailable));
            }
            let plugins = docker
                .path()
                .parent()
                .ok_or_else(|| err(ErrorKind::Unavailable))?
                .join("docker-cli-plugins");
            let user = User::current().map_err(fresh_failure)?;
            let mut own = Box::new(Self {
                user: Some(user),
                ancestors: Vec::new(),
                controller: None,
                bootstrap: None,
                source: None,
                records: Default::default(),
                init: None,
                config: None,
                stage: None,
                docker_dir: None,
                pending: None,
                mutation_live: false,
                profile: PathBuf::new(),
                plugins,
                docker: Some(docker),
                credentials: Some(credentials),
                intent: None,
                intent_bytes: Vec::new(),
                original: Vec::new(),
                scratch: codec::Scratch {
                    docker: None,
                    init: None,
                    config: None,
                },
                action,
                restart_uncommitted: false,
                completed_summary: false,
            });
            let (profile, ancestors) = profile_chain(own.user()).map_err(fresh_failure)?;
            own.profile = profile;
            own.ancestors = ancestors;
            let sd = Descriptor::private(own.user(), true).map_err(fresh_failure)?;
            let private = relative_status(
                &own.ancestors.last().unwrap().file,
                ".orbwin-private",
                true,
                FILE_OPEN_IF,
                Some(&sd),
                false,
            )
            .map_err(mapped)?;
            own.ancestors
                .push(Held::capture(private, own.user(), true, true).map_err(fresh_failure)?);
            let private = &own.ancestors.last().unwrap().file;
            let marker = fresh_optional(private, "client.create-intent", false, false)?;
            let existing = fresh_optional(private, "client", true, false)?;
            // A marker-absent existing client is an admitted/unknown owner, never import
            // scratch. Do not manufacture a marker in that namespace.
            if marker.is_none() && existing.is_some() {
                if let Some(file) = existing {
                    close_file(file).map_err(fresh_failure)?
                };
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let create = marker.is_none();
            if let Some(file) = marker {
                close_file(file).map_err(fresh_failure)?
            }
            own.bootstrap = Some(Bootstrap::claim(private, own.user(), create)?);
            if matches!(
                codec::marker_kind(&own.bootstrap.as_ref().unwrap().bytes),
                codec::MarkerKind::Fresh | codec::MarkerKind::Unsupported
            ) {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            let root = if let Some(file) = existing {
                file
            } else {
                relative_status(
                    &own.ancestors.last().unwrap().file,
                    "client",
                    true,
                    FILE_CREATE,
                    Some(&sd),
                    false,
                )
                .map_err(mapped)?
            };
            own.ancestors
                .push(Held::capture(root, own.user(), true, true).map_err(fresh_failure)?);
            let sd = Descriptor::private(own.user(), false).map_err(fresh_failure)?;
            let file = relative_status(
                &own.root().file,
                "controller.lock",
                false,
                FILE_OPEN_IF,
                Some(&sd),
                false,
            )
            .map_err(mapped)?;
            let controller = Data::capture(file, own.user()).map_err(fresh_failure)?;
            if !body(&controller.file).map_err(fresh_failure)?.is_empty() {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            own.controller = Some(controller);
            for name in RECORDS {
                if let Some(data) = fresh_record(&own.root().file, name, own.user(), false)? {
                    own.records.insert(name, data);
                }
            }
            own.load()?;
            own.check()?;
            Ok(own)
        }
        fn user(&self) -> &User {
            self.user.as_ref().unwrap()
        }
        fn root(&self) -> &Held {
            self.ancestors.last().unwrap()
        }
        fn private(&self) -> &File {
            &self.ancestors[self.ancestors.len() - 2].file
        }
        fn dir(&self) -> &File {
            &self.docker_dir.as_ref().unwrap().file
        }
        fn check(&self) -> Result<(), ClientConfigError> {
            writer_resources_known()?;
            if self.mutation_live {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            self.check_retained()
        }
        fn check_retained(&self) -> Result<(), ClientConfigError> {
            // Review and Apply may run on different blocking threads. Recheck
            // the actual ordinary, non-impersonated current caller; a stored
            // token/SID alone does not authorize native effects on this thread.
            let caller = User::current().map_err(fresh_failure)?;
            let same_caller =
                caller.sid == self.user().sid && caller.session == self.user().session;
            close_file(caller.token).map_err(fresh_failure)?;
            if !same_caller {
                return Err(err(ErrorKind::Unavailable));
            }
            for dir in &self.ancestors {
                dir.recheck(self.user()).map_err(fresh_failure)?
            }
            for data in self
                .records
                .values()
                .chain(self.controller.iter())
                .chain(self.init.iter())
                .chain(self.config.iter())
                .chain(self.stage.iter())
            {
                data.check(self.user()).map_err(fresh_failure)?
            }
            if let Some(dir) = &self.docker_dir {
                dir.recheck(self.user()).map_err(fresh_failure)?
            }
            if let Some(source) = &self.source {
                source.check(self.user())?
            }
            self.bootstrap.as_ref().unwrap().check(self.user())?;
            self.docker
                .as_ref()
                .unwrap()
                .check_held_origin()
                .map_err(fresh_failure)?;
            self.credentials
                .as_ref()
                .unwrap()
                .check_held_origin()
                .map_err(fresh_failure)?;
            Ok(())
        }
        fn retain(&mut self) {
            macro_rules! forget {
                ($field:ident) => {
                    if let Some(v) = self.$field.take() {
                        std::mem::forget(v)
                    }
                };
            }
            forget!(user);
            forget!(controller);
            forget!(bootstrap);
            forget!(source);
            forget!(init);
            forget!(config);
            forget!(stage);
            forget!(docker_dir);
            forget!(pending);
            forget!(docker);
            forget!(credentials);
            std::mem::forget(std::mem::take(&mut self.ancestors));
            std::mem::forget(std::mem::take(&mut self.records));
        }
        pub(in crate::client_config) fn release(&mut self) -> Result<(), ClientConfigError> {
            if std::thread::panicking()
                || self.mutation_live
                || self.bootstrap.as_ref().is_some_and(|b| b.mutation_live)
                || writer_resources_known().is_err()
            {
                retain_unknown();
                self.retain();
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let result = (|| {
                if let Some(f) = self.pending.take() {
                    close_file(f).map_err(fresh_failure)?
                }
                for data in [&mut self.stage, &mut self.config, &mut self.init] {
                    if let Some(v) = data.take() {
                        close_file(v.file).map_err(fresh_failure)?
                    }
                }
                while let Some((_, v)) = self.records.pop_last() {
                    close_file(v.file).map_err(fresh_failure)?
                }
                if let Some(source) = self.source.take() {
                    source.close()?
                }
                if let Some(bootstrap) = self.bootstrap.as_mut() {
                    bootstrap.close()?
                };
                self.bootstrap.take();
                if let Some(dir) = self.docker_dir.take() {
                    close_file(dir.file).map_err(fresh_failure)?
                }
                while let Some(dir) = self.ancestors.pop() {
                    close_file(dir.file).map_err(fresh_failure)?
                }
                if let Some(user) = self.user.take() {
                    close_file(user.token).map_err(fresh_failure)?
                }
                if let Some(data) = self.controller.take() {
                    close_file(data.file).map_err(fresh_failure)?
                }
                self.docker.take();
                self.credentials.take();
                Ok(())
            })();
            if result.is_err() {
                self.retain()
            };
            result
        }
        // Source bytes never flow through a JSON Value or a native child. This method
        // is called by BOTH Review and Apply before admitting any new target.
        fn capture_scratch(&mut self) -> Result<codec::Scratch, ClientConfigError> {
            let names = fresh_inventory(
                self.private(),
                "client",
                self.root(),
                self.user(),
                root_allowed,
                7,
            )?;
            for name in &names {
                if name.starts_with("client.import-") && !RECORDS.contains(&name.as_str()) {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            self.init = fresh_record(&self.root().file, "client.init", self.user(), false)?;
            if let Some(init) = &self.init {
                if !fresh_init_bytes(
                    &fresh_read(init)?,
                    &fresh_binding(self.user(), self.root().id),
                ) {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            self.docker_dir = fresh_optional(&self.root().file, "docker", true, false)?
                .map(|f| Held::capture(f, self.user(), true, true).map_err(fresh_failure))
                .transpose()?;
            if let Some(dir) = &self.docker_dir {
                fresh_inventory(
                    &self.root().file,
                    "docker",
                    dir,
                    self.user(),
                    |n| n == "config.json",
                    1,
                )?;
                self.config = fresh_record(&dir.file, "config.json", self.user(), false)?;
                if let Some(config) = &self.config {
                    if !fresh_default_bytes(&fresh_read(config)?, &self.profile) {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                }
            }
            Ok(codec::Scratch {
                docker: self.docker_dir.as_ref().map(|d| codec::DirectoryWitness {
                    id: fid(d.id),
                    sd_sha256: hex(&d.security),
                }),
                init: self
                    .init
                    .as_ref()
                    .map(|d| witness(d, self.user()))
                    .transpose()?,
                config: self
                    .config
                    .as_ref()
                    .map(|d| witness(d, self.user()))
                    .transpose()?,
            })
        }
        fn create_record(
            &mut self,
            name: &'static str,
            directory: bool,
            bytes: &[u8],
        ) -> Result<Data, ClientConfigError> {
            self.check()?;
            if bytes.len() > CONFIG_BUDGET || self.pending.is_some() {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let sd = Descriptor::private(self.user(), false).map_err(fresh_failure)?;
            let parent = if directory {
                self.dir()
            } else {
                &self.root().file
            };
            // A known CREATE_NEW/open refusal precedes the write attempt and can release.
            let file = relative_status(parent, name, false, FILE_CREATE, Some(&sd), true)
                .map_err(mapped)?;
            self.pending = Some(CheckedFile(Some(file)));
            self.mutation_live = true;
            let result = (|| {
                let file = self.pending.as_ref().unwrap();
                identity(file, false).map_err(fresh_failure)?;
                Security::read_file(file)
                    .map_err(fresh_failure)?
                    .private(self.user(), false)
                    .map_err(fresh_failure)?;
                let mut writer: &File = file;
                writer.write_all(bytes).map_err(fresh_failure)?;
                writer.sync_all().map_err(fresh_failure)?;
                if body(file).map_err(fresh_failure)? != bytes {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                let before = identity(file, false).map_err(fresh_failure)?;
                let security = Security::read_file(file).map_err(fresh_failure)?.hash();
                close_file(self.pending.take().unwrap()).map_err(fresh_failure)?;
                let parent = if directory {
                    self.dir()
                } else {
                    &self.root().file
                };
                let file =
                    relative_status(parent, name, false, FILE_OPEN, None, false).map_err(mapped)?;
                self.pending = Some(CheckedFile(Some(file)));
                let after =
                    identity(self.pending.as_ref().unwrap(), false).map_err(fresh_failure)?;
                if !same_closed_file(before, after)
                    || Security::read_file(self.pending.as_ref().unwrap())
                        .map_err(fresh_failure)?
                        .hash()
                        != security
                    || body(self.pending.as_ref().unwrap()).map_err(fresh_failure)? != bytes
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                codec::verified_transfer(
                    self,
                    |owner| owner.check_retained(),
                    |owner| {
                        owner.mutation_live = false;
                        Data {
                            file: owner.pending.take().unwrap(),
                            id: after,
                            security,
                            writer: false,
                        }
                    },
                )
            })();
            if result.is_err() {
                retain_unknown()
            };
            result
        }
        fn load(&mut self) -> Result<(), ClientConfigError> {
            if self.load_completed()? {
                return Ok(());
            }
            if let Some(record) = self.records.get("client.import-intent") {
                self.intent_bytes = body(&record.file).map_err(fresh_failure)?;
                let intent = match codec::parse_intent(&self.intent_bytes) {
                    Ok(v) => v,
                    Err(_) => return self.load_uncommitted(),
                };
                if intent.root != fid(self.root().id)
                    || intent.owner_sid != self.user().text
                    || self.bootstrap.as_ref().unwrap().bytes != codec::MARKER
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                self.scratch = intent.scratch.clone();
                self.intent = Some(intent);
                self.load_bound()?;
            } else {
                if !self.records.is_empty() {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if self.action != codec::ImportAction::ReviewCurrentSource {
                    return Err(err(ErrorKind::RequiresLegacyReview));
                }
                self.scratch = self.capture_scratch()?;
                self.source = Some(Source::capture(self.user())?);
                let source = self.source.as_ref().unwrap();
                let old = codec::captured_old_selector(&source.bytes)?;
                codec::target(
                    &source.bytes,
                    &old,
                    self.plugins
                        .to_str()
                        .ok_or_else(|| err(ErrorKind::Unavailable))?,
                    &self.profile,
                )?;
                codec::encode(&self.fresh_intent("1".repeat(64))?)?;
            };
            Ok(())
        }
        fn fresh_intent(&self, binding: String) -> Result<codec::Intent, ClientConfigError> {
            let source = self
                .source
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            Ok(codec::Intent {
                schema: 1,
                kind: "legacy-client-import".into(),
                policy: codec::POLICY.into(),
                binding,
                owner_sid: self.user().text.clone(),
                root: fid(self.root().id),
                source_namespace: codec::SOURCE_NAMESPACE.into(),
                source: source.witness.clone(),
                old_selector: codec::captured_old_selector(&source.bytes)?,
                scratch: self.scratch.clone(),
            })
        }
        fn load_uncommitted(&mut self) -> Result<(), ClientConfigError> {
            if self.action != codec::ImportAction::ReviewCurrentSource
                || self.records.len() != 1
                || self.bootstrap.as_ref().unwrap().bytes != codec::MARKER
            {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            self.scratch = self.capture_scratch()?;
            self.source = Some(Source::capture(self.user())?);
            let candidate = self.fresh_intent("1".repeat(64))?;
            if !codec::uncommitted_prefix(&self.intent_bytes, &candidate) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            codec::target(
                &self.source.as_ref().unwrap().bytes,
                &candidate.old_selector,
                self.plugins
                    .to_str()
                    .ok_or_else(|| err(ErrorKind::Unavailable))?,
                &self.profile,
            )?;
            self.restart_uncommitted = true;
            Ok(())
        }
        fn load_completed(&mut self) -> Result<bool, ClientConfigError> {
            let Some(init) = fresh_record(&self.root().file, "client.init", self.user(), false)?
            else {
                return Ok(false);
            };
            let bytes = body(&init.file).map_err(fresh_failure)?;
            let completed = match codec::parse_completed(
                &bytes,
                &self.user().text,
                &fid(self.root().id),
                &self.profile,
            ) {
                Ok(v) => v,
                Err(_) => {
                    close_file(init.file).map_err(fresh_failure)?;
                    return Ok(false);
                }
            };
            if self.bootstrap.as_ref().unwrap().bytes != codec::MARKER {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            fresh_inventory(
                self.private(),
                "client",
                self.root(),
                self.user(),
                root_allowed,
                7,
            )?;
            let dir = relative_status(&self.root().file, "docker", true, FILE_OPEN, None, false)
                .map_err(mapped)?;
            self.docker_dir =
                Some(Held::capture(dir, self.user(), true, true).map_err(fresh_failure)?);
            fresh_inventory(
                &self.root().file,
                "docker",
                self.docker_dir.as_ref().unwrap(),
                self.user(),
                |n| n == "config.json",
                1,
            )?;
            self.config = Some(
                fresh_record(self.dir(), "config.json", self.user(), false)?
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
            );
            same_witness(
                self.config.as_ref().unwrap(),
                &completed.initial,
                self.user(),
            )?;
            self.init = Some(init);
            if let Some(data) = self.records.get("client.import-intent") {
                self.intent_bytes = body(&data.file).map_err(fresh_failure)?;
                let intent = codec::parse_intent(&self.intent_bytes)?;
                if intent.root != fid(self.root().id)
                    || intent.owner_sid != self.user().text
                    || intent.binding != completed.binding
                    || intent.source != completed.source
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                };
                self.intent = Some(intent);
            } else if !self.records.is_empty() {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if let Some(data) = self.records.get("client.import-original") {
                self.original = body(&data.file).map_err(fresh_failure)?;
                if self.original.len() as u64 != completed.source.bytes
                    || codec::hash(&self.original) != completed.source.body_sha256
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if let Some(data) = self.records.get("client.import-source") {
                let copy = codec::parse_source_copy(
                    &body(&data.file).map_err(fresh_failure)?,
                    self.intent
                        .as_ref()
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
                    &self.intent_bytes,
                )?;
                if let Some(original) = self.records.get("client.import-original") {
                    same_witness(original, &copy.original, self.user())?
                }
            } else if self.records.contains_key("client.import-original") {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if let Some(data) = self.records.get("client.import-target") {
                let copy = self
                    .records
                    .get("client.import-source")
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                let target = codec::parse_target(
                    &body(&data.file).map_err(fresh_failure)?,
                    self.intent.as_ref().unwrap(),
                    &body(&copy.file).map_err(fresh_failure)?,
                    &self.original,
                    &self.profile,
                )?;
                if target.selector != completed.initial_selector
                    || target.target_sha256 != completed.initial.body_sha256
                    || target.target_bytes != completed.initial.bytes
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            };
            self.completed_summary = true;
            Ok(true)
        }
        fn load_bound(&mut self) -> Result<(), ClientConfigError> {
            fresh_inventory(
                self.private(),
                "client",
                self.root(),
                self.user(),
                root_allowed,
                7,
            )?;
            let intent = self.intent.as_ref().unwrap();
            if let Some(data) = self.records.get("client.import-original") {
                self.original = body(&data.file).map_err(fresh_failure)?;
                if self.original.len() as u64 != intent.source.bytes
                    || codec::hash(&self.original) != intent.source.body_sha256
                {
                    if self.records.contains_key("client.import-source")
                        || self.records.contains_key("client.import-target")
                    {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                    self.source = Some(Source::capture(self.user())?);
                    let source = self.source.as_ref().unwrap();
                    if source.witness != intent.source || !source.bytes.starts_with(&self.original)
                    {
                        return Err(err(ErrorKind::RequiresLegacyReview));
                    };
                    self.original = source.bytes.clone();
                }
            }
            if let Some(copy) = self.records.get("client.import-source") {
                let bytes = body(&copy.file).map_err(fresh_failure)?;
                let original = self
                    .records
                    .get("client.import-original")
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                let expected = codec::SourceCopy {
                    schema: 1,
                    kind: "legacy-source-copy".into(),
                    binding: intent.binding.clone(),
                    root: intent.root.clone(),
                    intent_sha256: codec::hash(&self.intent_bytes),
                    original: witness(original, self.user())?,
                };
                if !codec::encode(&expected)?.starts_with(&bytes) {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if codec::parse_source_copy(&bytes, intent, &self.intent_bytes).is_err() {
                    if self.records.contains_key("client.import-target") {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                }
            } else if self.original.is_empty() {
                // Until the exact protected source-copy receipt is full, the SAME source
                // binding is required. Missing/changed public bytes never select defaults.
                self.source = Some(Source::capture(self.user())?);
                let source = self.source.as_ref().unwrap();
                if source.witness != intent.source {
                    return Err(err(ErrorKind::RequiresLegacyReview));
                }
            }
            self.init = fresh_record(&self.root().file, "client.init", self.user(), false)?;
            self.docker_dir = fresh_optional(&self.root().file, "docker", true, false)?
                .map(|f| Held::capture(f, self.user(), true, true).map_err(fresh_failure))
                .transpose()?;
            if let Some(dir) = &self.docker_dir {
                fresh_inventory(
                    &self.root().file,
                    "docker",
                    dir,
                    self.user(),
                    |n| matches!(n, "config.json" | "config.stage"),
                    2,
                )?;
                self.config = fresh_record(&dir.file, "config.json", self.user(), false)?;
                self.stage = fresh_record(&dir.file, "config.stage", self.user(), false)?;
            }
            let raw = if self.original.is_empty() {
                &self
                    .source
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                    .bytes
            } else {
                &self.original
            };
            codec::target(
                raw,
                &intent.old_selector,
                self.plugins
                    .to_str()
                    .ok_or_else(|| err(ErrorKind::Unavailable))?,
                &self.profile,
            )?;
            self.validate_state()
        }
        fn validate_state(&self) -> Result<(), ClientConfigError> {
            let intent = self
                .intent
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            if let Some(dir) = &self.docker_dir {
                if let Some(before) = &intent.scratch.docker {
                    if fid(dir.id) != before.id || hex(&dir.security) != before.sd_sha256 {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                }
            }
            let target = self
                .records
                .get("client.import-target")
                .map(|data| {
                    let copy = self
                        .records
                        .get("client.import-source")
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                    let bytes = body(&data.file).map_err(fresh_failure)?;
                    let copy_bytes = body(&copy.file).map_err(fresh_failure)?;
                    if let Ok(target) = codec::parse_target(
                        &bytes,
                        intent,
                        &copy_bytes,
                        &self.original,
                        &self.profile,
                    ) {
                        return Ok(Some(target));
                    }
                    let candidate = self.new_target(&copy_bytes)?;
                    if self.config.is_some()
                        || self.init.is_some()
                        || self.stage.is_some()
                        || !codec::target_prefix(
                            &bytes,
                            &candidate,
                            &self.original,
                            &intent.old_selector,
                            &self.profile,
                        )
                    {
                        return Err(err(ErrorKind::RecoveryRequired));
                    };
                    Ok(None)
                })
                .transpose()?
                .flatten();
            if let Some(config) = &self.config {
                let bytes = body(&config.file).map_err(fresh_failure)?;
                let old =
                    intent.scratch.config.as_ref().is_some_and(|w| {
                        witness(config, self.user()).is_ok_and(|actual| actual == *w)
                    });
                let published = if let Some(record) = target.as_ref() {
                    bytes
                        == codec::target(
                            &self.original,
                            &intent.old_selector,
                            &record.selector,
                            &self.profile,
                        )?
                } else {
                    false
                };
                if (!old && !published) || (old && target.is_some()) || self.stage.is_some() {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if let Some(stage) = &self.stage {
                let record = target
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                let expected = codec::target(
                    &self.original,
                    &intent.old_selector,
                    &record.selector,
                    &self.profile,
                )?;
                if !expected.starts_with(&body(&stage.file).map_err(fresh_failure)?) {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if let Some(init) = &self.init {
                let bytes = body(&init.file).map_err(fresh_failure)?;
                let old =
                    intent.scratch.init.as_ref().is_some_and(|w| {
                        witness(init, self.user()).is_ok_and(|actual| actual == *w)
                    });
                if old && target.is_some() {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if !old {
                    let record = target
                        .as_ref()
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                    let expected = codec::encode(&self.completion(&record.selector)?)?;
                    if !expected.starts_with(&bytes) {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                }
            };
            Ok(())
        }
        pub(in crate::client_config) fn summary(&self) -> &'static str {
            if self.completed_summary {
                "Finish cleanup of the committed import, then refresh current OrbWin routes. The imported settings are already committed. OrbWin leaves the public source in place; no credential command is replayed."
            } else if self.restart_uncommitted {
                "Discard only the uncommitted import preparation and import the source newly captured for this review. Earlier-source continuity is unknown. Replace managed credential and plugin routes with current OrbWin routes and reset the private context to default. OrbWin leaves the public source in place."
            } else {
                "Import the reviewed historical OrbWin application settings into the private client. Replace managed credential and plugin routes with current OrbWin routes and reset the private context to default. OrbWin leaves the public source in place; historical context directories are not copied."
            }
        }
        // Caller supplies only a typed reviewed action. It cannot supply paths,
        // identities, records, runtime roles or source bytes.
        fn apply_inner(&mut self) -> Result<(), ClientConfigError> {
            self.check()?;
            if self.init.as_ref().is_some_and(|d| {
                body(&d.file).is_ok_and(|b| {
                    codec::parse_completed(
                        &b,
                        &self.user().text,
                        &fid(self.root().id),
                        &self.profile,
                    )
                    .is_ok()
                })
            }) {
                return self.cleanup_completed();
            }
            if self.intent.is_none() {
                let source = self
                    .source
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                source.check(self.user())?;
                if self.capture_scratch()? != self.scratch {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                let mut binding = [0u8; 32];
                if unsafe { BCryptGenRandom(null_mut(), binding.as_mut_ptr(), 32, 2) } < 0
                    || binding == [0; 32]
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                let intent = self.fresh_intent(hex(&binding))?;
                let bytes = codec::encode(&intent)?;
                let user = self.user.as_ref().unwrap();
                let private = &self.ancestors[self.ancestors.len() - 2].file;
                self.bootstrap
                    .as_mut()
                    .unwrap()
                    .select_import(private, user)?;
                if self.restart_uncommitted {
                    if !codec::uncommitted_prefix(
                        &body(&self.records.get("client.import-intent").unwrap().file)
                            .map_err(fresh_failure)?,
                        &intent,
                    ) {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                    self.remove_record("client.import-intent")?;
                }
                let data = self.create_record("client.import-intent", false, &bytes)?;
                self.records.insert("client.import-intent", data);
                self.intent_bytes = bytes;
                self.intent = Some(intent);
            }
            if !self.records.contains_key("client.import-original") {
                let bytes = self
                    .source
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                    .bytes
                    .clone();
                let data = self.create_record("client.import-original", false, &bytes)?;
                self.records.insert("client.import-original", data);
                self.original = bytes;
            }
            if self
                .records
                .get("client.import-original")
                .is_some_and(|d| body(&d.file).is_ok_and(|b| b != self.original))
            {
                let data = self.records.remove("client.import-original").unwrap();
                self.dispose("client.import-original", false, data)?;
                let bytes = self.original.clone();
                let data = self.create_record("client.import-original", false, &bytes)?;
                self.records.insert("client.import-original", data);
            }
            if let Some(data) = self.records.get("client.import-source") {
                let bytes = body(&data.file).map_err(fresh_failure)?;
                if codec::parse_source_copy(
                    &bytes,
                    self.intent.as_ref().unwrap(),
                    &self.intent_bytes,
                )
                .is_err()
                {
                    let data = self.records.remove("client.import-source").unwrap();
                    self.dispose("client.import-source", false, data)?;
                }
            }
            if !self.records.contains_key("client.import-source") {
                let intent = self.intent.as_ref().unwrap();
                let copy = codec::SourceCopy {
                    schema: 1,
                    kind: "legacy-source-copy".into(),
                    binding: intent.binding.clone(),
                    root: intent.root.clone(),
                    intent_sha256: codec::hash(&self.intent_bytes),
                    original: witness(
                        self.records.get("client.import-original").unwrap(),
                        self.user(),
                    )?,
                };
                let bytes = codec::encode(&copy)?;
                let data = self.create_record("client.import-source", false, &bytes)?;
                self.records.insert("client.import-source", data);
            }
            self.validate_state()?;
            self.publish_and_complete()
        }
        pub(in crate::client_config) fn apply(
            mut self: Box<Self>,
        ) -> Result<ClientConfigGuard, ClientConfigError> {
            self.apply_inner()?;
            if !self.records.is_empty()
                || self.bootstrap.is_some()
                || self.pending.is_some()
                || self.mutation_live
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if let Some(source) = self.source.take() {
                source.close()?
            }
            // Move the SAME exclusive controller and retained graph into the ordinary
            // owner. A historical C publication is refreshed to D as a separate normal
            // transaction, after import cleanup; no controller reacquisition/gap exists.
            let config = self
                .config
                .take()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let bytes = body(&config.file).map_err(fresh_failure)?;
            self.ancestors.push(
                self.docker_dir
                    .take()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
            );
            let profile = self.profile.clone();
            let path = profile
                .join(".orbwin-private")
                .join("client")
                .join("docker");
            let mut owned = Box::new(Owned {
                user: self.user.take().unwrap(),
                ancestors: std::mem::take(&mut self.ancestors),
                owner: self.controller.take(),
                init: self.init.take(),
                config: Some(config),
                intent: None,
                body: bytes,
                plugins: self.plugins.clone(),
                docker: self.docker.take().unwrap(),
                credentials: self.credentials.take().unwrap(),
                loaned: false,
                poison: false,
                binding: None,
                writer_base: None,
                writer_original: None,
                writer_prepare: None,
                writer_birth: Default::default(),
                recovery_record: None,
                recovery_next: None,
                completed_birth: false,
                publication_unknown: false,
                recovery_stage: None,
                recovery_stage_hash: None,
                recovery_preserves_source: false,
                local_replace_hash: None,
                local_eof: false,
                partial_prepare: false,
                partial_prepare_hash: None,
                partial_original: false,
                partial_original_hash: None,
                writer_keys: Vec::new(),
                writer_stopped: false,
            });
            let result = (|| {
                if let Some(next) = refresh_managed_field(&owned.body, &owned.plugins, &profile)? {
                    owned.replace_config(&next)?
                };
                parse_config(&owned.body, &owned.plugins)?;
                owned.check()
            })();
            if let Err(e) = result {
                release_pending(owned)?;
                return Err(e);
            };
            Ok(ClientConfigGuard {
                path,
                inner: Some(owned),
            })
        }
        fn dispose(
            &mut self,
            name: &str,
            in_docker: bool,
            data: Data,
        ) -> Result<(), ClientConfigError> {
            self.check()?;
            data.check(self.user()).map_err(fresh_failure)?;
            let expected = (data.id, data.security);
            let bytes = body(&data.file).map_err(fresh_failure)?;
            close_file(data.file).map_err(fresh_failure)?;
            let parent = if in_docker {
                self.dir()
            } else {
                &self.root().file
            };
            let file = relative_status_access(
                parent, name, false, FILE_OPEN, None, true, true, false, false,
            )
            .map_err(mapped)?;
            let data = Data::capture(file, self.user()).map_err(fresh_failure)?;
            if (data.id, data.security) != expected
                || body(&data.file).map_err(fresh_failure)? != bytes
            {
                close_file(data.file).map_err(fresh_failure)?;
                return Err(err(ErrorKind::RecoveryRequired));
            }
            self.pending = Some(data.file);
            self.mutation_live = true;
            let result = (|| {
                let disposition = FILE_DISPOSITION_INFO { DeleteFile: true };
                if unsafe {
                    SetFileInformationByHandle(
                        raw(self.pending.as_ref().unwrap()),
                        FileDispositionInfo,
                        (&disposition as *const FILE_DISPOSITION_INFO).cast(),
                        size_of::<FILE_DISPOSITION_INFO>() as u32,
                    )
                } == 0
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                close_file(self.pending.take().unwrap()).map_err(fresh_failure)?;
                let parent = if in_docker {
                    self.dir()
                } else {
                    &self.root().file
                };
                disposed_absent(parent, name, false).map_err(fresh_failure)?;
                self.check_retained()?;
                self.mutation_live = false;
                Ok(())
            })();
            if result.is_err() {
                retain_unknown()
            };
            result
        }
        fn remove_record(&mut self, name: &'static str) -> Result<(), ClientConfigError> {
            if let Some(data) = self.records.remove(name) {
                self.dispose(name, false, data)?
            };
            Ok(())
        }
        fn completion(&self, selector: &str) -> Result<codec::CompletedInit, ClientConfigError> {
            let intent = self.intent.as_ref().unwrap();
            let root = fid(self.root().id);
            Ok(codec::CompletedInit {
                schema: 2,
                product: "orbwin".into(),
                admission: "legacy-import".into(),
                policy: codec::POLICY.into(),
                owner_sid: self.user().text.clone(),
                root_volume: root.volume,
                root_high: root.high,
                root_low: root.low,
                binding: intent.binding.clone(),
                source_namespace: codec::SOURCE_NAMESPACE.into(),
                source: intent.source.clone(),
                initial: witness(
                    self.config
                        .as_ref()
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
                    self.user(),
                )?,
                initial_selector: selector.into(),
            })
        }
        fn new_target(&self, copy_bytes: &[u8]) -> Result<codec::Target, ClientConfigError> {
            let intent = self.intent.as_ref().unwrap();
            let selector = self
                .plugins
                .to_str()
                .ok_or_else(|| err(ErrorKind::Unavailable))?
                .to_owned();
            let bytes = codec::target(
                &self.original,
                &intent.old_selector,
                &selector,
                &self.profile,
            )?;
            Ok(codec::Target {
                schema: 1,
                kind: "legacy-import-target".into(),
                binding: intent.binding.clone(),
                root: intent.root.clone(),
                source_copy_sha256: codec::hash(copy_bytes),
                selector,
                target_sha256: codec::hash(&bytes),
                target_bytes: bytes.len() as u64,
            })
        }
        fn publish_and_complete(&mut self) -> Result<(), ClientConfigError> {
            self.check()?;
            let intent = self.intent.as_ref().unwrap().clone();
            let current_selector = self
                .plugins
                .to_str()
                .ok_or_else(|| err(ErrorKind::Unavailable))?
                .to_owned();
            let copy_bytes = body(&self.records.get("client.import-source").unwrap().file)
                .map_err(fresh_failure)?;
            let old_target = self
                .records
                .get("client.import-target")
                .map(|d| {
                    let bytes = body(&d.file).map_err(fresh_failure)?;
                    if let Ok(v) = codec::parse_target(
                        &bytes,
                        &intent,
                        &copy_bytes,
                        &self.original,
                        &self.profile,
                    ) {
                        return Ok(Some(v));
                    }
                    if self.config.is_some()
                        || self.init.is_some()
                        || self.stage.is_some()
                        || !codec::target_prefix(
                            &bytes,
                            &self.new_target(&copy_bytes)?,
                            &self.original,
                            &intent.old_selector,
                            &self.profile,
                        )
                    {
                        return Err(err(ErrorKind::RecoveryRequired));
                    };
                    Ok(None)
                })
                .transpose()?
                .flatten();
            // A verified published historical C finishes its own init/cleanup first.
            // It never executes C: ordinary refresh D is a separate transaction later.
            let published =
                if let (Some(record), Some(config)) = (old_target.as_ref(), self.config.as_ref()) {
                    let actual = body(&config.file).map_err(fresh_failure)?;
                    let expected = codec::target(
                        &self.original,
                        &intent.old_selector,
                        &record.selector,
                        &self.profile,
                    )?;
                    (actual == expected).then_some(record.clone())
                } else {
                    None
                };
            if let Some(data) = self.init.take() {
                let bytes = body(&data.file).map_err(fresh_failure)?;
                if let Some(target) = &published {
                    let completed = codec::encode(&self.completion(&target.selector)?)?;
                    if bytes == completed {
                        self.init = Some(data)
                    } else if completed.starts_with(&bytes) {
                        self.dispose("client.init", false, data)?
                    } else if intent
                        .scratch
                        .init
                        .as_ref()
                        .is_some_and(|w| witness(&data, self.user()).is_ok_and(|a| a == *w))
                    {
                        self.dispose("client.init", false, data)?
                    } else {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                } else {
                    same_witness(
                        &data,
                        intent
                            .scratch
                            .init
                            .as_ref()
                            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
                        self.user(),
                    )?;
                    self.dispose("client.init", false, data)?
                }
            }
            if published.is_none() {
                // Both scratch retirements are AFTER full intent + immutable original +
                // exact protected source receipt. The intent records each original cap.
                if let Some(config) = self.config.take() {
                    same_witness(
                        &config,
                        intent
                            .scratch
                            .config
                            .as_ref()
                            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
                        self.user(),
                    )?;
                    self.dispose("config.json", true, config)?
                }
                if let Some(stage) = self.stage.take() {
                    self.dispose("config.stage", true, stage)?
                }
                self.remove_record("client.import-target")?;
                if self.docker_dir.is_none() {
                    let sd = Descriptor::private(self.user(), true).map_err(fresh_failure)?;
                    let dir = relative_status(
                        &self.root().file,
                        "docker",
                        true,
                        FILE_CREATE,
                        Some(&sd),
                        false,
                    )
                    .map_err(mapped)?;
                    self.docker_dir =
                        Some(Held::capture(dir, self.user(), true, true).map_err(fresh_failure)?);
                }
                let target_bytes = codec::target(
                    &self.original,
                    &intent.old_selector,
                    &current_selector,
                    &self.profile,
                )?;
                let target = codec::Target {
                    schema: 1,
                    kind: "legacy-import-target".into(),
                    binding: intent.binding.clone(),
                    root: intent.root.clone(),
                    source_copy_sha256: codec::hash(&copy_bytes),
                    selector: current_selector.clone(),
                    target_sha256: codec::hash(&target_bytes),
                    target_bytes: target_bytes.len() as u64,
                };
                let encoded = codec::encode(&target)?;
                let data = self.create_record("client.import-target", false, &encoded)?;
                self.records.insert("client.import-target", data);
                self.stage = Some(self.create_record("config.stage", true, &target_bytes)?);
                self.mutation_live = true;
                let result = (|| {
                    let stage = self.stage.as_ref().unwrap();
                    let expected = (stage.id, stage.security);
                    rename(self.dir(), &stage.file, "config.json").map_err(fresh_failure)?;
                    close_file(self.stage.take().unwrap().file).map_err(fresh_failure)?;
                    let file =
                        relative_status(self.dir(), "config.json", false, FILE_OPEN, None, false)
                            .map_err(mapped)?;
                    self.pending = Some(CheckedFile(Some(file)));
                    let after =
                        identity(self.pending.as_ref().unwrap(), false).map_err(fresh_failure)?;
                    let sd = Security::read_file(self.pending.as_ref().unwrap())
                        .map_err(fresh_failure)?;
                    sd.private(self.user(), false).map_err(fresh_failure)?;
                    if (after, sd.hash()) != expected
                        || body(self.pending.as_ref().unwrap()).map_err(fresh_failure)?
                            != target_bytes
                    {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                    self.config = Some(Data {
                        file: self.pending.take().unwrap(),
                        id: after,
                        security: sd.hash(),
                        writer: false,
                    });
                    disposed_absent(self.dir(), "config.stage", false).map_err(fresh_failure)?;
                    self.check_retained()?;
                    self.mutation_live = false;
                    Ok(())
                })();
                if result.is_err() {
                    retain_unknown()
                };
                result?;
            }
            let selector = published
                .as_ref()
                .map_or(current_selector.as_str(), |v| v.selector.as_str());
            if self.init.is_none() {
                let bytes = codec::encode(&self.completion(selector)?)?;
                self.init = Some(self.create_record("client.init", false, &bytes)?)
            }
            codec::parse_completed(
                &body(&self.init.as_ref().unwrap().file).map_err(fresh_failure)?,
                &self.user().text,
                &fid(self.root().id),
                &self.profile,
            )?;
            self.cleanup_completed()
        }
        fn cleanup_completed(&mut self) -> Result<(), ClientConfigError> {
            // Raw original is retired before its full receipt, intent last. Each cold
            // cleanup cut is admitted ONLY by actual root-bound completed init + exact
            // initial target native witness, never by a filename or remembered bool.
            let completed = codec::parse_completed(
                &body(
                    &self
                        .init
                        .as_ref()
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                        .file,
                )
                .map_err(fresh_failure)?,
                &self.user().text,
                &fid(self.root().id),
                &self.profile,
            )?;
            same_witness(
                self.config
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
                &completed.initial,
                self.user(),
            )?;
            if let Some(stage) = self.stage.take() {
                self.dispose("config.stage", true, stage)?
            }
            for name in [
                "client.import-target",
                "client.import-original",
                "client.import-source",
                "client.import-intent",
            ] {
                self.remove_record(name)?
            }
            // Retire ONLY the captured full marker while retaining the same controller.
            self.check()?;
            let bootstrap = self.bootstrap.as_mut().unwrap();
            let expected = (bootstrap.id, bootstrap.security);
            bootstrap.close()?;
            // Positive close followed by known open Busy is still a pre-delete refusal.
            // Only the actual disposition call and its verification suffix arm unknown.
            let file = relative_status_access(
                self.private(),
                "client.create-intent",
                false,
                FILE_OPEN,
                None,
                true,
                true,
                false,
                false,
            )
            .map_err(mapped)?;
            self.pending = Some(CheckedFile(Some(file)));
            let file = self.pending.as_ref().unwrap();
            if (
                identity(file, false).map_err(fresh_failure)?,
                Security::read_file(file).map_err(fresh_failure)?.hash(),
            ) != expected
                || body(file).map_err(fresh_failure)? != codec::MARKER
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            self.mutation_live = true;
            let result = (|| {
                let disposition = FILE_DISPOSITION_INFO { DeleteFile: true };
                if unsafe {
                    SetFileInformationByHandle(
                        raw(self.pending.as_ref().unwrap()),
                        FileDispositionInfo,
                        (&disposition as *const FILE_DISPOSITION_INFO).cast(),
                        size_of::<FILE_DISPOSITION_INFO>() as u32,
                    )
                } == 0
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                close_file(self.pending.take().unwrap()).map_err(fresh_failure)?;
                disposed_absent(self.private(), "client.create-intent", false)
                    .map_err(fresh_failure)?;
                self.mutation_live = false;
                Ok(())
            })();
            if result.is_err() {
                retain_unknown()
            };
            result?;
            self.bootstrap.take();
            Ok(())
        }
    }
    impl Drop for Owner {
        fn drop(&mut self) {
            let _ = self.release();
        }
    }
}
