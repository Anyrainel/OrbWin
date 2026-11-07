// Private child of runtime_cache::native. Only Candidate4 uses this owner.
// Every original output and backing store is published inside the memo-owned
// operation before FFI. No native resource is converted into std::fs::File.
use super::super::custody_model as policy;
use super::*;
use std::{
    mem::ManuallyDrop,
    sync::{Arc, Mutex},
};

#[link(name = "kernel32")]
unsafe extern "system" {
    #[link_name = "GetModuleFileNameW"]
    fn candidate_module_name(module: HANDLE, output: *mut u16, capacity: u32) -> u32;
}
#[link(name = "ntdll")]
unsafe extern "system" {
    fn NtReadFile(
        file: HANDLE,
        event: HANDLE,
        apc: *const c_void,
        context: *const c_void,
        status: *mut IoStatus,
        buffer: *mut c_void,
        length: u32,
        offset: *const i64,
        key: *const u32,
    ) -> i32;
    fn NtWriteFile(
        file: HANDLE,
        event: HANDLE,
        apc: *const c_void,
        context: *const c_void,
        status: *mut IoStatus,
        buffer: *const c_void,
        length: u32,
        offset: *const i64,
        key: *const u32,
    ) -> i32;
}
#[derive(Clone)]
pub(super) struct CandidateLease {
    operation: Arc<Mutex<Operation>>,
}
impl CandidateLease {
    pub(super) fn new() -> Self {
        Self::with_mode(policy::Mode::Candidate4)
    }
    pub(super) fn ordinary() -> Self {
        Self::with_mode(policy::Mode::OrdinaryRdd2)
    }
    fn with_mode(mode: policy::Mode) -> Self {
        Self {
            operation: Arc::new(Mutex::new(Operation::new(mode))),
        }
    }
    pub(super) fn prepare_or_recheck(&self) -> Result<(), String> {
        if !cfg!(target_arch = "x86_64") {
            return Err(bad("owned Rdd native AMD64 scope"));
        }
        let mut operation = self
            .operation
            .lock()
            .map_err(|_| bad("owned Rdd original graph poisoned"))?;
        let result = match operation.phase {
            Phase::New => {
                operation.phase = Phase::Preparing;
                operation.failure = bad("owned Rdd preparation did not settle");
                operation.prepare()
            }
            Phase::Ready => {
                let full = operation.mode == policy::Mode::Candidate4;
                operation.recheck_with_hash(full)
            }
            _ => return Err(operation.failure.clone()),
        };
        operation.settle(result)
    }
    pub(super) fn recheck(&self, full: bool) -> Result<(), String> {
        let mut operation = self
            .operation
            .lock()
            .map_err(|_| bad("owned Rdd graph poisoned"))?;
        if !matches!(operation.phase, Phase::Ready) {
            return Err(operation.failure.clone());
        }
        let result = operation.recheck_with_hash(full);
        operation.settle(result)
    }
    pub(super) fn ordinary_data(&self) -> Result<(PathBuf, serde_json::Value), String> {
        let operation = self
            .operation
            .lock()
            .map_err(|_| bad("ordinary Rdd graph poisoned"))?;
        if !matches!(operation.phase, Phase::Ready) || operation.mode != policy::Mode::OrdinaryRdd2
        {
            return Err(bad("ordinary result unavailable"));
        }
        Ok((
            operation
                .owned
                .cache_path
                .clone()
                .ok_or_else(|| bad("ordinary cache path missing"))?,
            operation
                .owned
                .manifest_value
                .clone()
                .ok_or_else(|| bad("ordinary manifest missing"))?,
        ))
    }
    pub(super) fn verify_original_process(
        &self,
        process: std::os::windows::io::BorrowedHandle<'_>,
        expected: Option<(u32, u64, u32)>,
    ) -> Result<(), String> {
        let mut operation = self
            .operation
            .lock()
            .map_err(|_| bad("ordinary verifier graph poisoned"))?;
        if !matches!(operation.phase, Phase::Ready) || operation.mode != policy::Mode::OrdinaryRdd2
        {
            return Err(bad("ordinary process verifier unavailable"));
        }
        let result = operation.verify_original(process, expected);
        operation.settle(result)
    }
}
#[derive(Clone, Copy)]
enum Phase {
    New,
    Preparing,
    Ready,
    Refused,
    Unknown,
}
struct HandleSlot {
    value: HANDLE,
    close: policy::CloseMemo,
}
#[derive(Clone, Copy)]
enum AllocationKind {
    Task,
    Local,
}
struct AllocationSlot {
    value: *mut c_void,
    kind: AllocationKind,
    close: policy::CloseMemo,
    text: Vec<u16>,
    returned: bool,
    raw: i32,
    free_return: *mut c_void,
}
struct OpenFrame {
    text: Vec<u16>,
    unicode: Unicode,
    attributes: Attributes,
    io: IoStatus,
    raw: i32,
    returned: bool,
    slot: usize,
    parent: Option<usize>,
}
struct RenameFrame {
    buffer: Vec<usize>,
    io: IoStatus,
    raw: i32,
    returned: bool,
    parent: usize,
    stage: usize,
    bytes: usize,
}
struct QueryFrame {
    entered: bool,
    result: u32,
    error: u32,
    needed: u32,
    elevation: u32,
    session: u32,
    token: Vec<u64>,
    info: BY_HANDLE_FILE_INFORMATION,
    full_id: FILE_ID_INFO,
    path: Vec<u16>,
    normalized: String,
    flags: u32,
    sd: Vec<u32>,
    sd_length: usize,
    owner: *mut c_void,
    acl: *mut ACL,
    defaulted: i32,
    present: i32,
    control: u16,
    revision: u32,
    ace: *mut c_void,
}
struct IoFrame {
    entered: bool,
    returned: bool,
    raw: i32,
    io: IoStatus,
    buffer: Vec<u8>,
    offset: i64,
}
#[derive(Clone)]
struct UserData {
    sid: Vec<u8>,
    text: String,
    session: u32,
    token: usize,
}
#[derive(Clone)]
struct Object {
    slot: usize,
    id: policy::Identity,
    sd: Option<[u8; 32]>,
    directory: bool,
    private: bool,
}
struct Asset {
    source: Object,
    final_object: Option<Object>,
    expected: [u8; 32],
    limit: u64,
    name: &'static str,
}
struct ProcessFrame {
    entered: bool,
    returned: bool,
    result: u32,
    error: u32,
    original_input: usize,
    pid: u32,
    session: u32,
    times: [FILETIME; 4],
    late_times: [FILETIME; 4],
    path: Vec<u16>,
    length: u32,
    duplicate: Option<usize>,
    image: Option<usize>,
    image_open: Box<OpenFrame>,
}
struct Owned {
    handles: Vec<Box<HandleSlot>>,
    allocations: Vec<Box<AllocationSlot>>,
    opens: Vec<Box<OpenFrame>>,
    renames: Vec<Box<RenameFrame>>,
    query: Box<QueryFrame>,
    io: Box<IoFrame>,
    user: Option<UserData>,
    anchors: Vec<Object>,
    assets: Vec<Asset>,
    manifest: Vec<u8>,
    manifest_object: Option<Object>,
    probe_token: Option<usize>,
    scratch_token: Option<usize>,
    source_directory: Option<usize>,
    cache_path: Option<PathBuf>,
    manifest_value: Option<serde_json::Value>,
    process: Box<ProcessFrame>,
}
struct Operation {
    mode: policy::Mode,
    phase: Phase,
    unknown: bool,
    failure: String,
    owned: ManuallyDrop<Owned>,
}
// Original buffers/cells are boxed and only accessed under this mutex. A pending
// call freezes the operation before any later access, including diagnostic reads.
unsafe impl Send for Operation {}
impl Operation {
    fn new(mode: policy::Mode) -> Self {
        Self {
            mode,
            phase: Phase::New,
            unknown: false,
            failure: bad("candidate unprepared"),
            owned: ManuallyDrop::new(Owned {
                handles: Vec::new(),
                allocations: Vec::new(),
                opens: Vec::new(),
                renames: Vec::new(),
                query: Box::new(QueryFrame {
                    entered: false,
                    result: 0,
                    error: 0,
                    needed: 0,
                    elevation: 0,
                    session: 0,
                    token: vec![0; 512],
                    info: unsafe { zeroed() },
                    full_id: unsafe { zeroed() },
                    path: vec![0; 32768],
                    normalized: String::new(),
                    flags: 0,
                    sd: vec![0; policy::SD_LIMIT / 4],
                    sd_length: 0,
                    owner: null_mut(),
                    acl: null_mut(),
                    defaulted: 0,
                    present: 0,
                    control: 0,
                    revision: 0,
                    ace: null_mut(),
                }),
                io: Box::new(IoFrame {
                    entered: false,
                    returned: false,
                    raw: 0,
                    io: IoStatus {
                        status: policy::SENTINEL,
                        information: policy::SENTINEL,
                    },
                    buffer: vec![0; policy::IO_LIMIT],
                    offset: 0,
                }),
                user: None,
                anchors: Vec::new(),
                assets: Vec::new(),
                manifest: Vec::new(),
                manifest_object: None,
                probe_token: None,
                scratch_token: None,
                source_directory: None,
                cache_path: None,
                manifest_value: None,
                process: Box::new(ProcessFrame {
                    entered: false,
                    returned: false,
                    result: 0,
                    error: 0,
                    original_input: 0,
                    pid: 0,
                    session: 0,
                    times: [unsafe { zeroed() }; 4],
                    late_times: [unsafe { zeroed() }; 4],
                    path: vec![0; 32768],
                    length: 0,
                    duplicate: None,
                    image: None,
                    image_open: Box::new(OpenFrame {
                        text: vec![0; 32768],
                        unicode: Unicode {
                            length: 0,
                            maximum: 0,
                            buffer: null_mut(),
                        },
                        attributes: unsafe { zeroed() },
                        io: IoStatus {
                            status: policy::SENTINEL,
                            information: policy::SENTINEL,
                        },
                        raw: 0,
                        returned: false,
                        slot: 0,
                        parent: None,
                    }),
                }),
            }),
        }
    }
    fn settle(&mut self, result: Result<(), String>) -> Result<(), String> {
        match result {
            Ok(()) => {
                self.phase = Phase::Ready;
                Ok(())
            }
            Err(error) => {
                self.failure = error.clone();
                self.phase = if self.unknown {
                    Phase::Unknown
                } else {
                    Phase::Refused
                };
                Err(error)
            }
        }
    }
    fn known(&self) -> Result<(), String> {
        if self.unknown
            || self.owned.query.entered
            || self.owned.io.entered
            || self.owned.process.entered
        {
            Err(bad("candidate unsettled native call"))
        } else {
            Ok(())
        }
    }
    fn freeze<T>(&mut self, stage: &str) -> Result<T, String> {
        self.unknown = true;
        Err(bad(stage))
    }
    fn handle(&self, slot: usize) -> Result<HANDLE, String> {
        self.known()?;
        let h = self
            .owned
            .handles
            .get(slot)
            .ok_or_else(|| bad("candidate slot"))?;
        if h.close.attempted || h.value.is_null() || h.value == INVALID_HANDLE_VALUE {
            Err(bad("candidate closed/empty slot"))
        } else {
            Ok(h.value)
        }
    }
    fn reserve_handle(&mut self) -> Result<usize, String> {
        self.known()?;
        if !policy::room(self.owned.handles.len(), policy::HANDLE_LIMIT) {
            return Err(bad("candidate handle bound"));
        }
        let index = self.owned.handles.len();
        self.owned.handles.push(Box::new(HandleSlot {
            value: null_mut(),
            close: Default::default(),
        }));
        Ok(index)
    }
    fn begin_query(&mut self) -> Result<(), String> {
        self.known()?;
        self.owned.query.entered = true;
        Ok(())
    }
    fn end_query(&mut self) {
        self.owned.query.entered = false;
    }
    fn close_handle(&mut self, slot: usize) -> Result<(), String> {
        self.known()?;
        let h = self.handle(slot)?;
        if !self.owned.handles[slot].close.arm() {
            return self.freeze("candidate close repeated");
        }
        self.begin_query()?;
        self.owned.query.result = unsafe { CloseHandle(h) } as u32;
        if self.owned.query.result == 0 {
            self.owned.query.error = unsafe { GetLastError() };
        }
        self.end_query();
        if self.owned.query.result == 0 {
            return self.freeze("candidate close unknown");
        }
        self.owned.handles[slot].close.finish(true);
        Ok(())
    }
    fn recycle_handle(&mut self, slot: usize) -> Result<(), String> {
        self.known()?;
        let h = &mut self.owned.handles[slot];
        if !(h.close.complete || (!h.close.attempted && h.value.is_null())) {
            return self.freeze("candidate scratch handle not settled");
        }
        h.value = null_mut();
        h.close = Default::default();
        Ok(())
    }
    fn allocation(&mut self, kind: AllocationKind, text: Vec<u16>) -> Result<usize, String> {
        self.known()?;
        if !policy::room(self.owned.allocations.len(), policy::ALLOCATION_LIMIT) {
            return Err(bad("candidate allocation bound"));
        }
        let i = self.owned.allocations.len();
        self.owned.allocations.push(Box::new(AllocationSlot {
            value: null_mut(),
            kind,
            close: Default::default(),
            text,
            returned: false,
            raw: 0,
            free_return: null_mut(),
        }));
        Ok(i)
    }
    fn free_allocation(&mut self, index: usize) -> Result<(), String> {
        self.known()?;
        let a = &mut self.owned.allocations[index];
        if a.value.is_null() || !a.close.arm() {
            return self.freeze("candidate allocation settlement");
        }
        let pointer = a.value;
        let kind = a.kind;
        self.begin_query()?;
        let complete = match kind {
            AllocationKind::Task => {
                unsafe { CoTaskMemFree(pointer) };
                true // Void API: normal-return contract, not an observed status code.
            }
            AllocationKind::Local => {
                self.owned.allocations[index].free_return = unsafe { LocalFree(pointer) };
                self.owned.allocations[index].free_return.is_null()
            }
        };
        self.end_query();
        if !complete {
            return self.freeze("candidate allocation free unknown");
        }
        self.owned.allocations[index].close.finish(true);
        Ok(())
    }
    fn path_open(&mut self, path: &Path, directory: bool) -> Result<Option<usize>, String> {
        self.known()?;
        let text = wide(path)?;
        if text.len() > policy::PATH_LIMIT + 1 {
            return Err(bad("candidate path bound"));
        }
        if !policy::room(
            self.owned.opens.len()
                + self.owned.renames.len()
                + usize::from(self.mode == policy::Mode::OrdinaryRdd2),
            policy::FRAME_LIMIT,
        ) {
            return Err(bad("candidate frame bound"));
        }
        let slot = self.reserve_handle()?;
        let frame = OpenFrame {
            text,
            unicode: Unicode {
                length: 0,
                maximum: 0,
                buffer: null_mut(),
            },
            attributes: unsafe { zeroed() },
            io: IoStatus {
                status: policy::SENTINEL,
                information: policy::SENTINEL,
            },
            raw: 0,
            returned: false,
            slot,
            parent: None,
        };
        self.owned.opens.push(Box::new(frame));
        let n = self.owned.opens.len() - 1;
        let access = READ_CONTROL_MASK
            | FILE_READ_ATTRIBUTES
            | if directory {
                retained_directory_access()
            } else {
                FILE_GENERIC_READ
            };
        let flags = FILE_FLAG_OPEN_REPARSE_POINT
            | if directory {
                FILE_FLAG_BACKUP_SEMANTICS
            } else {
                0
            };
        // CreateFileW returns its original HANDLE value, not an out-pointer. The
        // original return is deposited into the already-owned cell, never copied
        // from a temporary File/RAII resource.
        self.owned.handles[slot].value = unsafe {
            CreateFileW(
                self.owned.opens[n].text.as_ptr(),
                access,
                FILE_SHARE_READ | if directory { FILE_SHARE_WRITE } else { 0 },
                null(),
                OPEN_EXISTING,
                flags,
                null_mut(),
            )
        };
        self.owned.opens[n].returned = true;
        let h = self.owned.handles[slot].value;
        if h == INVALID_HANDLE_VALUE {
            self.owned.query.error = unsafe { GetLastError() };
            // Documented invalid-handle failure is a known no-acquisition.
            return if matches!(
                self.owned.query.error,
                ERROR_FILE_NOT_FOUND | ERROR_PATH_NOT_FOUND
            ) {
                Ok(None)
            } else {
                Err(bad("candidate source open"))
            };
        }
        if h.is_null() {
            return self.freeze("candidate source original return unknown");
        }
        Ok(Some(slot))
    }
    fn relative(
        &mut self,
        parent: usize,
        name: &str,
        directory: bool,
        disposition: u32,
        descriptor: Option<usize>,
        write: bool,
    ) -> Result<Option<usize>, String> {
        self.known()?;
        if !native_component(name) {
            return Err(bad("candidate component"));
        }
        if !policy::room(
            self.owned.opens.len()
                + self.owned.renames.len()
                + usize::from(self.mode == policy::Mode::OrdinaryRdd2),
            policy::FRAME_LIMIT,
        ) {
            return Err(bad("candidate frame bound"));
        }
        let parent_handle = self.handle(parent)?;
        let sd = if let Some(i) = descriptor {
            let a = &self.owned.allocations[i];
            if a.close.attempted || a.value.is_null() {
                return Err(bad("candidate descriptor lease"));
            }
            a.value
        } else {
            null_mut()
        };
        let text: Vec<u16> = name.encode_utf16().collect();
        let length = u16::try_from(text.len() * 2).map_err(|_| bad("candidate name bound"))?;
        let slot = self.reserve_handle()?;
        self.owned.opens.push(Box::new(OpenFrame {
            text,
            unicode: Unicode {
                length,
                maximum: length,
                buffer: null_mut(),
            },
            attributes: Attributes {
                length: size_of::<Attributes>() as u32,
                root: parent_handle,
                name: null_mut(),
                flags: OBJ_CASE_INSENSITIVE | OBJ_DONT_REPARSE,
                descriptor: sd,
                qos: null_mut(),
            },
            io: IoStatus {
                status: policy::SENTINEL,
                information: policy::SENTINEL,
            },
            raw: 0,
            returned: false,
            slot,
            parent: Some(parent),
        }));
        let n = self.owned.opens.len() - 1;
        let owned: &mut Owned = &mut self.owned;
        let frame = &mut owned.opens[n];
        frame.unicode.buffer = frame.text.as_mut_ptr();
        frame.attributes.name = &mut frame.unicode;
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
        frame.raw = unsafe {
            NtCreateFile(
                &mut owned.handles[slot].value,
                access,
                &mut frame.attributes,
                &mut frame.io,
                null(),
                0,
                FILE_SHARE_READ | if directory { FILE_SHARE_WRITE } else { 0 },
                disposition,
                options,
                null(),
                0,
            )
        };
        frame.returned = true;
        // Evaluate raw first; never read a still-pending IOSB/output HANDLE.
        if frame.raw as u32 == policy::PENDING {
            return self.freeze("candidate open pending");
        }
        let status = frame.io.status;
        if status as u32 == policy::PENDING {
            return self.freeze("candidate open IOSB pending");
        }
        let result = policy::open_completion(
            frame.raw as u32,
            owned.handles[slot].value as usize,
            status,
            frame.io.information,
            disposition,
        );
        match result {
            policy::Completion::Complete => Ok(Some(slot)),
            policy::Completion::Missing => Ok(None),
            policy::Completion::Refused => Err(bad("candidate native open")),
            policy::Completion::Unknown => self.freeze("candidate open outputs unknown"),
        }
    }
    fn require_relative(
        &mut self,
        parent: usize,
        name: &str,
        directory: bool,
        disposition: u32,
        sd: Option<usize>,
        write: bool,
    ) -> Result<usize, String> {
        self.relative(parent, name, directory, disposition, sd, write)?
            .ok_or_else(|| bad("candidate required object missing"))
    }
    fn user(&mut self, initial: bool) -> Result<UserData, String> {
        let probe = if let Some(i) = self.owned.probe_token {
            i
        } else {
            let i = self.reserve_handle()?;
            self.owned.probe_token = Some(i);
            i
        };
        self.recycle_handle(probe)?;
        self.begin_query()?;
        self.owned.query.result = unsafe {
            OpenThreadToken(
                GetCurrentThread(),
                TOKEN_QUERY,
                1,
                &mut self.owned.handles[probe].value,
            )
        } as u32;
        if self.owned.query.result == 0 {
            self.owned.query.error = unsafe { GetLastError() };
        }
        self.end_query();
        if self.owned.query.result != 0 {
            if self.owned.handles[probe].value.is_null()
                || self.owned.handles[probe].value == INVALID_HANDLE_VALUE
            {
                return self.freeze("candidate thread token output");
            }
            self.close_handle(probe)?;
            return Err(bad("impersonated caller"));
        }
        if !self.owned.handles[probe].value.is_null() {
            return self.freeze("candidate failed thread-token partial");
        }
        if self.owned.query.error != ERROR_NO_TOKEN {
            return Err(bad("candidate effective token"));
        }
        let token = if initial {
            self.reserve_handle()?
        } else {
            let i = if let Some(i) = self.owned.scratch_token {
                i
            } else {
                let i = self.reserve_handle()?;
                self.owned.scratch_token = Some(i);
                i
            };
            self.recycle_handle(i)?;
            i
        };
        self.begin_query()?;
        self.owned.query.result = unsafe {
            OpenProcessToken(
                GetCurrentProcess(),
                TOKEN_QUERY | TOKEN_IMPERSONATE,
                &mut self.owned.handles[token].value,
            )
        } as u32;
        if self.owned.query.result == 0 {
            self.owned.query.error = unsafe { GetLastError() };
        }
        self.end_query();
        if self.owned.query.result == 0 {
            if !self.owned.handles[token].value.is_null() {
                return self.freeze("candidate process-token partial");
            }
            return Err(bad("candidate process token"));
        }
        if self.owned.handles[token].value.is_null()
            || self.owned.handles[token].value == INVALID_HANDLE_VALUE
        {
            return self.freeze("candidate process-token success output unknown");
        }
        let h = self.handle(token)?;
        self.owned.query.needed = 0;
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetTokenInformation(
                h,
                TokenElevation,
                (&mut self.owned.query.elevation as *mut u32).cast(),
                4,
                &mut self.owned.query.needed,
            )
        } as u32;
        self.end_query();
        if self.owned.query.result == 0
            || self.owned.query.needed != 4
            || self.owned.query.elevation != 0
        {
            return Err(bad("candidate ordinary user"));
        }
        self.owned.query.needed = 0;
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetTokenInformation(
                h,
                TokenSessionId,
                (&mut self.owned.query.session as *mut u32).cast(),
                4,
                &mut self.owned.query.needed,
            )
        } as u32;
        self.end_query();
        if self.owned.query.result == 0
            || self.owned.query.needed != 4
            || self.owned.query.session == 0
        {
            return Err(bad("candidate user session"));
        }
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetTokenInformation(
                h,
                TokenUser,
                self.owned.query.token.as_mut_ptr().cast(),
                (self.owned.query.token.len() * 8) as u32,
                &mut self.owned.query.needed,
            )
        } as u32;
        self.end_query();
        if self.owned.query.result == 0
            || self.owned.query.needed < size_of::<TOKEN_USER>() as u32
            || self.owned.query.needed as usize > self.owned.query.token.len() * 8
        {
            return Err(bad("candidate token user bounds"));
        }
        let raw_user = unsafe { &*self.owned.query.token.as_ptr().cast::<TOKEN_USER>() };
        let sid = self.sid_copy(
            self.owned.query.token.as_ptr().cast(),
            self.owned.query.needed as usize,
            raw_user.User.Sid.cast(),
        )?;
        let text = policy::sid_text(&sid).ok_or_else(|| bad("candidate SID"))?;
        if !text.starts_with("S-1-5-21-") {
            return Err(bad("candidate ordinary user SID"));
        }
        let result = UserData {
            sid,
            text,
            session: self.owned.query.session,
            token,
        };
        if !initial {
            self.close_handle(token)?;
        }
        Ok(result)
    }
    fn sid_copy(
        &self,
        base: *const u8,
        length: usize,
        pointer: *const c_void,
    ) -> Result<Vec<u8>, String> {
        let offset = (pointer as usize)
            .checked_sub(base as usize)
            .ok_or_else(|| bad("candidate SID offset"))?;
        if pointer.is_null() || offset > length || length - offset < 8 {
            return Err(bad("candidate SID bounds"));
        }
        let count = unsafe { *pointer.cast::<u8>().add(1) } as usize;
        let size = 8 + count * 4;
        if count > 15 || size > length - offset {
            return Err(bad("candidate SID bounds"));
        }
        let sid = unsafe { std::slice::from_raw_parts(pointer.cast::<u8>(), size) }.to_vec();
        policy::sid_text(&sid).ok_or_else(|| bad("candidate SID shape"))?;
        Ok(sid)
    }
    fn known_profile(&mut self, user: &UserData) -> Result<PathBuf, String> {
        let i = self.allocation(AllocationKind::Task, Vec::new())?;
        self.owned.allocations[i].raw = unsafe {
            SHGetKnownFolderPath(
                &FOLDERID_Profile,
                0,
                self.handle(user.token)?,
                (&mut self.owned.allocations[i].value as *mut *mut c_void).cast(),
            )
        };
        self.owned.allocations[i].returned = true;
        let status = self.owned.allocations[i].raw;
        if status > 0 {
            return self.freeze("candidate profile unexpected positive HRESULT");
        }
        if status < 0 {
            // The documented allocation must be freed on failure too. It is not
            // decoded, and free uncertainty prevents every following effect.
            if !self.owned.allocations[i].value.is_null() {
                self.free_allocation(i)?;
            }
            return Err(bad("candidate native profile"));
        }
        if self.owned.allocations[i].value.is_null() {
            return self.freeze("candidate profile allocation output");
        }
        let pointer = self.owned.allocations[i].value.cast::<u16>();
        let mut n = 0;
        while n < 1024 && unsafe { *pointer.add(n) } != 0 {
            n += 1;
        }
        if n == 0 || n >= 1024 {
            return Err(bad("candidate profile bounds"));
        }
        let text = String::from_utf16(unsafe { std::slice::from_raw_parts(pointer, n) })
            .map_err(|_| bad("candidate profile encoding"))?;
        self.free_allocation(i)?;
        Ok(PathBuf::from(text))
    }
    fn private_descriptor(&mut self, user: &UserData, directory: bool) -> Result<usize, String> {
        let inherit = if directory { "OICI" } else { "" };
        let text: Vec<u16> = format!(
            "O:{}D:P(A;{inherit};FA;;;{})(A;{inherit};FA;;;SY)(A;{inherit};FA;;;BA)",
            user.text, user.text
        )
        .encode_utf16()
        .chain(Some(0))
        .collect();
        if text.len() > 1024 {
            return Err(bad("candidate SDDL bounds"));
        }
        let i = self.allocation(AllocationKind::Local, text)?;
        self.owned.allocations[i].raw = unsafe {
            ConvertStringSecurityDescriptorToSecurityDescriptorW(
                self.owned.allocations[i].text.as_ptr(),
                1,
                &mut self.owned.allocations[i].value,
                null_mut(),
            )
        };
        self.owned.allocations[i].returned = true;
        if self.owned.allocations[i].raw == 0 {
            if !self.owned.allocations[i].value.is_null() {
                return self.freeze("candidate descriptor partial output");
            }
            return Err(bad("candidate private descriptor"));
        }
        if self.owned.allocations[i].value.is_null() {
            return self.freeze("candidate descriptor empty output");
        }
        self.begin_query()?;
        self.owned.query.result =
            unsafe { GetSecurityDescriptorLength(self.owned.allocations[i].value) };
        self.end_query();
        if !(20..=policy::SD_LIMIT as u32).contains(&self.owned.query.result) {
            return Err(bad("candidate converted descriptor size"));
        }
        Ok(i)
    }
    fn identity(&mut self, slot: usize, directory: bool) -> Result<policy::Identity, String> {
        let h = self.handle(slot)?;
        self.begin_query()?;
        self.owned.query.result =
            unsafe { GetFileInformationByHandle(h, &mut self.owned.query.info) } as u32;
        self.end_query();
        if self.owned.query.result == 0 {
            return Err(bad("candidate identity query"));
        }
        // ReFS does not guarantee uniqueness of BY_HANDLE_FILE_INFORMATION's
        // 64-bit file index. The complete original FILE_ID_INFO output is owned
        // before this normal synchronous query; failure has no 64-bit fallback.
        if size_of::<FILE_ID_INFO>() != 24 {
            return Err(bad("full file identity ABI"));
        }
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetFileInformationByHandleEx(
                h,
                FileIdInfo,
                (&mut self.owned.query.full_id as *mut FILE_ID_INFO).cast(),
                24,
            )
        } as u32;
        if self.owned.query.result == 0 {
            self.owned.query.error = unsafe { GetLastError() };
        }
        self.end_query();
        if self.owned.query.result == 0 {
            return Err(bad("full file identity unavailable"));
        }
        self.begin_query()?;
        self.owned.query.result = unsafe { GetFileType(h) };
        self.end_query();
        let info = &self.owned.query.info;
        if self.owned.query.result != FILE_TYPE_DISK
            || info.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT != 0
            || (info.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY != 0) != directory
            || (!directory && info.nNumberOfLinks != 1)
        {
            return Err(bad("candidate object kind"));
        }
        let id = policy::Identity {
            volume: info.dwVolumeSerialNumber,
            high: info.nFileIndexHigh,
            low: info.nFileIndexLow,
            volume64: self.owned.query.full_id.VolumeSerialNumber,
            id128: self.owned.query.full_id.FileId.Identifier,
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
        };
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetFinalPathNameByHandleW(
                h,
                self.owned.query.path.as_mut_ptr(),
                self.owned.query.path.len() as u32,
                2,
            )
        };
        self.end_query();
        let n = self.owned.query.result as usize;
        if n == 0 || n > policy::PATH_LIMIT || n >= self.owned.query.path.len() {
            return Err(bad("candidate normalized path bound"));
        }
        let text = String::from_utf16(&self.owned.query.path[..n])
            .map_err(|_| bad("candidate normalized encoding"))?;
        if !text.starts_with(r"\Device\HarddiskVolume") {
            return Err(bad("candidate local volume path"));
        }
        self.owned.query.normalized = text;
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetVolumeInformationByHandleW(
                h,
                null_mut(),
                0,
                null_mut(),
                null_mut(),
                &mut self.owned.query.flags,
                null_mut(),
                0,
            )
        } as u32;
        self.end_query();
        if self.owned.query.result == 0 || self.owned.query.flags & FILE_PERSISTENT_ACLS == 0 {
            return Err(bad("candidate persistent ACL volume"));
        }
        Ok(id)
    }
    fn security(
        &mut self,
        slot: usize,
        user: &UserData,
        private: bool,
        directory: bool,
    ) -> Result<[u8; 32], String> {
        let h = self.handle(slot)?;
        self.owned.query.needed = 0;
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetKernelObjectSecurity(
                h,
                OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                self.owned.query.sd.as_mut_ptr().cast(),
                policy::SD_LIMIT as u32,
                &mut self.owned.query.needed,
            )
        } as u32;
        self.end_query();
        let length = self.owned.query.needed as usize;
        if self.owned.query.result == 0 || !(20..=policy::SD_LIMIT).contains(&length) {
            return Err(bad("candidate SD query bounds"));
        }
        self.owned.query.sd_length = length;
        let descriptor = self.owned.query.sd.as_mut_ptr().cast();
        // These documented descriptor/SID helpers are synchronous nonallocating
        // queries of our retained SD bytes. Every writable pointer/scalar output
        // lives in QueryFrame; none is an owned allocation from those getters.
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetSecurityDescriptorOwner(
                descriptor,
                &mut self.owned.query.owner,
                &mut self.owned.query.defaulted,
            )
        } as u32;
        self.end_query();
        if self.owned.query.result == 0 {
            return Err(bad("candidate descriptor owner"));
        }
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetSecurityDescriptorDacl(
                descriptor,
                &mut self.owned.query.present,
                &mut self.owned.query.acl,
                &mut self.owned.query.defaulted,
            )
        } as u32;
        self.end_query();
        if self.owned.query.result == 0
            || self.owned.query.present == 0
            || self.owned.query.acl.is_null()
        {
            return Err(bad("candidate descriptor DACL"));
        }
        self.begin_query()?;
        self.owned.query.result = unsafe {
            GetSecurityDescriptorControl(
                descriptor,
                &mut self.owned.query.control,
                &mut self.owned.query.revision,
            )
        } as u32;
        self.end_query();
        if self.owned.query.result == 0 {
            return Err(bad("candidate descriptor control"));
        }
        let base = self.owned.query.sd.as_ptr().cast::<u8>();
        let owner = self.sid_copy(base, length, self.owned.query.owner)?;
        if !policy::descriptor_header_allowed(&owner, &user.sid, self.owned.query.control, private)
        {
            return Err(bad("candidate descriptor owner/protection"));
        }
        let acl = self.owned.query.acl;
        let offset = (acl as usize)
            .checked_sub(base as usize)
            .ok_or_else(|| bad("candidate ACL offset"))?;
        if offset > length || length - offset < size_of::<ACL>() {
            return Err(bad("candidate ACL bounds"));
        }
        let size = unsafe { (*acl).AclSize } as usize;
        let count = unsafe { (*acl).AceCount } as usize;
        if size < size_of::<ACL>() || size > length - offset || count > 256 {
            return Err(bad("candidate ACL shape"));
        }
        let mut aces = Vec::with_capacity(count);
        for index in 0..count {
            self.begin_query()?;
            self.owned.query.result =
                unsafe { GetAce(acl, index as u32, &mut self.owned.query.ace) } as u32;
            self.end_query();
            let pointer = self.owned.query.ace;
            if self.owned.query.result == 0 || pointer.is_null() {
                return Err(bad("candidate ACE query"));
            }
            let off = (pointer as usize)
                .checked_sub(acl as usize)
                .ok_or_else(|| bad("candidate ACE offset"))?;
            if off > size || size - off < 8 {
                return Err(bad("candidate ACE bounds"));
            }
            let ace = unsafe { &*pointer.cast::<ACCESS_ALLOWED_ACE>() };
            let bytes = ace.Header.AceSize as usize;
            if bytes < 16
                || bytes > size - off
                || ace.Header.AceType > 1
                || ace.Header.AceFlags & !0x1f != 0
            {
                return Err(bad("candidate ACE shape"));
            }
            let sid = self.sid_copy(
                pointer.cast(),
                bytes,
                unsafe { pointer.cast::<u8>().add(8) }.cast(),
            )?;
            aces.push(policy::Ace {
                kind: ace.Header.AceType,
                flags: ace.Header.AceFlags,
                mask: ace.Mask,
                sid,
            });
        }
        if !policy::descriptor_allowed(
            &owner,
            &user.sid,
            self.owned.query.control,
            &aces,
            private,
            directory,
        ) {
            return Err(bad("candidate descriptor policy"));
        }
        // Hash the untouched entire original SD span, not sorted/canonical ACEs.
        Ok(Sha256::digest(unsafe { std::slice::from_raw_parts(base, length) }).into())
    }
    fn object(
        &mut self,
        slot: usize,
        user: &UserData,
        directory: bool,
        private: bool,
    ) -> Result<Object, String> {
        let id = self.identity(slot, directory)?;
        let sd = self.security(slot, user, private, directory)?;
        Ok(Object {
            slot,
            id,
            sd: Some(sd),
            directory,
            private,
        })
    }

    fn source_directory(&mut self) -> Result<PathBuf, String> {
        self.begin_query()?;
        self.owned.query.result = unsafe {
            candidate_module_name(
                null_mut(),
                self.owned.query.path.as_mut_ptr(),
                self.owned.query.path.len() as u32,
            )
        };
        if self.owned.query.result == 0
            || self.owned.query.result as usize >= self.owned.query.path.len()
        {
            self.owned.query.error = unsafe { GetLastError() };
        }
        self.end_query();
        let n = self.owned.query.result as usize;
        if n == 0
            || n > policy::PATH_LIMIT
            || n >= self.owned.query.path.len()
            || self.owned.query.path[n] != 0
        {
            return Err(bad("candidate module path bound"));
        }
        let path = PathBuf::from(
            String::from_utf16(&self.owned.query.path[..n])
                .map_err(|_| bad("candidate module encoding"))?,
        );
        let runtime = path
            .parent()
            .ok_or_else(|| bad("candidate module parent"))?
            .join("runtime");
        if let Some(slot) = self.path_open(&runtime, true)? {
            self.identity(slot, true)?;
            self.owned.source_directory = Some(slot);
            return Ok(runtime.join("backend"));
        }
        #[cfg(debug_assertions)]
        {
            Ok(PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/backend"))
        }
        #[cfg(not(debug_assertions))]
        {
            Err(bad("candidate installed runtime missing"))
        }
    }
    fn stream(
        &mut self,
        slot: usize,
        offset: u64,
        length: usize,
        reading: bool,
    ) -> Result<usize, String> {
        self.known()?;
        if length == 0 || length > policy::IO_LIMIT || offset > 512 << 20 {
            return Err(bad("candidate stream bounds"));
        }
        let h = self.handle(slot)?;
        self.owned.io.offset = i64::try_from(offset).map_err(|_| bad("candidate stream offset"))?;
        self.owned.io.io = IoStatus {
            status: policy::SENTINEL,
            information: policy::SENTINEL,
        };
        self.owned.io.returned = false;
        self.owned.io.entered = true;
        let frame = &mut self.owned.io;
        frame.raw = if reading {
            unsafe {
                NtReadFile(
                    h,
                    null_mut(),
                    null(),
                    null(),
                    &mut frame.io,
                    frame.buffer.as_mut_ptr().cast(),
                    length as u32,
                    &frame.offset,
                    null(),
                )
            }
        } else {
            unsafe {
                NtWriteFile(
                    h,
                    null_mut(),
                    null(),
                    null(),
                    &mut frame.io,
                    frame.buffer.as_ptr().cast(),
                    length as u32,
                    &frame.offset,
                    null(),
                )
            }
        };
        frame.returned = true;
        // A pending return owns this buffer/IOSB/offset. Do not even evaluate the
        // subsequent helper arguments or inspect the IOSB until raw is settled.
        if frame.raw as u32 == policy::PENDING {
            return self.freeze("candidate stream pending");
        }
        let status = frame.io.status;
        if status as u32 == policy::PENDING {
            return self.freeze("candidate stream IOSB pending");
        }
        let information = frame.io.information;
        if policy::io_completion(frame.raw as u32, status, information, length, reading)
            != policy::Completion::Complete
        {
            return self.freeze("candidate stream unknown");
        }
        self.owned.io.entered = false;
        Ok(information)
    }
    fn hash(&mut self, object: &Object, limit: u64, manifest: bool) -> Result<[u8; 32], String> {
        if object.id.size == 0 || object.id.size > limit || limit > 512 << 20 {
            return Err(bad("candidate hash bounds"));
        }
        let mut hash = Sha256::new();
        let mut offset = 0u64;
        if manifest {
            self.owned.manifest.clear();
        }
        while offset < object.id.size {
            let count = self.stream(
                object.slot,
                offset,
                ((object.id.size - offset).min(policy::IO_LIMIT as u64)) as usize,
                true,
            )?;
            if count == 0 {
                return Err(bad("candidate premature source EOF"));
            }
            hash.update(&self.owned.io.buffer[..count]);
            if manifest {
                if self.owned.manifest.len() + count > policy::MANIFEST_LIMIT as usize {
                    return Err(bad("candidate manifest bound"));
                }
                // Native write into IO backing completed before this pure copy.
                let chunk = self.owned.io.buffer[..count].to_vec();
                self.owned.manifest.extend_from_slice(&chunk);
            }
            offset += count as u64;
        }
        if self.identity(object.slot, false)? != object.id {
            return Err(bad("candidate original source changed"));
        }
        Ok(hash.finalize().into())
    }
    fn copy(
        &mut self,
        source: &Object,
        stage: usize,
        expected: [u8; 32],
        limit: u64,
    ) -> Result<(), String> {
        if source.id.size == 0 || source.id.size > limit {
            return Err(bad("candidate copy bound"));
        }
        let mut hash = Sha256::new();
        let mut offset = 0u64;
        while offset < source.id.size {
            let count = self.stream(
                source.slot,
                offset,
                ((source.id.size - offset).min(policy::IO_LIMIT as u64)) as usize,
                true,
            )?;
            if count == 0 {
                return Err(bad("candidate copy EOF"));
            }
            hash.update(&self.owned.io.buffer[..count]);
            let mut written = 0usize;
            // A short write is progress, not a failed-operation retry. Keep the
            // remaining bytes in the same owned buffer before the next call.
            while written < count {
                let remaining = count - written;
                let n = self.stream(stage, offset + written as u64, remaining, false)?;
                if n == 0 {
                    return Err(bad("candidate zero write progress"));
                }
                written += n;
                if n < remaining {
                    self.owned.io.buffer.copy_within(n..remaining, 0);
                }
            }
            offset += count as u64;
        }
        if <[u8; 32]>::from(hash.finalize()) != expected
            || self.identity(source.slot, false)? != source.id
        {
            return Err(bad("candidate copied source pin"));
        }
        Ok(())
    }
    fn flush(&mut self, slot: usize) -> Result<(), String> {
        let h = self.handle(slot)?;
        self.begin_query()?;
        self.owned.query.result = unsafe { FlushFileBuffers(h) } as u32;
        if self.owned.query.result == 0 {
            self.owned.query.error = unsafe { GetLastError() };
        }
        self.end_query();
        if self.owned.query.result == 0 {
            return self.freeze("candidate flush completion unknown");
        }
        Ok(())
    }
    fn rename(&mut self, parent: usize, stage: usize, name: &str) -> Result<(), String> {
        self.known()?;
        if !component(name) {
            return Err(bad("candidate publish name"));
        }
        if !policy::room(
            self.owned.opens.len()
                + self.owned.renames.len()
                + usize::from(self.mode == policy::Mode::OrdinaryRdd2),
            policy::FRAME_LIMIT,
        ) {
            return Err(bad("candidate frame bound"));
        }
        let text: Vec<u16> = name.encode_utf16().collect();
        let bytes = 20 + text.len() * 2;
        if bytes > 1024 {
            return Err(bad("candidate rename bound"));
        }
        let p = self.handle(parent)?;
        let s = self.handle(stage)?;
        self.owned.renames.push(Box::new(RenameFrame {
            buffer: vec![0; bytes.div_ceil(size_of::<usize>())],
            io: IoStatus {
                status: policy::SENTINEL,
                information: policy::SENTINEL,
            },
            raw: 0,
            returned: false,
            parent,
            stage,
            bytes,
        }));
        let i = self.owned.renames.len() - 1;
        let frame = &mut self.owned.renames[i];
        let header = frame.buffer.as_mut_ptr().cast::<Rename>();
        unsafe {
            (*header).replace = 0;
            (*header).root = p;
            (*header).length = (text.len() * 2) as u32;
            std::ptr::copy_nonoverlapping(
                text.as_ptr(),
                frame.buffer.as_mut_ptr().cast::<u8>().add(20).cast::<u16>(),
                text.len(),
            );
        }
        frame.raw = unsafe {
            NtSetInformationFile(
                s,
                &mut frame.io,
                frame.buffer.as_mut_ptr().cast(),
                bytes as u32,
                10,
            )
        };
        frame.returned = true;
        if frame.raw as u32 == policy::PENDING {
            return self.freeze("candidate rename pending");
        }
        let status = frame.io.status;
        if status as u32 == policy::PENDING {
            return self.freeze("candidate rename IOSB pending");
        }
        if policy::rename_completion(frame.raw as u32, status, frame.io.information, bytes)
            != policy::Completion::Complete
        {
            return self.freeze("candidate rename unknown");
        }
        Ok(())
    }
    fn profile_chain(&mut self, user: &UserData) -> Result<(PathBuf, usize), String> {
        let profile = self.known_profile(user)?;
        let text = profile
            .to_str()
            .ok_or_else(|| bad("candidate profile encoding"))?;
        if text.len() < 4
            || text.as_bytes()[1] != b':'
            || text.as_bytes()[2] != b'\\'
            || !text.as_bytes()[0].is_ascii_alphabetic()
        {
            return Err(bad("candidate profile drive shape"));
        }
        let parts: Vec<&str> = text[3..].split('\\').collect();
        if parts.len() > policy::PROFILE_LIMIT || parts.iter().any(|p| !native_component(p)) {
            return Err(bad("candidate profile components"));
        }
        let root = PathBuf::from(&text[..3]);
        let slot = self
            .path_open(&root, true)?
            .ok_or_else(|| bad("candidate profile volume missing"))?;
        let object = self.object(slot, user, true, false)?;
        self.owned.anchors.push(object);
        let mut parent = slot;
        for part in parts {
            let slot = self.require_relative(parent, part, true, FILE_OPEN, None, false)?;
            let object = self.object(slot, user, true, false)?;
            self.owned.anchors.push(object);
            parent = slot;
        }
        Ok((profile, parent))
    }
    fn capture_asset(
        &mut self,
        index: usize,
        parent: usize,
        fresh: bool,
        user: &UserData,
        file_sd: usize,
    ) -> Result<(), String> {
        let name = self.owned.assets[index].name;
        let expected = self.owned.assets[index].expected;
        let limit = self.owned.assets[index].limit;
        let source = self.owned.assets[index].source.clone();
        let existing = if !fresh || self.mode == policy::Mode::OrdinaryRdd2 {
            self.relative(parent, name, false, FILE_OPEN, None, false)?
        } else {
            None
        };
        let object = if let Some(slot) = existing {
            self.object(slot, user, false, true)?
        } else {
            if !fresh && self.mode == policy::Mode::Candidate4 {
                return Err(bad("existing candidate incomplete"));
            }
            let number = NEXT.fetch_add(1, Ordering::Relaxed);
            if number == u64::MAX {
                return Err(bad("candidate stage sequence exhausted"));
            }
            let stage_name = format!(".candidate-stage-{number}-{index}");
            let stage = self.require_relative(
                parent,
                &stage_name,
                false,
                FILE_CREATE,
                Some(file_sd),
                true,
            )?;
            let created = self.object(stage, user, false, true)?;
            if created.id.size != 0 {
                return Err(bad("candidate new stage not empty"));
            }
            self.copy(&source, stage, expected, limit)?;
            self.flush(stage)?;
            let writer = self.object(stage, user, false, true)?;
            if writer.sd != created.sd || !policy::same_object(writer.id, created.id) {
                return Err(bad("candidate stage identity/SD changed"));
            }
            if self.hash(&writer, limit, false)? != expected {
                return Err(bad("candidate stage hash"));
            }
            self.rename(parent, stage, name)?;
            self.flush(stage)?;
            self.close_handle(stage)?;
            // Win32 only guarantees finalized last-write time after all writers
            // close. This first readonly capture adopts that actual timestamp,
            // never a different object, attributes, links, size, hash or SD.
            let slot = self.require_relative(parent, name, false, FILE_OPEN, None, false)?;
            let reader = self.object(slot, user, false, true)?;
            if !policy::first_readonly_matches(writer.id, reader.id) || reader.sd != writer.sd {
                return Err(bad("candidate readonly handoff identity"));
            }
            reader
        };
        if !policy::downstream_path_units(self.owned.query.normalized.encode_utf16().count()) {
            return Err(bad("cached NT image path exceeds READ21"));
        }
        if !policy::asset_size(index, object.id.size)
            || self.hash(&object, limit, false)? != expected
            || self.identity(source.slot, false)? != source.id
        {
            return Err(bad("candidate cached asset pin"));
        }
        self.owned.assets[index].final_object = Some(object);
        Ok(())
    }
    fn prepare(&mut self) -> Result<(), String> {
        self.known()?;
        // Independent desktop pins are pure inputs. No source selection/query or
        // manifest read occurred before the caller published this operation.
        let mut expected = Vec::new();
        for name in candidate_stage::ROLE_NAMES.iter().take(self.mode.roles()) {
            expected.push(digest(&crate::runtime::component_binary_pin(name)?)?);
        }
        let directory = self.source_directory()?;
        let user = self.user(true)?;
        self.owned.user = Some(user.clone());
        let manifest_slot = self
            .path_open(&directory.join("build-manifest.json"), false)?
            .ok_or_else(|| bad("candidate manifest missing"))?;
        let manifest = Object {
            slot: manifest_slot,
            id: self.identity(manifest_slot, false)?,
            sd: None,
            directory: false,
            private: false,
        };
        if manifest.id.size == 0 || manifest.id.size > policy::MANIFEST_LIMIT {
            return Err(bad("candidate manifest bound"));
        }
        self.owned.manifest_object = Some(manifest.clone());
        self.hash(&manifest, policy::MANIFEST_LIMIT, true)?;
        let value: serde_json::Value = serde_json::from_slice(&self.owned.manifest)
            .map_err(|_| bad("candidate provenance JSON"))?;
        crate::runtime::validate_backend_manifest(&value)?;
        if value["binarySha256"].as_str() != Some(hex(&expected[0]).as_str()) {
            return Err(bad("candidate manifest/desktop Rdd pin"));
        }
        self.owned.manifest_value = Some(value);
        for index in 0..self.mode.roles() {
            let name = candidate_stage::ROLE_NAMES[index];
            let limit = candidate_stage::ROLE_LIMITS[index];
            let slot = self
                .path_open(&directory.join(name), false)?
                .ok_or_else(|| bad("candidate source missing"))?;
            let source = Object {
                slot,
                id: self.identity(slot, false)?,
                sd: None,
                directory: false,
                private: false,
            };
            if !policy::asset_size(index, source.id.size)
                || self.hash(&source, limit, false)? != expected[index]
            {
                return Err(bad("candidate source pin"));
            }
            self.owned.assets.push(Asset {
                source,
                final_object: None,
                expected: expected[index],
                limit,
                name,
            });
        }
        let (profile, mut parent) = self.profile_chain(&user)?;
        let dir_sd = self.private_descriptor(&user, true)?;
        let file_sd = self.private_descriptor(&user, false)?;
        for name in [".orbwin-private", "runtime"] {
            let slot =
                self.require_relative(parent, name, true, FILE_OPEN_IF, Some(dir_sd), false)?;
            let object = self.object(slot, &user, true, true)?;
            self.owned.anchors.push(object);
            parent = slot;
        }
        let role_name = format!("rdd-{}", hex(&expected[0]));
        let (slot, fresh) = match self.mode {
            policy::Mode::Candidate4 => {
                if let Some(slot) =
                    self.relative(parent, &role_name, true, FILE_OPEN, None, false)?
                {
                    (slot, false)
                } else {
                    (
                        self.require_relative(
                            parent,
                            &role_name,
                            true,
                            FILE_CREATE,
                            Some(dir_sd),
                            false,
                        )?,
                        true,
                    )
                }
            }
            policy::Mode::OrdinaryRdd2 => (
                self.require_relative(parent, &role_name, true, FILE_OPEN_IF, Some(dir_sd), false)?,
                false,
            ),
        };
        let object = self.object(slot, &user, true, true)?;
        let normalized_parent = self.owned.query.normalized.clone();
        // This is the actual held parent's normalized NT name, not DOS/profile
        // text. Reject every final READ21 role before any stage/copy/publish.
        if candidate_stage::ROLE_NAMES
            .iter()
            .take(self.mode.roles())
            .any(|name| !policy::downstream_child(&normalized_parent, name))
        {
            return Err(bad("cache NT path exceeds READ21"));
        }
        self.owned.anchors.push(object);
        parent = slot;
        self.owned.cache_path = Some(
            profile
                .join(".orbwin-private")
                .join("runtime")
                .join(&role_name)
                .join("rdd.exe"),
        );
        for index in 0..self.mode.roles() {
            self.capture_asset(index, parent, fresh, &user, file_sd)?;
        }
        self.recheck()?;
        self.free_allocation(file_sd)?;
        self.free_allocation(dir_sd)?;
        Ok(())
    }
    fn recheck(&mut self) -> Result<(), String> {
        self.recheck_with_hash(true)
    }
    fn recheck_with_hash(&mut self, full: bool) -> Result<(), String> {
        self.known()?;
        let user = self
            .owned
            .user
            .clone()
            .ok_or_else(|| bad("candidate user missing"))?;
        let current = self.user(false)?;
        if current.sid != user.sid || current.session != user.session {
            return Err(bad("candidate current user changed"));
        }
        // Object records are pure snapshots, never resource ownership transfers.
        let anchors = self.owned.anchors.clone();
        for object in anchors {
            if self.identity(object.slot, object.directory)? != object.id
                || Some(self.security(object.slot, &user, object.private, object.directory)?)
                    != object.sd
            {
                return Err(bad("candidate retained parent changed"));
            }
        }
        let manifest = self
            .owned
            .manifest_object
            .clone()
            .ok_or_else(|| bad("candidate provenance owner missing"))?;
        if self.identity(manifest.slot, false)? != manifest.id {
            return Err(bad("candidate provenance object changed"));
        }
        for index in 0..self.owned.assets.len() {
            let source = self.owned.assets[index].source.clone();
            let object = self.owned.assets[index]
                .final_object
                .clone()
                .ok_or_else(|| bad("candidate final sibling missing"))?;
            let expected = self.owned.assets[index].expected;
            let limit = self.owned.assets[index].limit;
            if self.identity(source.slot, false)? != source.id
                || self.identity(object.slot, false)? != object.id
                || Some(self.security(object.slot, &user, true, false)?) != object.sd
                || (full && self.hash(&object, limit, false)? != expected)
            {
                return Err(bad("candidate retained sibling changed"));
            }
        }
        Ok(())
    }
    fn begin_process_query(&mut self) -> Result<(), String> {
        self.known()?;
        self.owned.process.entered = true;
        self.owned.process.returned = false;
        Ok(())
    }
    fn end_process_query(&mut self) {
        self.owned.process.returned = true;
        self.owned.process.entered = false;
    }
    fn process_times(&mut self, process: HANDLE, late: bool) -> Result<u64, String> {
        self.begin_process_query()?;
        let frame = &mut self.owned.process;
        let rows = if late {
            frame.late_times.as_mut_ptr()
        } else {
            frame.times.as_mut_ptr()
        };
        frame.result =
            unsafe { GetProcessTimes(process, rows, rows.add(1), rows.add(2), rows.add(3)) } as u32;
        if frame.result == 0 {
            frame.error = unsafe { GetLastError() };
        }
        self.end_process_query();
        if self.owned.process.result == 0 {
            return Err(bad("ordinary process times"));
        }
        let created = if late {
            self.owned.process.late_times[0]
        } else {
            self.owned.process.times[0]
        };
        // Exit-time output is UNDEFINED for a live process; never use it to judge
        // liveness. Only the actual duplicate's zero-time wait does that.
        Ok((u64::from(created.dwHighDateTime) << 32) | u64::from(created.dwLowDateTime))
    }
    fn process_alive(&mut self, process: HANDLE) -> Result<(), String> {
        self.begin_process_query()?;
        self.owned.process.result = unsafe { WaitForSingleObject(process, 0) };
        if self.owned.process.result == WAIT_FAILED {
            self.owned.process.error = unsafe { GetLastError() };
        }
        self.end_process_query();
        if self.owned.process.result != WAIT_TIMEOUT {
            return Err(bad("ordinary original process not live"));
        }
        Ok(())
    }
    fn verifier_slots(&mut self) -> Result<(usize, usize), String> {
        self.known()?;
        if let (Some(process), Some(image)) =
            (self.owned.process.duplicate, self.owned.process.image)
        {
            if !self.owned.handles[process].close.complete
                || !self.owned.handles[image].close.complete
            {
                return self.freeze("ordinary verifier scratch not positively closed");
            }
            self.recycle_handle(process)?;
            self.recycle_handle(image)?;
            return Ok((process, image));
        }
        // Both fixed original cells exist before DuplicateHandle entry. No
        // successful CLI/inspection call appends another pair of resources.
        if self.owned.handles.len() > policy::HANDLE_LIMIT - 2 {
            return Err(bad("ordinary verifier slot bound"));
        }
        let process = self.reserve_handle()?;
        let image = self.reserve_handle()?;
        self.owned.process.duplicate = Some(process);
        self.owned.process.image = Some(image);
        self.owned.process.image_open.slot = image;
        Ok((process, image))
    }
    fn verify_original(
        &mut self,
        original: std::os::windows::io::BorrowedHandle<'_>,
        expected: Option<(u32, u64, u32)>,
    ) -> Result<(), String> {
        use std::os::windows::io::AsRawHandle;
        self.known()?;
        if original.as_raw_handle().is_null() || original.as_raw_handle() == INVALID_HANDLE_VALUE {
            return Err(bad("ordinary original process handle absent"));
        }
        let (duplicate, image) = self.verifier_slots()?;
        self.owned.process.original_input = original.as_raw_handle() as usize;
        self.begin_process_query()?;
        // The borrowed original is only a live input to this synchronous call.
        // We do not save it as a static HANDLE or reopen any numeric PID.
        self.owned.process.result = unsafe {
            DuplicateHandle(
                GetCurrentProcess(),
                original.as_raw_handle(),
                GetCurrentProcess(),
                &mut self.owned.handles[duplicate].value,
                0,
                0,
                DUPLICATE_SAME_ACCESS,
            )
        } as u32;
        if self.owned.process.result == 0 {
            self.owned.process.error = unsafe { GetLastError() };
        }
        self.end_process_query();
        if self.owned.process.result == 0 && self.owned.process.error == ERROR_IO_PENDING {
            return self.freeze("ordinary process duplication pending");
        }
        let output = self.owned.handles[duplicate].value;
        let aliases = self.owned.handles.iter().enumerate().any(|(i, h)| {
            i != duplicate
                && !h.close.attempted
                && !h.value.is_null()
                && h.value != INVALID_HANDLE_VALUE
                && h.value == output
        });
        match policy::duplicate_completion(
            self.owned.process.result != 0,
            output as usize,
            self.owned.process.original_input,
            aliases,
        ) {
            policy::Completion::Complete => {}
            policy::Completion::Refused => return Err(bad("ordinary original process duplicate")),
            _ => return self.freeze("ordinary original duplicate output/alias unknown"),
        }
        let process = self.handle(duplicate)?;
        self.recheck_with_hash(false)?;
        let user = self
            .owned
            .user
            .clone()
            .ok_or_else(|| bad("ordinary original user missing"))?;
        self.begin_process_query()?;
        self.owned.process.pid = unsafe { GetProcessId(process) };
        if self.owned.process.pid == 0 {
            self.owned.process.error = unsafe { GetLastError() };
        }
        self.end_process_query();
        if self.owned.process.pid == 0 {
            return Err(bad("ordinary original PID"));
        }
        let birth = self.process_times(process, false)?;
        self.begin_process_query()?;
        self.owned.process.result = unsafe {
            ProcessIdToSessionId(self.owned.process.pid, &mut self.owned.process.session)
        } as u32;
        if self.owned.process.result == 0 {
            self.owned.process.error = unsafe { GetLastError() };
        }
        self.end_process_query();
        if self.owned.process.result == 0
            || !policy::process_tuple_matches(
                (self.owned.process.pid, birth, self.owned.process.session),
                expected,
                user.session,
            )
        {
            return Err(bad("ordinary original generation/session"));
        }
        self.process_alive(process)?;
        self.owned.process.length = self.owned.process.path.len() as u32;
        self.begin_process_query()?;
        self.owned.process.result = unsafe {
            QueryFullProcessImageNameW(
                process,
                0,
                self.owned.process.path.as_mut_ptr(),
                &mut self.owned.process.length,
            )
        } as u32;
        if self.owned.process.result == 0 {
            self.owned.process.error = unsafe { GetLastError() };
        }
        self.end_process_query();
        let length = self.owned.process.length as usize;
        if self.owned.process.result == 0
            || length == 0
            || length > policy::PATH_LIMIT
            || length >= self.owned.process.path.len()
            || self.owned.process.path[..length].contains(&0)
        {
            return Err(bad("ordinary original image path"));
        }
        String::from_utf16(&self.owned.process.path[..length])
            .map_err(|_| bad("ordinary original image encoding"))?;
        // The fixed image-open frame owns the exact UTF16 backing before entry.
        {
            let owned: &mut Owned = &mut self.owned;
            let frame: &mut ProcessFrame = &mut owned.process;
            frame.image_open.text[..length].copy_from_slice(&frame.path[..length]);
            frame.image_open.text[length] = 0;
            frame.image_open.returned = false;
        }
        self.owned.handles[image].value = unsafe {
            CreateFileW(
                self.owned.process.image_open.text.as_ptr(),
                FILE_GENERIC_READ | READ_CONTROL_MASK,
                FILE_SHARE_READ,
                null(),
                OPEN_EXISTING,
                FILE_FLAG_OPEN_REPARSE_POINT,
                null_mut(),
            )
        };
        self.owned.process.image_open.returned = true;
        if self.owned.handles[image].value == INVALID_HANDLE_VALUE {
            self.owned.process.error = unsafe { GetLastError() };
            return Err(bad("ordinary original image open"));
        }
        if self.owned.handles[image].value.is_null() {
            return self.freeze("ordinary original image output unknown");
        }
        let output = self.owned.handles[image].value;
        if output as usize == self.owned.process.original_input
            || self.owned.handles.iter().enumerate().any(|(i, h)| {
                i != image
                    && !h.close.attempted
                    && !h.value.is_null()
                    && h.value != INVALID_HANDLE_VALUE
                    && h.value == output
            })
        {
            return self.freeze("ordinary image output aliases another original owner");
        }
        let cached = self
            .owned
            .assets
            .first()
            .and_then(|a| a.final_object.as_ref())
            .ok_or_else(|| bad("ordinary cached image absent"))?
            .id;
        if self.identity(image, false)? != cached {
            return Err(bad("ordinary original image is a different object"));
        }
        if !policy::downstream_path_units(self.owned.query.normalized.encode_utf16().count()) {
            return Err(bad("ordinary actual cached image path exceeds READ21"));
        }
        if self.process_times(process, true)? != birth {
            return Err(bad("ordinary original birth changed"));
        }
        self.process_alive(process)?;
        self.recheck_with_hash(false)?;
        // No borrowed process needs to outlive this call: the original duplicate
        // stays retained until every query and image check has normally settled.
        self.close_handle(image)?;
        self.close_handle(duplicate)?;
        Ok(())
    }
}
