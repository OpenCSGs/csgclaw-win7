//! WinPTY console transport for systems without ConPTY.
//!
//! The bundled DLL extension starts its trusted console agent suspended under
//! the requested token, then assigns the command job before allowing it to run.
//! User commands inherit both the token and the non-breakaway job from that agent.

use anyhow::Context;
use anyhow::Result;
use anyhow::bail;
use filedescriptor::FileDescriptor;
use portable_pty::CommandBuilder;
use shared_library::shared_library;
use std::ffi::c_void;
use std::os::windows::io::AsRawHandle;
use std::os::windows::io::FromRawHandle;
use std::os::windows::io::OwnedHandle;
use std::os::windows::io::RawHandle;
use std::ptr;
use std::sync::Arc;
use std::sync::Mutex;
use std::sync::OnceLock;
use winapi::um::fileapi::CreateFileW;
use winapi::um::fileapi::OPEN_EXISTING;
use winapi::um::handleapi::INVALID_HANDLE_VALUE;
use winapi::um::winbase::CREATE_BREAKAWAY_FROM_JOB;
use winapi::um::winnt::FILE_ATTRIBUTE_NORMAL;
use winapi::um::winnt::GENERIC_READ;
use winapi::um::winnt::GENERIC_WRITE;

use crate::TerminalSize;
use crate::win::JobObject;
use crate::win::legacy_windows_jobs;
use crate::win::psuedocon::build_cmdline;
use crate::win::psuedocon::build_environment_block;
use crate::win::psuedocon::resolve_current_directory;

type Opaque = *mut c_void;

shared_library!(WinPtyApi,
    fn winpty_config_new(flags: u64, error: *mut Opaque) -> Opaque,
    fn winpty_config_free(config: Opaque),
    fn winpty_config_set_initial_size(config: Opaque, cols: i32, rows: i32),
    fn winpty_config_set_spawn_context(config: Opaque, token: RawHandle, job: RawHandle, flags: u32, desktop: *const u16, cwd: *const u16, env: *const u16, env_len: u32, error: *mut Opaque) -> i32,
    fn winpty_open(config: Opaque, error: *mut Opaque) -> Opaque,
    fn winpty_conin_name(pty: Opaque) -> *const u16,
    fn winpty_conout_name(pty: Opaque) -> *const u16,
    fn winpty_spawn_config_new(flags: u64, app: *const u16, command: *const u16, cwd: *const u16, env: *const u16, error: *mut Opaque) -> Opaque,
    fn winpty_spawn_config_free(config: Opaque),
    fn winpty_spawn(pty: Opaque, config: Opaque, process: *mut RawHandle, thread: *mut RawHandle, create_error: *mut u32, error: *mut Opaque) -> i32,
    fn winpty_set_size(pty: Opaque, cols: i32, rows: i32, error: *mut Opaque) -> i32,
    fn winpty_error_msg(error: Opaque) -> *const u16,
    fn winpty_error_free(error: Opaque),
    fn winpty_free(pty: Opaque),
);

static API: OnceLock<std::result::Result<WinPtyApi, String>> = OnceLock::new();

fn api() -> Result<&'static WinPtyApi> {
    API.get_or_init(|| {
        let path = std::env::current_exe()
            .map_err(|error| error.to_string())?
            .parent()
            .context("missing executable directory")
            .map_err(|error| error.to_string())?
            .join("winpty.dll");
        // Load only the library next to this executable. Never search the
        // workspace, PATH, or a caller-provided environment override.
        WinPtyApi::open(&path).map_err(|error| format!("{error:?}"))
    })
    .as_ref()
    .map_err(|error| anyhow::anyhow!("WinPTY backend unavailable: {error}"))
}

pub fn supported() -> bool {
    api().is_ok()
}

fn error(api: &WinPtyApi, raw: Opaque, operation: &str) -> anyhow::Error {
    if raw.is_null() {
        return anyhow::anyhow!("{operation} failed");
    }
    // The library owns a NUL-terminated message until winpty_error_free.
    let message = unsafe {
        let text = (api.winpty_error_msg)(raw);
        let mut len = 0;
        while *text.add(len) != 0 {
            len += 1;
        }
        let message = String::from_utf16_lossy(std::slice::from_raw_parts(text, len));
        (api.winpty_error_free)(raw);
        message
    };
    anyhow::anyhow!("{operation}: {message}")
}

/// Owns an agent and serializes its control operations independently of pipe I/O.
pub struct WinPtySession {
    api: &'static WinPtyApi,
    raw: Option<usize>,
    pub(crate) job: Arc<JobObject>,
}

/// Handles transferred to the caller after a contained command has been started.
pub struct WinPtySpawn {
    pub session: Arc<Mutex<WinPtySession>>,
    pub process: OwnedHandle,
    pub thread: OwnedHandle,
    pub input: FileDescriptor,
    pub output: FileDescriptor,
    /// Platform launch resources retained until process output has drained.
    pub owner: Option<Box<dyn Send>>,
    pub(crate) job: Arc<JobObject>,
}

