//! Adapt a contained WinPTY command to the shared asynchronous process driver.

use std::io::Read;
use std::io::Write;
use std::os::windows::io::AsRawHandle;
use std::sync::Arc;
use tokio::sync::broadcast;
use tokio::sync::mpsc;
use tokio::sync::oneshot;
use winapi::um::processthreadsapi::GetExitCodeProcess;
use winapi::um::synchapi::WaitForSingleObject;
use winapi::um::winbase::INFINITE;

use crate::ProcessDriver;
use crate::SpawnedProcess;
use crate::WindowsTtyInputNormalizer;
use crate::spawn_from_driver;
use crate::win::winpty::WinPtySpawn;

/// Owns all transport and launch resources through command exit and final output.
pub fn spawn_driver(spawned: WinPtySpawn, timeout_ms: Option<u64>) -> SpawnedProcess {
    let WinPtySpawn {
        session,
        process,
        thread,
        mut input,
        mut output,
        owner,
        job,
    } = spawned;
    let (writer_tx, mut writer_rx) = mpsc::channel::<Vec<u8>>(128);
    let (stdout_tx, stdout_rx) = broadcast::channel::<Vec<u8>>(256);
    let output_join = std::thread::spawn(move || {
        let mut bytes = [0u8; 8192];
        loop {
            match output.read(&mut bytes) {
                Ok(0) => break,
                Ok(count) => {
                    let _ = stdout_tx.send(bytes[..count].to_vec());
                }
                Err(error) if error.kind() == std::io::ErrorKind::Interrupted => continue,
                Err(_) => break,
            }
        }
    });
    let writer_handle = tokio::task::spawn_blocking(move || {
        let mut normalizer = WindowsTtyInputNormalizer::default();
        while let Some(bytes) = writer_rx.blocking_recv() {
            if input.write_all(&normalizer.normalize(&bytes)).is_err() || input.flush().is_err() {
                break;
            }
        }
    });
    let (exit_tx, exit_rx) = oneshot::channel();
    let wait_job = Arc::clone(&job);
    let wait_session = Arc::clone(&session);
    std::thread::spawn(move || {
        let _owner = owner;
        let _thread = thread;
        let timeout = timeout_ms.map_or(INFINITE, |ms| ms.min(u64::from(u32::MAX - 1)) as u32);
        let result = unsafe { WaitForSingleObject(process.as_raw_handle().cast(), timeout) };
        let timed_out = result == 258;
        if timed_out || result == u32::MAX {
            let _ = wait_job.terminate();
            unsafe { WaitForSingleObject(process.as_raw_handle().cast(), INFINITE) };
        } else {
            let _ = wait_job.preserve_descendants();
        }
        let mut exit_code = 1;
        unsafe { GetExitCodeProcess(process.as_raw_handle().cast(), &mut exit_code) };
        // AUTO_SHUTDOWN + EXIT_AFTER_SHUTDOWN make the agent emit the final
        // console frame and close output before it exits. Do not free it early.
        let _ = output_join.join();
        if let Ok(mut console) = wait_session.lock() {
            console.close();
        }
        let _ = exit_tx.send(if timed_out { 124 } else { exit_code as i32 });
    });
    spawn_from_driver(ProcessDriver {
        writer_tx,
        stdout_rx,
        stderr_rx: None,
        exit_rx,
        terminator: Some(Box::new(move || {
            let _ = job.terminate();
        })),
        writer_handle: Some(writer_handle),
        resizer: Some(Box::new(move |size| {
            session
                .lock()
                .map_err(|_| anyhow::anyhow!("WinPTY session lock poisoned"))?
                .resize(size)
        })),
        tty: true,
    })
}
