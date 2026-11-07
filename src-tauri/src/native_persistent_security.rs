// Private owned acquisition adapter. Policy is shared with legacy, acquisition is not.
#[derive(Default)]
struct SecurityReply {
    entered: bool,
    returned: bool,
    result: i32,
    error: u32,
}
macro_rules! security_call {
    ($reply:expr,$call:expr) => {{
        $reply.entered = true;
        $reply.result = unsafe { $call };
        $reply.error = unsafe { GetLastError() };
        $reply.returned = true;
    }};
}
struct SecurityToken {
    reply: SecurityReply,
    raw: HANDLE,
    owned: HANDLE,
    close_entered: bool,
    close_result: i32,
    close_error: u32,
}
impl SecurityToken {
    fn new() -> Self {
        Self {
            reply: SecurityReply::default(),
            raw: null_mut(),
            owned: null_mut(),
            close_entered: false,
            close_result: 0,
            close_error: 0,
        }
    }
}
struct SidAllocation {
    sid: PSID,
    valid: SecurityReply,
    convert: SecurityReply,
    raw: *mut u16,
    owned: *mut u16,
    free: crate::persistent_security_policy::FreeState,
    free_result: *mut c_void,
    free_error: u32,
}
impl SidAllocation {
    fn new(sid: PSID) -> Self {
        Self {
            sid,
            valid: SecurityReply::default(),
            convert: SecurityReply::default(),
            raw: null_mut(),
            owned: null_mut(),
            free: crate::persistent_security_policy::FreeState::default(),
            free_result: null_mut(),
            free_error: 0,
        }
    }
}
struct PipeSecurity {
    needed: u32,
    sizing: SecurityReply,
    storage: Vec<u64>,
    returned: u32,
    query: SecurityReply,
    valid: SecurityReply,
    exact: u32,
    owner: PSID,
    owner_defaulted: i32,
    owner_query: SecurityReply,
    present: i32,
    acl: *mut ACL,
    dacl_defaulted: i32,
    dacl_query: SecurityReply,
    control: u16,
    revision: u32,
    control_query: SecurityReply,
    aces: [*mut c_void; 3],
    ace_queries: [SecurityReply; 3],
}
impl PipeSecurity {
    fn new() -> Self {
        Self {
            needed: 0,
            sizing: SecurityReply::default(),
            storage: Vec::new(),
            returned: 0,
            query: SecurityReply::default(),
            valid: SecurityReply::default(),
            exact: 0,
            owner: null_mut(),
            owner_defaulted: 0,
            owner_query: SecurityReply::default(),
            present: 0,
            acl: null_mut(),
            dacl_defaulted: 0,
            dacl_query: SecurityReply::default(),
            control: 0,
            revision: 0,
            control_query: SecurityReply::default(),
            aces: [null_mut(); 3],
            ace_queries: std::array::from_fn(|_| SecurityReply::default()),
        }
    }
}
struct Security {
    unknown: bool,
    capture_entered: bool,
    capture_complete: bool,
    dispatch_entered: bool,
    close_entered: bool,
    closed: bool,
    thread: SecurityToken,
    primary: SecurityToken,
    dispatch_thread: SecurityToken,
    elevated: u32,
    elevation_needed: u32,
    elevation_query: SecurityReply,
    session: u32,
    session_needed: u32,
    session_query: SecurityReply,
    user_bytes: [u64; 256],
    user_needed: u32,
    user_query: SecurityReply,
    sids: [SidAllocation; 9],
    sid_count: usize,
    pipe_queries: [PipeSecurity; 2],
    pipe_count: usize,
    user: String,
    sddl: Vec<u16>,
    descriptor_reply: SecurityReply,
    descriptor_raw: *mut c_void,
    descriptor_owned: *mut c_void,
    descriptor_bytes: u32,
    descriptor_valid: SecurityReply,
    descriptor_exact: u32,
    descriptor_free: crate::persistent_security_policy::FreeState,
    descriptor_free_result: *mut c_void,
    descriptor_free_error: u32,
}
// Every pointee is owned by this one stable owner or an original OS allocation;
// only its exclusive continuation may query/close it. No concurrent native use.
unsafe impl Send for Security {}
impl Security {
    fn new() -> Self {
        Self {
            unknown: false,
            capture_entered: false,
            capture_complete: false,
            dispatch_entered: false,
            close_entered: false,
            closed: false,
            thread: SecurityToken::new(),
            primary: SecurityToken::new(),
            dispatch_thread: SecurityToken::new(),
            elevated: 0,
            elevation_needed: 0,
            elevation_query: SecurityReply::default(),
            session: 0,
            session_needed: 0,
            session_query: SecurityReply::default(),
            user_bytes: [0; 256],
            user_needed: 0,
            user_query: SecurityReply::default(),
            sids: std::array::from_fn(|_| SidAllocation::new(null_mut())),
            sid_count: 0,
            pipe_queries: std::array::from_fn(|_| PipeSecurity::new()),
            pipe_count: 0,
            user: String::new(),
            sddl: Vec::new(),
            descriptor_reply: SecurityReply::default(),
            descriptor_raw: null_mut(),
            descriptor_owned: null_mut(),
            descriptor_bytes: 0,
            descriptor_valid: SecurityReply::default(),
            descriptor_exact: 0,
            descriptor_free: crate::persistent_security_policy::FreeState::default(),
            descriptor_free_result: null_mut(),
            descriptor_free_error: 0,
        }
    }
    fn fence(&mut self, stage: &str) -> String {
        self.unknown = true;
        UNKNOWN.store(true, Ordering::Release);
        format!("Persistent security {stage} unknown; original graph retained.")
    }
    fn classify_token(t: &mut SecurityToken) -> crate::persistent_setup_model::HandleReply {
        let reply = crate::persistent_setup_model::duplicate_reply(
            t.reply.result,
            t.raw as usize,
            t.reply.error,
        );
        if reply == crate::persistent_setup_model::HandleReply::Owned {
            t.owned = t.raw
        }
        reply
    }
    fn capture(&mut self) -> Result<(), String> {
        if self.unknown || self.capture_entered || self.closed {
            return Err(self.fence("repeated security capture"));
        }
        self.capture_entered = true;
        security_call!(
            self.thread.reply,
            OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, 1, &mut self.thread.raw)
        );
        if !(crate::persistent_security_policy::thread_token_absent(
            self.thread.reply.result,
            self.thread.raw as usize,
            self.thread.reply.error,
        )) {
            return match Self::classify_token(&mut self.thread) {
                crate::persistent_setup_model::HandleReply::Owned => {
                    Err("Impersonated native client caller refused.".into())
                }
                crate::persistent_setup_model::HandleReply::NoOutput => {
                    Err("Native effective token unavailable.".into())
                }
                crate::persistent_setup_model::HandleReply::Unknown => {
                    Err(self.fence("thread token reply"))
                }
            };
        }
        security_call!(
            self.primary.reply,
            OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut self.primary.raw)
        );
        match Self::classify_token(&mut self.primary) {
            crate::persistent_setup_model::HandleReply::Owned => {}
            crate::persistent_setup_model::HandleReply::NoOutput => {
                return Err("Native process token unavailable.".into())
            }
            crate::persistent_setup_model::HandleReply::Unknown => {
                return Err(self.fence("primary token reply"))
            }
        }
        security_call!(
            self.elevation_query,
            GetTokenInformation(
                self.primary.owned,
                TokenElevation,
                (&mut self.elevated as *mut u32).cast(),
                4,
                &mut self.elevation_needed
            )
        );
        if !crate::persistent_security_policy::scalar_token_query(
            self.elevation_query.result,
            self.elevation_needed,
        ) {
            return Err(self.fence("token elevation query"));
        }
        if self.elevated != 0 {
            return Err("Ordinary native client token required.".into());
        }
        security_call!(
            self.session_query,
            GetTokenInformation(
                self.primary.owned,
                TokenSessionId,
                (&mut self.session as *mut u32).cast(),
                4,
                &mut self.session_needed
            )
        );
        if !crate::persistent_security_policy::scalar_token_query(
            self.session_query.result,
            self.session_needed,
        ) {
            return Err(self.fence("token session query"));
        }
        if self.session == 0 {
            return Err("Interactive native client token required.".into());
        }
        security_call!(
            self.user_query,
            GetTokenInformation(
                self.primary.owned,
                TokenUser,
                self.user_bytes.as_mut_ptr().cast(),
                size_of_val(&self.user_bytes) as u32,
                &mut self.user_needed
            )
        );
        if !crate::persistent_security_policy::user_token_query(
            self.user_query.result,
            self.user_needed,
            size_of_val(&self.user_bytes),
            size_of::<TOKEN_USER>(),
        ) {
            return Err(self.fence("token user query"));
        }
        let sid = unsafe { (*self.user_bytes.as_ptr().cast::<TOKEN_USER>()).User.Sid };
        self.user = self.sid_text(
            self.user_bytes.as_ptr() as usize,
            self.user_needed as usize,
            sid,
        )?;
        if !crate::persistent_security_policy::ordinary_sid(&self.user) {
            return Err("Ordinary native user SID required.".into());
        }
        self.sddl = crate::persistent_security_policy::pipe_sddl(&self.user)
            .encode_utf16()
            .chain(Some(0))
            .collect();
        security_call!(
            self.descriptor_reply,
            ConvertStringSecurityDescriptorToSecurityDescriptorW(
                self.sddl.as_ptr(),
                1,
                &mut self.descriptor_raw,
                &mut self.descriptor_bytes
            )
        );
        if !crate::persistent_security_policy::failed_allocation_size_consistent(
            self.descriptor_reply.result,
            self.descriptor_bytes,
        ) {
            return Err(self.fence("descriptor failed-size reply"));
        }
        match crate::persistent_setup_model::duplicate_reply(
            self.descriptor_reply.result,
            self.descriptor_raw as usize,
            self.descriptor_reply.error,
        ) {
            crate::persistent_setup_model::HandleReply::Owned => {
                if !(self.descriptor_raw as usize).is_multiple_of(4)
                    || !(20..=65536).contains(&self.descriptor_bytes)
                {
                    return Err(self.fence("descriptor allocation bounds"));
                }
                self.descriptor_owned = self.descriptor_raw
            }
            crate::persistent_setup_model::HandleReply::NoOutput => {
                return Err("Pipe descriptor conversion refused.".into())
            }
            crate::persistent_setup_model::HandleReply::Unknown => {
                return Err(self.fence("descriptor allocation reply"))
            }
        }
        security_call!(
            self.descriptor_valid,
            IsValidSecurityDescriptor(self.descriptor_owned)
        );
        if self.descriptor_valid.result == 0 {
            return Err(self.fence("descriptor validity"));
        }
        self.descriptor_exact = unsafe { GetSecurityDescriptorLength(self.descriptor_owned) };
        if self.descriptor_exact != self.descriptor_bytes {
            return Err(self.fence("descriptor allocation shape"));
        }
        self.capture_complete = true;
        Ok(())
    }
    // Pipe connect awaits may move this continuation to another Tokio thread.
    // Re-observe the CURRENT dispatch thread, never infer token-object equality.
    fn recheck_dispatch_thread(&mut self) -> Result<(), String> {
        if self.unknown || !self.capture_complete || self.closed || self.dispatch_entered {
            return Err(self.fence("dispatch capture phase"));
        }
        self.dispatch_entered = true;
        security_call!(
            self.dispatch_thread.reply,
            OpenThreadToken(
                GetCurrentThread(),
                TOKEN_QUERY,
                1,
                &mut self.dispatch_thread.raw
            )
        );
        if crate::persistent_security_policy::thread_token_absent(
            self.dispatch_thread.reply.result,
            self.dispatch_thread.raw as usize,
            self.dispatch_thread.reply.error,
        ) {
            return Ok(());
        }
        match Self::classify_token(&mut self.dispatch_thread) {
            crate::persistent_setup_model::HandleReply::Owned => {
                Err("Impersonated native dispatch refused.".into())
            }
            crate::persistent_setup_model::HandleReply::NoOutput => {
                Err("Native dispatch token unavailable.".into())
            }
            crate::persistent_setup_model::HandleReply::Unknown => {
                Err(self.fence("dispatch token reply"))
            }
        }
    }
    fn sid_text(&mut self, base: usize, length: usize, sid: PSID) -> Result<String, String> {
        let off = crate::persistent_security_policy::sid_offset(base, length, sid as usize)
            .ok_or_else(|| self.fence("SID header bounds"))?;
        let count = unsafe { *sid.cast::<u8>().add(1) };
        crate::persistent_security_policy::sid_size(length - off, count)
            .ok_or_else(|| self.fence("SID size bounds"))?;
        if self.sid_count >= 9 {
            return Err(self.fence("SID allocation quota"));
        }
        self.sids[self.sid_count] = SidAllocation::new(sid);
        let i = self.sid_count;
        self.sid_count += 1;
        let a = &mut self.sids[i];
        security_call!(a.valid, IsValidSid(a.sid));
        if a.valid.result == 0 {
            return Err(self.fence("SID validity"));
        }
        security_call!(a.convert, ConvertSidToStringSidW(a.sid, &mut a.raw));
        match crate::persistent_setup_model::duplicate_reply(
            a.convert.result,
            a.raw as usize,
            a.convert.error,
        ) {
            crate::persistent_setup_model::HandleReply::Owned => {
                if !(a.raw as usize).is_multiple_of(2) {
                    return Err(self.fence("SID allocation alignment"));
                }
                a.owned = a.raw
            }
            crate::persistent_setup_model::HandleReply::NoOutput => {
                return Err("SID conversion refused.".into())
            }
            crate::persistent_setup_model::HandleReply::Unknown => {
                return Err(self.fence("SID allocation reply"))
            }
        }
        let mut n = 0;
        while n < 184 && unsafe { *a.owned.add(n) } != 0 {
            n += 1
        }
        if n == 184 {
            return Err(self.fence("SID text bounds"));
        }
        let text = match String::from_utf16(unsafe { std::slice::from_raw_parts(a.owned, n) }) {
            Ok(text) => text,
            Err(_) => return Err(self.fence("SID encoding")),
        };
        if !a.free.begin() {
            return Err(self.fence("SID repeated free"));
        }
        a.free_result = unsafe { LocalFree(a.owned.cast()) };
        a.free_error = unsafe { GetLastError() };
        if !a.free.finish(a.free_result as usize, a.free_error) {
            return Err(self.fence("SID LocalFree"));
        }
        a.owned = null_mut();
        Ok(text)
    }
    fn verify_pipe(&mut self, h: HANDLE) -> Result<(), String> {
        if self.unknown || !self.capture_complete || self.closed {
            return Err(self.fence("pipe verification phase"));
        }
        if self.pipe_count >= 2 {
            return Err(self.fence("pipe query quota"));
        }
        let i = self.pipe_count;
        self.pipe_count += 1;
        let q = &mut self.pipe_queries[i];
        security_call!(
            q.sizing,
            GetKernelObjectSecurity(
                h,
                OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                null_mut(),
                0,
                &mut q.needed
            )
        );
        if q.sizing.result != 0
            || q.sizing.error != ERROR_INSUFFICIENT_BUFFER
            || !(20..=65536).contains(&q.needed)
        {
            return Err(self.fence("pipe SD size"));
        }
        q.storage = vec![0; (q.needed as usize).div_ceil(8)];
        security_call!(
            q.query,
            GetKernelObjectSecurity(
                h,
                OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                q.storage.as_mut_ptr().cast(),
                (q.storage.len() * 8) as u32,
                &mut q.returned
            )
        );
        if q.query.result == 0 || q.returned as usize > q.storage.len() * 8 {
            return Err(self.fence("pipe SD query"));
        }
        security_call!(
            q.valid,
            IsValidSecurityDescriptor(q.storage.as_mut_ptr().cast())
        );
        if q.valid.result == 0 {
            return Err(self.fence("pipe SD validity"));
        }
        q.exact = unsafe { GetSecurityDescriptorLength(q.storage.as_mut_ptr().cast()) };
        if q.exact < 20 || q.exact > q.needed || q.exact as usize > q.storage.len() * 8 {
            return Err(self.fence("pipe SD exact bounds"));
        }
        security_call!(
            q.owner_query,
            GetSecurityDescriptorOwner(
                q.storage.as_mut_ptr().cast(),
                &mut q.owner,
                &mut q.owner_defaulted
            )
        );
        if q.owner_query.result == 0 {
            return Err(self.fence("pipe owner query"));
        }
        if crate::persistent_security_policy::sid_offset(
            q.storage.as_ptr() as usize,
            q.exact as usize,
            q.owner as usize,
        )
        .is_none()
        {
            return Err(self.fence("pipe owner bounds"));
        }
        security_call!(
            q.dacl_query,
            GetSecurityDescriptorDacl(
                q.storage.as_mut_ptr().cast(),
                &mut q.present,
                &mut q.acl,
                &mut q.dacl_defaulted
            )
        );
        if q.dacl_query.result == 0 {
            return Err(self.fence("pipe DACL query"));
        }
        if q.present == 0 || q.acl.is_null() {
            return Err(self.fence("pipe DACL shape"));
        }
        let acl_offset = (q.acl as usize).checked_sub(q.storage.as_ptr() as usize);
        if acl_offset
            .is_none_or(|off| off > q.exact as usize || q.exact as usize - off < size_of::<ACL>())
        {
            return Err(self.fence("pipe ACL bounds"));
        }
        security_call!(
            q.control_query,
            GetSecurityDescriptorControl(
                q.storage.as_mut_ptr().cast(),
                &mut q.control,
                &mut q.revision
            )
        );
        if q.owner_query.result == 0
            || q.dacl_query.result == 0
            || q.control_query.result == 0
            || q.present == 0
            || q.acl.is_null()
            || !crate::persistent_security_policy::descriptor_control(q.control, q.revision)
        {
            return Err(self.fence("pipe SD control"));
        }
        let (base, exact, owner, acl) = (
            q.storage.as_ptr() as usize,
            q.exact as usize,
            q.owner,
            q.acl,
        );
        if self.sid_text(base, exact, owner)? != self.user {
            return Err("Pipe owner differs.".into());
        }
        let off = (acl as usize)
            .checked_sub(base)
            .ok_or_else(|| self.fence("ACL offset"))?;
        if off > exact || exact - off < size_of::<ACL>() {
            return Err(self.fence("ACL header bounds"));
        }
        let total = unsafe { (*acl).AclSize } as usize;
        if total < size_of::<ACL>()
            || total > exact - off
            || unsafe { (*acl).AceCount } as usize
                != crate::persistent_security_policy::PIPE_ACE_COUNT
        {
            return Err("Pipe ACL scope refused.".into());
        }
        let mut trustees = Vec::new();
        for j in 0..crate::persistent_security_policy::PIPE_ACE_COUNT {
            let q = &mut self.pipe_queries[i];
            security_call!(q.ace_queries[j], GetAce(acl, j as u32, &mut q.aces[j]));
            if q.ace_queries[j].result == 0 || q.aces[j].is_null() {
                return Err(self.fence("ACE query"));
            }
            let raw = q.aces[j];
            let off = (raw as usize)
                .checked_sub(acl as usize)
                .ok_or_else(|| self.fence("ACE offset"))?;
            if off > total || total - off < 16 {
                return Err(self.fence("ACE header bounds"));
            }
            let ace = unsafe { &*raw.cast::<ACCESS_ALLOWED_ACE>() };
            let size = ace.Header.AceSize as usize;
            if size < 16
                || size > total - off
                || !crate::persistent_security_policy::allowed_ace(
                    ace.Header.AceType,
                    ace.Header.AceFlags,
                    ace.Mask,
                )
            {
                return Err("Pipe ACE differs from closed scope.".into());
            }
            trustees.push(self.sid_text(
                raw as usize,
                size,
                unsafe { raw.cast::<u8>().add(8) }.cast(),
            )?);
        }
        if !crate::persistent_security_policy::trustees_match(&self.user, &trustees) {
            return Err("Pipe trustees differ.".into());
        }
        Ok(())
    }
    fn close_token(t: &mut SecurityToken) -> bool {
        if t.owned.is_null() {
            return true;
        }
        if t.close_entered {
            return false;
        }
        t.close_entered = true;
        t.close_result = unsafe { CloseHandle(t.owned) };
        t.close_error = unsafe { GetLastError() };
        if t.close_result == 0 {
            return false;
        }
        t.owned = null_mut();
        true
    }
    fn close(&mut self) -> Result<(), String> {
        if self.unknown {
            return Err("Persistent security already unknown.".into());
        }
        if self.closed {
            return Ok(());
        }
        if self.close_entered {
            return Err(self.fence("repeated security close"));
        }
        self.close_entered = true;
        for a in &mut self.sids {
            if !a.owned.is_null() {
                if !a.free.begin() {
                    return Err(self.fence("SID repeated free"));
                }
                a.free_result = unsafe { LocalFree(a.owned.cast()) };
                a.free_error = unsafe { GetLastError() };
                if !a.free.finish(a.free_result as usize, a.free_error) {
                    return Err(self.fence("SID LocalFree"));
                }
                a.owned = null_mut();
            }
        }
        if !self.descriptor_owned.is_null() {
            if !self.descriptor_free.begin() {
                return Err(self.fence("descriptor repeated free"));
            }
            self.descriptor_free_result = unsafe { LocalFree(self.descriptor_owned) };
            self.descriptor_free_error = unsafe { GetLastError() };
            if !self.descriptor_free.finish(
                self.descriptor_free_result as usize,
                self.descriptor_free_error,
            ) {
                return Err(self.fence("descriptor LocalFree"));
            }
            self.descriptor_owned = null_mut();
        }
        if !Self::close_token(&mut self.dispatch_thread) {
            return Err(self.fence("dispatch token CloseHandle"));
        }
        if !Self::close_token(&mut self.primary) {
            return Err(self.fence("primary token CloseHandle"));
        }
        if !Self::close_token(&mut self.thread) {
            return Err(self.fence("thread token CloseHandle"));
        }
        self.closed = true;
        Ok(())
    }
}