impl WinPtySession {
    /// Starts a console command with the requested token and an owned command job.
    pub fn spawn(
        command: CommandBuilder,
        token: Option<RawHandle>,
        job: Arc<JobObject>,
        size: TerminalSize,
        desktop: Option<&[u16]>,
    ) -> Result<WinPtySpawn> {
        if size.rows == 0
            || size.cols == 0
            || size.rows > i16::MAX as u16
            || size.cols > i16::MAX as u16
        {
            bail!("invalid WinPTY terminal dimensions");
        }
        if desktop.is_some_and(|name| name.last() != Some(&0)) {
            bail!("WinPTY desktop name must be NUL terminated");
        }
        let api = api()?;
        let mut issue = ptr::null_mut();
        let cfg = unsafe {
            (api.winpty_config_new)(/*flags*/ 0, &mut issue)
        };
        if cfg.is_null() {
            return Err(error(api, issue, "configure WinPTY"));
        }
        let cwd = resolve_current_directory(&command).unwrap_or_else(|| vec![0]);
        let mut env = build_environment_block(&command);
        if env.len() == 1 {
            env.push(0);
        }
        let flags = if legacy_windows_jobs() {
            CREATE_BREAKAWAY_FROM_JOB
        } else {
            0
        };
        let configured = unsafe {
            (api.winpty_config_set_initial_size)(cfg, i32::from(size.cols), i32::from(size.rows));
            (api.winpty_config_set_spawn_context)(
                cfg,
                token.unwrap_or(ptr::null_mut()),
                job.as_raw_handle(),
                flags,
                desktop.map_or(ptr::null(), |name| name.as_ptr()),
                cwd.as_ptr(),
                env.as_ptr(),
                env.len() as u32,
                &mut issue,
            )
        };
        if configured == 0 {
            unsafe { (api.winpty_config_free)(cfg) };
            return Err(error(api, issue, "configure contained WinPTY agent"));
        }
        let raw = unsafe { (api.winpty_open)(cfg, &mut issue) };
        unsafe { (api.winpty_config_free)(cfg) };
        if raw.is_null() {
            return Err(error(api, issue, "start contained WinPTY agent"));
        }
        let session = Self {
            api,
            raw: Some(raw as usize),
            job,
        };
        let open_pipe = |name, access| -> Result<FileDescriptor> {
            let handle = unsafe {
                CreateFileW(
                    name,
                    access,
                    /*dwShareMode*/ 0,
                    ptr::null_mut(),
                    OPEN_EXISTING,
                    FILE_ATTRIBUTE_NORMAL,
                    ptr::null_mut(),
                )
            };
            if handle == INVALID_HANDLE_VALUE {
                return Err(std::io::Error::last_os_error()).context("connect WinPTY console pipe");
            }
            Ok(unsafe { FileDescriptor::from_raw_handle(handle.cast()) })
        };
        let input = open_pipe(unsafe { (api.winpty_conin_name)(raw) }, GENERIC_WRITE)?;
        let output = open_pipe(unsafe { (api.winpty_conout_name)(raw) }, GENERIC_READ)?;
        let (app, line) = build_cmdline(&command)?;
        let spawn_cfg = unsafe {
            (api.winpty_spawn_config_new)(
                /*flags*/ 3,
                app.as_ptr(),
                line.as_ptr(),
                cwd.as_ptr(),
                env.as_ptr(),
                &mut issue,
            )
        };
        if spawn_cfg.is_null() {
            return Err(error(api, issue, "configure WinPTY command"));
        }
        let mut process = ptr::null_mut();
        let mut thread = ptr::null_mut();
        let mut create_error = 0;
        let spawned = unsafe {
            (api.winpty_spawn)(
                raw,
                spawn_cfg,
                &mut process,
                &mut thread,
                &mut create_error,
                &mut issue,
            )
        };
        unsafe { (api.winpty_spawn_config_free)(spawn_cfg) };
        if spawned == 0 {
            return Err(error(api, issue, "start restricted console command"));
        }
        Ok(WinPtySpawn {
            job: Arc::clone(&session.job),
            session: Arc::new(Mutex::new(session)),
            process: unsafe { OwnedHandle::from_raw_handle(process) },
            thread: unsafe { OwnedHandle::from_raw_handle(thread) },
            input,
            output,
            owner: None,
        })
    }

    pub fn resize(&mut self, size: TerminalSize) -> Result<()> {
        let Some(raw) = self.raw else {
            bail!("console session already closed")
        };
        let mut issue = ptr::null_mut();
        if unsafe {
            (self.api.winpty_set_size)(
                raw as Opaque,
                i32::from(size.cols),
                i32::from(size.rows),
                &mut issue,
            )
        } == 0
        {
            return Err(error(self.api, issue, "resize WinPTY console"));
        }
        Ok(())
    }

    pub fn close(&mut self) {
        if let Some(raw) = self.raw.take() {
            unsafe { (self.api.winpty_free)(raw as Opaque) };
        }
    }
}

impl Drop for WinPtySession {
    fn drop(&mut self) {
        let _ = self.job.terminate();
        self.close();
    }
}
