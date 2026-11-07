// Included privately inside native import_owner. A public routing draft is NOT
// source/code/cache/storage admission. Only actual native constructors seal it.
mod source_route {
    use super::*;
    const BAD_MUTATION: u32 =
        0x10000 | 0x40000 | 0x80000 | 0x40 | 0x2 | 0x100 | 0x10 | 0x10000000 | 0x40000000;
    #[derive(Clone, Copy, Debug, PartialEq, Eq)]
    enum Policy {
        OldStrict,
        ForeignMutation,
    }
    #[derive(Clone, Copy, Debug, PartialEq, Eq)]
    enum State {
        OldStrict,
        NeedsBoundary,
        Protected,
    }
    #[derive(Clone, Copy)]
    struct AceFact {
        kind: u8,
        flags: u8,
        mask: u32,
        trusted: bool,
    }
    // Classification DATA only. This never produces a native route/capability.
    fn classify(owner_trusted: bool, aces: &[AceFact]) -> Result<Policy, ClientConfigError> {
        if !owner_trusted {
            return Err(err(ErrorKind::RequiresLegacyReview));
        }
        let mut foreign = false;
        for a in aces {
            if a.kind > 1 || a.flags & !0x1f != 0 {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            if a.flags & 0x08 != 0 || a.kind == 1 || a.trusted {
                continue;
            }
            foreign |= a.mask & BAD_MUTATION != 0;
        }
        Ok(if foreign {
            Policy::ForeignMutation
        } else {
            Policy::OldStrict
        })
    }
    fn advance(
        state: State,
        policy: Policy,
        actual_protected: bool,
    ) -> Result<State, ClientConfigError> {
        match (state, policy, actual_protected) {
            (State::Protected, Policy::ForeignMutation, _) => {
                Err(err(ErrorKind::RequiresLegacyReview))
            }
            (State::Protected, Policy::OldStrict, _) => Ok(State::Protected),
            (_, Policy::OldStrict, true) => Ok(State::Protected),
            (State::OldStrict, Policy::OldStrict, false) => Ok(State::OldStrict),
            (_, _, _) => Ok(State::NeedsBoundary),
        }
    }
    struct Parsed {
        policy: Policy,
        exact_private: bool,
    }
    // Exact existing private-directory policy, computed only from successfully
    // parsed actual SD facts. A failed native conversion is NOT "no boundary".
    fn exact_private(
        owner_is_user: bool,
        protected: bool,
        aces: &[(AceFact, String)],
        user_text: &str,
    ) -> bool {
        if !owner_is_user || !protected {
            return false;
        }
        let mut actual = Vec::new();
        for (a, sid) in aces {
            if a.kind != 0 || a.flags != 3 || a.mask != 0x1f01ff {
                return false;
            }
            actual.push(sid.as_str());
        }
        actual.sort();
        let mut expected = vec![user_text, "S-1-5-18", "S-1-5-32-544"];
        expected.sort();
        actual == expected
    }
    fn parsed_policy(sd: &Security, user: &User) -> Result<Parsed, ClientConfigError> {
        let (owner, acl, control) = sd.owner_acl().map_err(fresh_failure)?;
        let owner_trusted = Security::trusted(&owner, user).map_err(fresh_failure)?;
        let mut facts = Vec::new();
        for (kind, flags, mask, sid) in sd.aces(acl).map_err(fresh_failure)? {
            let fact = AceFact {
                kind,
                flags,
                mask,
                trusted: Security::trusted(&sid, user).map_err(fresh_failure)?,
            };
            facts.push((fact, sid_text(&sid).map_err(fresh_failure)?))
        }
        Ok(Parsed {
            policy: classify(
                owner_trusted,
                &facts.iter().map(|(a, _)| *a).collect::<Vec<_>>(),
            )?,
            exact_private: exact_private(
                owner == user.sid,
                control & SE_DACL_PROTECTED != 0,
                &facts,
                &user.text,
            ),
        })
    }
    fn complete(state: State) -> Result<(), ClientConfigError> {
        if state == State::NeedsBoundary {
            Err(err(ErrorKind::RequiresLegacyReview))
        } else {
            Ok(())
        }
    }
    // Sequencing only, used by the actual native edge observer. An inspection
    // refusal is known and may checked-close the temporary; an unwind leaves
    // it in the actual owning slot. A failed close stops all later work.
    fn inspect_and_close<T, E>(
        slot: &mut Option<T>,
        inspect: impl FnOnce(&T) -> Result<(), E>,
        close: impl FnOnce(T) -> Result<(), E>,
    ) -> Result<(), E> {
        let observed = inspect(slot.as_ref().unwrap());
        close(slot.take().unwrap())?;
        observed
    }
    fn verified_read<T, E>(
        check: impl FnOnce() -> Result<(), E>,
        read: impl FnOnce() -> Result<T, E>,
    ) -> Result<T, E> {
        check()?;
        read()
    }
    fn capture_source(file: File, user: &User) -> Result<Held, ClientConfigError> {
        let mut pending = BootstrapFile(Some(CheckedFile(Some(file))));
        let id = identity(&pending, false).map_err(fresh_failure)?;
        let sd = Security::read_file(&pending).map_err(fresh_failure)?;
        sd.integrity(user).map_err(fresh_failure)?;
        if sd.owner_acl().map_err(fresh_failure)?.0 != user.sid {
            return Err(err(ErrorKind::RequiresLegacyReview));
        }
        Ok(Held {
            file: pending.0.take().unwrap(),
            id,
            security: sd.hash(),
            directory: false,
            private: false,
        })
    }
    struct Node {
        file: Option<CheckedFile>,
        id: Identity,
        security: [u8; 32],
        name: Option<String>,
        policy: Policy,
        protected: bool,
    }
    impl Node {
        fn capture(
            file: File,
            user: &User,
            name: Option<String>,
            boundary_eligible: bool,
        ) -> Result<Self, ClientConfigError> {
            // Actual output is owned before ANY identity/SD/policy inspection.
            let mut pending = BootstrapFile(Some(CheckedFile(Some(file))));
            let id = identity(&pending, true).map_err(fresh_failure)?;
            let sd = Security::read(&pending).map_err(fresh_failure)?;
            let parsed = parsed_policy(&sd, user)?;
            let policy = parsed.policy;
            let protected = boundary_eligible && parsed.exact_private;
            if protected {
                sd.private(user, true).map_err(fresh_failure)?;
            }
            // Policy matching is attempted only after successful owner/ACL parsing.
            // Unsupported/native errors never become a deferred public route.
            let file = pending.0.take().unwrap();
            Ok(Self {
                file: Some(file),
                id,
                security: sd.hash(),
                name,
                policy,
                protected,
            })
        }
        fn file(&self) -> &File {
            self.file.as_ref().unwrap()
        }
        fn check(&self, user: &User) -> Result<(), ClientConfigError> {
            if identity(self.file(), true).map_err(fresh_failure)? != self.id {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            let sd = Security::read(self.file()).map_err(fresh_failure)?;
            let parsed = parsed_policy(&sd, user)?;
            if sd.hash() != self.security || parsed.policy != self.policy {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            if self.protected {
                if !parsed.exact_private {
                    return Err(err(ErrorKind::RequiresLegacyReview));
                }
                sd.private(user, true).map_err(fresh_failure)?
            }
            Ok(())
        }
        fn close(&mut self) -> Result<(), ClientConfigError> {
            if let Some(f) = self.file.take() {
                close_file(f).map_err(fresh_failure)?
            }
            Ok(())
        }
        fn retain(&mut self) {
            if let Some(f) = self.file.take() {
                std::mem::forget(f)
            }
        }
    }
    impl Drop for Node {
        fn drop(&mut self) {
            if std::thread::panicking() || writer_resources_known().is_err() {
                retain_unknown();
                self.retain()
            } else {
                let _ = self.close();
            }
        }
    }
    pub(super) struct Draft {
        nodes: Vec<Node>,
        state: State,
        user_sid: Vec<u8>,
        session: u32,
    }
    impl Draft {
        pub(super) fn known_local(user: &User) -> Result<Self, ClientConfigError> {
            writer_resources_known()?;
            let local = known_local(user).map_err(fresh_failure)?;
            let text = local.to_str().ok_or_else(|| err(ErrorKind::Unavailable))?;
            if text.len() < 4
                || text.as_bytes()[1..3] != *b":\\"
                || !text.as_bytes()[0].is_ascii_alphabetic()
            {
                return Err(err(ErrorKind::Unavailable));
            }
            let parts: Vec<_> = text[3..].split('\\').collect();
            if parts.len() > 16 || parts.iter().any(|p| !native_component(p)) {
                return Err(err(ErrorKind::Unavailable));
            }
            let volume = wide(Path::new(&text[..3])).map_err(fresh_failure)?;
            let handle = unsafe {
                CreateFileW(
                    volume.as_ptr(),
                    READ_CONTROL_MASK | FILE_READ_ATTRIBUTES | retained_directory_access(false),
                    FILE_SHARE_READ | FILE_SHARE_WRITE,
                    null(),
                    OPEN_EXISTING,
                    FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT,
                    null_mut(),
                )
            };
            let output = capture_output(handle);
            let file = codec::native_output(
                !handle.is_null() && handle != INVALID_HANDLE_VALUE,
                output,
                uncertain_output,
                || err(ErrorKind::Unavailable),
            )?
            .take_file();
            let root = Node::capture(file, user, None, false)?;
            let state = advance(State::OldStrict, root.policy, false)?;
            let mut draft = Self {
                nodes: vec![root],
                state,
                user_sid: user.sid.clone(),
                session: user.session,
            };
            for part in parts {
                draft.append(part, user, false)?
            }
            Ok(draft)
        }
        fn append(
            &mut self,
            name: &str,
            user: &User,
            boundary_eligible: bool,
        ) -> Result<(), ClientConfigError> {
            let file = relative_status(
                self.nodes.last().unwrap().file(),
                name,
                true,
                FILE_OPEN,
                None,
                false,
            )
            .map_err(mapped)?;
            let node = Node::capture(file, user, Some(name.into()), boundary_eligible)?;
            let next = advance(self.state, node.policy, node.protected)?;
            self.nodes.push(node);
            self.state = next;
            Ok(())
        }
        pub(super) fn source(mut self, user: &User) -> Result<Completed, ClientConfigError> {
            for name in ["OrbWinData", "home", ".rdorbwin", "docker"] {
                self.append(name, user, true)?
            }
            complete(self.state)?;
            let file = relative_status(
                self.nodes.last().unwrap().file(),
                "config.json",
                false,
                FILE_OPEN,
                None,
                false,
            )
            .map_err(mapped)?;
            let source = capture_source(file, user)?;
            let completed = Completed {
                draft: Some(self),
                source: Some(source),
                pending: std::sync::Mutex::new(None),
            };
            completed.check(user)?;
            Ok(completed)
        }
        fn close(&mut self) -> Result<(), ClientConfigError> {
            while let Some(mut node) = self.nodes.pop() {
                if let Err(e) = node.close() {
                    std::mem::forget(node);
                    return Err(e);
                }
            }
            Ok(())
        }
        fn retain(&mut self) {
            for node in &mut self.nodes {
                node.retain()
            }
        }
    }
    impl Drop for Draft {
        fn drop(&mut self) {
            if std::thread::panicking() || writer_resources_known().is_err() {
                retain_unknown();
                self.retain()
            } else if self.close().is_err() {
                self.retain()
            }
        }
    }
    pub(super) struct Completed {
        draft: Option<Draft>,
        source: Option<Held>,
        pending: std::sync::Mutex<Option<CheckedFile>>,
    }
    impl Completed {
        pub(super) fn source(&self) -> &Held {
            self.source.as_ref().unwrap()
        }
        fn observe_edge(
            &self,
            parent: &File,
            name: &str,
            directory: bool,
            expected: (Identity, [u8; 32]),
        ) -> Result<(), ClientConfigError> {
            let mut slot = self.pending.lock().map_err(|_| {
                retain_unknown();
                err(ErrorKind::RecoveryRequired)
            })?;
            if slot.is_some() {
                retain_unknown();
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let output =
                relative_status(parent, name, directory, FILE_OPEN, None, false).map_err(mapped)?;
            *slot = Some(CheckedFile(Some(output)));
            inspect_and_close(
                &mut slot,
                |f| {
                    let id = identity(f, directory).map_err(fresh_failure)?;
                    let sd = if directory {
                        Security::read(f)
                    } else {
                        Security::read_file(f)
                    }
                    .map_err(fresh_failure)?;
                    if (id, sd.hash()) != expected {
                        return Err(err(ErrorKind::RequiresLegacyReview));
                    }
                    Ok(())
                },
                |f| close_file(f).map_err(fresh_failure),
            )
        }
        pub(super) fn check(&self, user: &User) -> Result<(), ClientConfigError> {
            writer_resources_known()?;
            let draft = self.draft.as_ref().unwrap();
            if draft.user_sid != user.sid || draft.session != user.session {
                return Err(err(ErrorKind::Unavailable));
            }
            let mut state = State::OldStrict;
            for (i, node) in draft.nodes.iter().enumerate() {
                node.check(user)?;
                state = advance(state, node.policy, node.protected)?;
                if i > 0 {
                    self.observe_edge(
                        draft.nodes[i - 1].file(),
                        node.name.as_ref().unwrap(),
                        true,
                        (node.id, node.security),
                    )?
                }
            }
            if state != draft.state {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            complete(state)?;
            let source = self.source();
            source.recheck(user).map_err(fresh_failure)?;
            if Security::read_file(&source.file)
                .map_err(fresh_failure)?
                .owner_acl()
                .map_err(fresh_failure)?
                .0
                != user.sid
            {
                return Err(err(ErrorKind::RequiresLegacyReview));
            }
            self.observe_edge(
                draft.nodes.last().unwrap().file(),
                "config.json",
                false,
                (source.id, source.security),
            )
        }
        pub(super) fn read(&self, user: &User) -> Result<Vec<u8>, ClientConfigError> {
            verified_read(
                || self.check(user),
                || body(&self.source().file).map_err(fresh_failure),
            )
        }
        fn retain(&mut self) {
            if let Some(v) = self.source.take() {
                std::mem::forget(v)
            }
            if let Some(mut d) = self.draft.take() {
                d.retain();
                std::mem::forget(d)
            }
            // An inspection unwind poisons the mutex, but does not surrender
            // ownership of its actual pending FILE_OBJECT.
            let slot = match self.pending.get_mut() {
                Ok(slot) => slot,
                Err(poisoned) => poisoned.into_inner(),
            };
            if let Some(f) = slot.take() {
                std::mem::forget(f)
            }
        }
        pub(super) fn close(mut self) -> Result<(), ClientConfigError> {
            self.release()
        }
        fn release(&mut self) -> Result<(), ClientConfigError> {
            if std::thread::panicking() || writer_resources_known().is_err() {
                retain_unknown();
                self.retain();
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let result = (|| {
                if let Some(f) = self
                    .pending
                    .get_mut()
                    .map_err(|_| {
                        retain_unknown();
                        err(ErrorKind::RecoveryRequired)
                    })?
                    .take()
                {
                    close_file(f).map_err(fresh_failure)?
                }
                if let Some(s) = self.source.take() {
                    close_file(s.file).map_err(fresh_failure)?
                }
                if let Some(d) = self.draft.as_mut() {
                    d.close()?
                };
                self.draft.take();
                Ok(())
            })();
            if result.is_err() {
                self.retain()
            }
            result
        }
    }
    impl Drop for Completed {
        fn drop(&mut self) {
            let _ = self.release();
        }
    }
    #[cfg(test)]
    mod tests {
        use super::*;
        fn ace(mask: u32) -> AceFact {
            AceFact {
                kind: 0,
                flags: 0,
                mask,
                trusted: false,
            }
        }
        #[test]
        fn complete_old_safe_routes_keep_compatibility_without_new_protected_requirement() {
            let p = classify(true, &[ace(0x1200a9)]).unwrap();
            assert_eq!(p, Policy::OldStrict);
            let mut s = State::OldStrict;
            for _ in 0..20 {
                s = advance(s, p, false).unwrap()
            }
            assert_eq!(s, State::OldStrict)
        }
        #[test]
        fn extra_mutation_needs_real_boundary_and_cannot_fall_back_after_it() {
            let p = classify(true, &[ace(0x1f01ff)]).unwrap();
            assert_eq!(p, Policy::ForeignMutation);
            let s = advance(State::OldStrict, p, false).unwrap();
            assert_eq!(s, State::NeedsBoundary);
            assert_eq!(
                advance(s, Policy::OldStrict, false).unwrap(),
                State::NeedsBoundary
            );
            let s = advance(s, Policy::OldStrict, true).unwrap();
            assert_eq!(s, State::Protected);
            assert!(advance(s, p, false).is_err());
            assert!(advance(s, p, true).is_err())
        }
        #[test]
        fn malformed_owner_or_unsupported_aces_never_defer() {
            assert!(classify(false, &[]).is_err());
            for a in [
                AceFact { kind: 2, ..ace(0) },
                AceFact {
                    flags: 0x20,
                    ..ace(0)
                },
            ] {
                assert!(classify(true, &[a]).is_err())
            }
            assert_eq!(
                classify(
                    true,
                    &[
                        AceFact {
                            flags: 8,
                            ..ace(0x1f01ff)
                        },
                        AceFact {
                            kind: 1,
                            ..ace(0x1f01ff)
                        },
                        AceFact {
                            trusted: true,
                            ..ace(0x1f01ff)
                        }
                    ]
                )
                .unwrap(),
                Policy::OldStrict
            )
        }
        #[test]
        fn boundary_requires_exact_owner_protection_and_three_principals() {
            let principal = |sid: &str| {
                (
                    AceFact {
                        kind: 0,
                        flags: 3,
                        mask: 0x1f01ff,
                        trusted: true,
                    },
                    sid.into(),
                )
            };
            let user = "S-1-5-21-1";
            let mut aces = vec![
                principal(user),
                principal("S-1-5-18"),
                principal("S-1-5-32-544"),
            ];
            assert!(exact_private(true, true, &aces, user));
            aces.reverse();
            assert!(exact_private(true, true, &aces, user));
            assert!(!exact_private(false, true, &aces, user));
            assert!(!exact_private(true, false, &aces, user));
            let saved = aces[0].0;
            for changed in [
                AceFact { flags: 0, ..saved },
                AceFact { kind: 1, ..saved },
                AceFact {
                    mask: saved.mask ^ 4,
                    ..saved
                },
            ] {
                aces[0].0 = changed;
                assert!(!exact_private(true, true, &aces, user));
            }
            aces[0].0 = saved;
            aces.push(principal(user));
            assert!(!exact_private(true, true, &aces, user));
        }
        #[test]
        fn incomplete_route_cannot_read_and_protected_descendants_cannot_restart_search() {
            use std::cell::Cell;
            let reads = Cell::new(0);
            let state = advance(State::OldStrict, Policy::ForeignMutation, false).unwrap();
            let result = verified_read(
                || complete(state),
                || {
                    reads.set(reads.get() + 1);
                    Ok(())
                },
            );
            assert!(result.is_err());
            assert_eq!(reads.get(), 0);
            for state in [State::OldStrict, State::Protected] {
                verified_read(
                    || complete(state),
                    || {
                        reads.set(reads.get() + 1);
                        Ok(())
                    },
                )
                .unwrap();
            }
            assert_eq!(reads.get(), 2);
            assert_eq!(
                advance(State::Protected, Policy::OldStrict, false).unwrap(),
                State::Protected
            );
            assert!(advance(State::Protected, Policy::ForeignMutation, true).is_err());
        }
        #[test]
        fn edge_temporary_is_checked_closed_on_known_refusal_and_close_failure_wins() {
            use std::cell::RefCell;
            let events = RefCell::new(Vec::new());
            for refusal in [false, true] {
                let mut slot = Some(7);
                let result = inspect_and_close(
                    &mut slot,
                    |v| {
                        assert_eq!(*v, 7);
                        events.borrow_mut().push("inspect");
                        if refusal {
                            Err("changed")
                        } else {
                            Ok(())
                        }
                    },
                    |v| {
                        assert_eq!(v, 7);
                        events.borrow_mut().push("close");
                        Ok(())
                    },
                );
                assert_eq!(result, if refusal { Err("changed") } else { Ok(()) });
                assert!(slot.is_none());
            }
            assert_eq!(*events.borrow(), ["inspect", "close", "inspect", "close"]);
            let mut slot = Some(7);
            assert_eq!(
                inspect_and_close(&mut slot, |_| Err("changed"), |_| Err("close-unknown")),
                Err("close-unknown")
            );
            assert!(slot.is_none()); // native close consumes raw handle: never retry it
        }
        #[test]
        fn edge_unwind_keeps_temporary_in_owner_slot_before_outer_guard() {
            let mut slot = Some(7);
            let panic = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
                let _: Result<(), &str> = inspect_and_close(
                    &mut slot,
                    |_| panic!("inspection"),
                    |_| panic!("must not close"),
                );
            }));
            assert!(panic.is_err());
            assert_eq!(slot, Some(7));
        }
    }
}
