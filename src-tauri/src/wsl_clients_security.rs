//! Create secret-bearing recovery/replacement files with the original owner and
//! DACL from the outset. Protect copied ACEs from broader directory inheritance.
//! No parent ACL changes, privileges, SACL access, or elevation are requested.
use std::{fs::File, path::Path};

#[cfg(windows)]
pub(super) struct Policy(Vec<u32>, bool);

#[cfg(windows)]
impl Policy {
    pub(super) fn capture(file: &File) -> Result<Self, String> {
        use std::os::windows::io::AsRawHandle;
        use windows_sys::Win32::Security::*;
        let mut needed = 0;
        unsafe {
            GetKernelObjectSecurity(
                file.as_raw_handle(),
                OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                std::ptr::null_mut(),
                0,
                &mut needed,
            );
        }
        if needed == 0 || needed > 65536 {
            return Err("Cannot bound private config owner/DACL; nothing was copied.".into());
        }
        let mut bytes = vec![0u32; (needed as usize).div_ceil(4)];
        if unsafe {
            GetKernelObjectSecurity(
                file.as_raw_handle(),
                OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                bytes.as_mut_ptr().cast(),
                needed,
                &mut needed,
            )
        } == 0
        {
            return Err("Cannot read private config owner/DACL; nothing was copied.".into());
        }
        let mut control = 0;
        let mut revision = 0;
        if unsafe {
            GetSecurityDescriptorControl(bytes.as_mut_ptr().cast(), &mut control, &mut revision)
        } == 0
        {
            return Err("Cannot inspect private config permission inheritance.".into());
        }
        let protected = control & SE_DACL_PROTECTED != 0;
        if unsafe {
            SetSecurityDescriptorControl(
                bytes.as_mut_ptr().cast(),
                SE_DACL_PROTECTED,
                SE_DACL_PROTECTED,
            )
        } == 0
        {
            return Err("Cannot protect copied private config permissions.".into());
        }
        Ok(Self(bytes, protected))
    }

    // Compare meaningful owner/SID and exact ACL bytes; self-relative descriptor
    // offsets/padding/control bookkeeping need not be identical after creation.
    fn fingerprint(&self) -> Result<Vec<u8>, String> {
        use windows_sys::Win32::Security::*;
        let descriptor = self.0.as_ptr().cast_mut().cast();
        let mut owner = std::ptr::null_mut();
        let mut defaulted = 0;
        let mut present = 0;
        let mut acl = std::ptr::null_mut();
        if unsafe { GetSecurityDescriptorOwner(descriptor, &mut owner, &mut defaulted) } == 0
            || owner.is_null()
            || unsafe {
                GetSecurityDescriptorDacl(descriptor, &mut present, &mut acl, &mut defaulted)
            } == 0
        {
            return Err("Cannot inspect private config owner/DACL.".into());
        }
        let sid_length = unsafe { GetLengthSid(owner) } as usize;
        if sid_length == 0 || sid_length > 65536 {
            return Err("Invalid private config owner SID.".into());
        }
        let mut out =
            unsafe { std::slice::from_raw_parts(owner.cast::<u8>(), sid_length) }.to_vec();
        out.extend_from_slice(&present.to_le_bytes());
        out.push(u8::from(acl.is_null()));
        if !acl.is_null() {
            let length = unsafe { (*acl).AclSize } as usize;
            if length < std::mem::size_of::<ACL>() || length > 65536 {
                return Err("Invalid private config ACL size.".into());
            }
            out.extend_from_slice(unsafe { std::slice::from_raw_parts(acl.cast::<u8>(), length) });
        }
        Ok(out)
    }

    pub(super) fn same(&self, other: &Self) -> Result<bool, String> {
        Ok(self.fingerprint()? == other.fingerprint()?)
    }

    pub(super) fn create(&self, path: &Path) -> Result<File, String> {
        use std::os::windows::{ffi::OsStrExt, io::FromRawHandle};
        use windows_sys::Win32::{
            Foundation::INVALID_HANDLE_VALUE, Security::*, Storage::FileSystem::*,
        };
        let path: Vec<u16> = path.as_os_str().encode_wide().chain(Some(0)).collect();
        let attributes = SECURITY_ATTRIBUTES {
            nLength: std::mem::size_of::<SECURITY_ATTRIBUTES>() as u32,
            lpSecurityDescriptor: self.0.as_ptr().cast_mut().cast(),
            bInheritHandle: 0,
        };
        let handle = unsafe {
            CreateFileW(
                path.as_ptr(),
                FILE_GENERIC_READ | FILE_GENERIC_WRITE,
                0,
                &attributes,
                CREATE_NEW,
                FILE_ATTRIBUTE_NORMAL | FILE_FLAG_OPEN_REPARSE_POINT,
                std::ptr::null_mut(),
            )
        };
        if handle == INVALID_HANDLE_VALUE {
            return Err(format!(
                "Cannot create private recovery file with original permissions: {}",
                std::io::Error::last_os_error()
            ));
        }
        let file = unsafe { File::from_raw_handle(handle) };
        // Fail BEFORE writing secret bytes if the OS could not preserve policy.
        let copied = Self::capture(&file)?;
        if !copied.1 || !self.same(&copied)? {
            return Err(
                "Original owner/DACL was not preserved; empty recovery file retained.".into(),
            );
        }
        Ok(file)
    }
}

#[cfg(all(test, windows))]
pub(super) fn restrict_owner_only(path: &Path) {
    use std::os::windows::{fs::OpenOptionsExt, io::AsRawHandle};
    use windows_sys::Win32::{Security::*, Storage::FileSystem::FILE_ALL_ACCESS};
    let file = std::fs::OpenOptions::new()
        .access_mode(0x00040000 | 0x00020000)
        .share_mode(7)
        .open(path)
        .unwrap();
    let original = Policy::capture(&file).unwrap();
    let mut owner = std::ptr::null_mut();
    let mut defaulted = 0;
    assert_ne!(
        unsafe {
            GetSecurityDescriptorOwner(
                original.0.as_ptr().cast_mut().cast(),
                &mut owner,
                &mut defaulted,
            )
        },
        0
    );
    let mut acl = vec![0u32; 1024];
    assert_ne!(
        unsafe { InitializeAcl(acl.as_mut_ptr().cast(), 4096, ACL_REVISION) },
        0
    );
    assert_ne!(
        unsafe {
            AddAccessAllowedAce(
                acl.as_mut_ptr().cast(),
                ACL_REVISION,
                FILE_ALL_ACCESS,
                owner,
            )
        },
        0
    );
    let mut descriptor: SECURITY_DESCRIPTOR = unsafe { std::mem::zeroed() };
    let pointer = (&mut descriptor as *mut SECURITY_DESCRIPTOR).cast();
    assert_ne!(
        unsafe { InitializeSecurityDescriptor(pointer, 1) }, // SECURITY_DESCRIPTOR_REVISION
        0
    );
    assert_ne!(
        unsafe { SetSecurityDescriptorDacl(pointer, 1, acl.as_mut_ptr().cast(), 0) },
        0
    );
    assert_ne!(
        unsafe {
            SetKernelObjectSecurity(
                file.as_raw_handle(),
                DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION,
                pointer,
            )
        },
        0
    );
}

#[cfg(not(windows))]
pub(super) struct Policy(std::fs::Permissions);
#[cfg(not(windows))]
impl Policy {
    pub(super) fn capture(file: &File) -> Result<Self, String> {
        Ok(Self(
            file.metadata().map_err(|e| e.to_string())?.permissions(),
        ))
    }
    pub(super) fn same(&self, other: &Self) -> Result<bool, String> {
        use std::os::unix::fs::PermissionsExt;
        Ok(self.0.mode() == other.0.mode())
    }
    pub(super) fn create(&self, path: &Path) -> Result<File, String> {
        let file = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(path)
            .map_err(|e| e.to_string())?;
        file.set_permissions(self.0.clone())
            .map_err(|e| e.to_string())?;
        Ok(file)
    }
}
