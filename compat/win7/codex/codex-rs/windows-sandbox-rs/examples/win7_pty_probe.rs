//! Native Windows console verification for the Win7 maintenance build.

#[cfg(not(windows))]
fn main() {
    eprintln!("Run this probe on Windows.");
}

#[cfg(windows)]
fn main() -> anyhow::Result<()> {
    use std::io::Write;
    use windows_sys::Win32::Foundation::CloseHandle;
    use windows_sys::Win32::Security::IsTokenRestricted;
    use windows_sys::Win32::Security::TOKEN_QUERY;
    use windows_sys::Win32::System::Console::CONSOLE_SCREEN_BUFFER_INFO;
    use windows_sys::Win32::System::Console::GetConsoleMode;
    use windows_sys::Win32::System::Console::GetConsoleScreenBufferInfo;
    use windows_sys::Win32::System::Console::GetStdHandle;
    use windows_sys::Win32::System::Console::STD_INPUT_HANDLE;
    use windows_sys::Win32::System::Console::STD_OUTPUT_HANDLE;
    use windows_sys::Win32::System::Threading::GetCurrentProcess;
    use windows_sys::Win32::System::Threading::OpenProcessToken;

    let args = std::env::args().collect::<Vec<_>>();
    if args.get(1).is_some_and(|arg| arg == "--wait-child") {
        std::thread::sleep(std::time::Duration::from_secs(300));
        return Ok(());
    }
    if args
        .get(1)
        .is_some_and(|arg| arg == "--console-child" || arg == "--console-tree")
    {
        let mut mode = 0;
        anyhow::ensure!(
            unsafe { GetConsoleMode(GetStdHandle(STD_INPUT_HANDLE), &mut mode) } != 0,
            "stdin is not a real Windows console"
        );
        let mut token = 0;
        anyhow::ensure!(
            unsafe { OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) } != 0,
            "query child token"
        );
        let restricted = unsafe { IsTokenRestricted(token) } != 0;
        unsafe { CloseHandle(token) };
        anyhow::ensure!(
            restricted,
            "console child did not retain its restricted token"
        );
        std::fs::write("pty-inside.txt", "WIN7_PTY_INSIDE")?;
        let outside = args.get(2).expect("outside write fixture path");
        match std::fs::write(outside, "must be denied") {
            Err(error) if error.kind() == std::io::ErrorKind::PermissionDenied => {}
            result => anyhow::bail!("outside write was not denied: {result:?}"),
        }
        println!("WIN7_REAL_CONSOLE_READY RESTRICTED_TOKEN_OK OUTSIDE_WRITE_DENIED");
        std::io::stdout().flush()?;
        if args[1] == "--console-tree" {
            let marker = std::path::PathBuf::from(args.get(3).expect("process marker path"));
            let mut child = match std::process::Command::new(std::env::current_exe()?)
                .arg("--wait-child")
                .spawn()
            {
                Ok(child) => child,
                Err(error) => {
                    std::fs::write(marker.with_extension("error.txt"), error.to_string())?;
                    return Err(error.into());
                }
            };
            let mut pids = [0u32; 32];
            let deadline = std::time::Instant::now() + std::time::Duration::from_secs(10);
            let count = loop {
                let count = unsafe {
                    windows_sys::Win32::System::Console::GetConsoleProcessList(
                        pids.as_mut_ptr(),
                        pids.len() as u32,
                    )
                };
                if count >= 3 || std::time::Instant::now() >= deadline {
                    break count;
                }
                if let Some(status) = child.try_wait()? {
                    std::fs::write(
                        marker.with_extension("error.txt"),
                        format!("descendant exited during console attach: {status}"),
                    )?;
                    anyhow::bail!("descendant exited during console attach");
                }
                std::thread::sleep(std::time::Duration::from_millis(20));
            };
            if count < 3 {
                std::fs::write(
                    marker.with_extension("error.txt"),
                    format!("console process list only has {count}: {pids:?}"),
                )?;
            }
            anyhow::ensure!(
                count >= 3 && count <= pids.len() as u32,
                "console process tree count {count}"
            );
            std::fs::write(marker, serde_json::to_vec(&pids[..count as usize])?)?;
            let _ = child.wait()?;
            return Ok(());
        }
        loop {
            let mut line = String::new();
            if std::io::stdin().read_line(&mut line)? == 0 {
                break;
            }
            let line = line.trim_end_matches(['\r', '\n']);
            if line == "quit" {
                println!("WIN7_PTY_BYE");
                break;
            }
            if line == "size" {
                let mut size: CONSOLE_SCREEN_BUFFER_INFO = unsafe { std::mem::zeroed() };
                anyhow::ensure!(
                    unsafe {
                        GetConsoleScreenBufferInfo(GetStdHandle(STD_OUTPUT_HANDLE), &mut size)
                    } != 0,
                    "read console dimensions"
                );
                println!(
                    "SIZE_{}x{}",
                    size.srWindow.Right - size.srWindow.Left + 1,
                    size.srWindow.Bottom - size.srWindow.Top + 1
                );
            } else {
                println!("ECHO_{line}");
            }
            std::io::stdout().flush()?;
        }
        return Ok(());
    }
    let runtime = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()?;
    if args.get(1).is_some_and(|arg| arg == "--owner") {
        return runtime.block_on(async {
            let base = std::path::PathBuf::from(args.get(2).expect("owner base directory"));
            let (_process, _outside) = launch(&base, "--console-tree", Some(300_000)).await?;
            tokio::time::sleep(std::time::Duration::from_secs(300)).await;
            Ok(())
        });
    }
    runtime.block_on(run())
}

#[cfg(windows)]
async fn launch(
    base: &std::path::Path,
    mode: &str,
    timeout_ms: Option<u64>,
) -> anyhow::Result<(codex_utils_pty::SpawnedProcess, std::path::PathBuf)> {
    use codex_protocol::models::PermissionProfile;
    use codex_utils_absolute_path::AbsolutePathBuf;
    let workspace = base.join("workspace");
    let home = base.join("codex-home");
    std::fs::create_dir_all(&workspace)?;
    std::fs::create_dir_all(&home)?;
    let outside = base.join("outside.txt");
    let roots = vec![AbsolutePathBuf::from_absolute_path(workspace.clone())?];
    let permission = PermissionProfile::workspace_write();
    let command = vec![
        std::env::current_exe()?.display().to_string(),
        mode.to_string(),
        outside.display().to_string(),
        workspace.join("tree-pids.json").display().to_string(),
    ];
    let spawned = codex_windows_sandbox::spawn_windows_sandbox_session_legacy(
        &permission,
        &roots,
        &home,
        command,
        &workspace,
        std::env::vars().collect(),
        timeout_ms,
        &[],
        &[],
        /*tty*/ true,
        /*stdin_open*/ true,
        /*use_private_desktop*/ true,
    )
    .await?;
    Ok((spawned, outside))
}

#[cfg(windows)]
async fn run() -> anyhow::Result<()> {
    use anyhow::Context;
    use codex_utils_pty::SpawnedProcess;
    use codex_utils_pty::TerminalSize;
    use std::os::windows::io::AsRawHandle;
    use std::os::windows::io::FromRawHandle;
    use std::os::windows::io::OwnedHandle;
    use std::path::Path;
    use std::time::Duration;
    use windows_sys::Win32::System::Threading::OpenProcess;
    use windows_sys::Win32::System::Threading::PROCESS_SYNCHRONIZE;
    use windows_sys::Win32::System::Threading::WaitForSingleObject;

    async fn expect(
        spawned: &mut SpawnedProcess,
        output: &mut Vec<u8>,
        marker: &str,
    ) -> anyhow::Result<()> {
        tokio::time::timeout(Duration::from_secs(90), async {
            loop {
                if String::from_utf8_lossy(output).contains(marker) {
                    return Ok(());
                }
                match spawned.stdout_rx.recv().await {
                    Some(chunk) => output.extend(chunk),
                    None => anyhow::bail!(
                        "output ended before {marker}: {}",
                        String::from_utf8_lossy(output)
                    ),
                }
                anyhow::ensure!(
                    output.len() < 1024 * 1024,
                    "unexpected console output volume"
                );
            }
        })
        .await
        .with_context(|| {
            format!(
                "waiting for {marker}; output={}",
                String::from_utf8_lossy(output)
            )
        })?
    }

    let base = std::env::current_dir()?.join(format!("win7-pty-probe-{}", std::process::id()));
    let (mut spawned, outside) =
        launch(&base.join("interactive"), "--console-child", Some(120_000)).await?;
    println!("PHASE interactive started");
    let mut output = Vec::new();
    expect(&mut spawned, &mut output, "OUTSIDE_WRITE_DENIED").await?;
    spawned
        .session
        .writer_sender()
        .send("你好_win7\n".as_bytes().to_vec())
        .await?;
    expect(&mut spawned, &mut output, "ECHO_你好_win7").await?;
    spawned
        .session
        .writer_sender()
        .send(b"abc\x1b[D\x7fZ\r".to_vec())
        .await?;
    expect(&mut spawned, &mut output, "ECHO_aZc").await?;
    spawned.session.resize(TerminalSize {
        cols: 120,
        rows: 35,
    })?;
    spawned
        .session
        .writer_sender()
        .send(b"size\r".to_vec())
        .await?;
    expect(&mut spawned, &mut output, "SIZE_120x35").await?;
    spawned
        .session
        .writer_sender()
        .send(b"quit\r".to_vec())
        .await?;
    expect(&mut spawned, &mut output, "WIN7_PTY_BYE").await?;
    let exit = tokio::time::timeout(Duration::from_secs(30), &mut spawned.exit_rx).await??;
    anyhow::ensure!(exit == 0, "console exit code {exit}");
    anyhow::ensure!(!outside.exists(), "outside write escaped workspace");
    println!("PHASE interactive passed");

    let (mut interrupted, _) =
        launch(&base.join("ctrl-c"), "--console-child", Some(120_000)).await?;
    expect(&mut interrupted, &mut Vec::new(), "OUTSIDE_WRITE_DENIED").await?;
    interrupted.session.writer_sender().send(vec![3]).await?;
    println!("PHASE Ctrl+C sent");
    let interrupt_exit = tokio::time::timeout(Duration::from_secs(30), &mut interrupted.exit_rx)
        .await
        .context("Ctrl+C console exit")??;
    anyhow::ensure!(
        interrupt_exit != 0,
        "Ctrl+C did not interrupt the console process"
    );
    println!("PHASE Ctrl+C passed");

    async fn track(path: &Path) -> anyhow::Result<Vec<OwnedHandle>> {
        let pids: Vec<u32> = tokio::time::timeout(Duration::from_secs(60), async {
            loop {
                if let Ok(data) = std::fs::read(path) {
                    return serde_json::from_slice(&data);
                }
                if let Ok(error) = std::fs::read_to_string(path.with_extension("error.txt")) {
                    return Err(serde_json::Error::io(std::io::Error::other(error)));
                }
                tokio::time::sleep(Duration::from_millis(50)).await;
            }
        })
        .await??;
        anyhow::ensure!(pids.len() >= 3, "agent/command/descendant not all tracked");
        let mut handles = Vec::new();
        for pid in pids {
            let handle = unsafe { OpenProcess(PROCESS_SYNCHRONIZE, 0, pid) };
            anyhow::ensure!(handle != 0, "open console process {pid}");
            let handle = unsafe { OwnedHandle::from_raw_handle(handle as _) };
            anyhow::ensure!(
                unsafe { WaitForSingleObject(handle.as_raw_handle() as _, 0) } == 258,
                "console process already exited"
            );
            handles.push(handle);
        }
        Ok(handles)
    }

    fn exited(handles: &[OwnedHandle]) -> anyhow::Result<()> {
        for handle in handles {
            anyhow::ensure!(
                unsafe { WaitForSingleObject(handle.as_raw_handle() as _, 15000) } == 0,
                "console agent or descendant survived"
            );
        }
        Ok(())
    }

    for timed_out in [false, true] {
        println!("PHASE tree timed_out={timed_out}");
        let case = base.join(if timed_out { "timeout" } else { "cancel" });
        let (mut tree, _) = launch(
            &case,
            "--console-tree",
            Some(if timed_out { 15_000 } else { 120_000 }),
        )
        .await?;
        let handles = track(&case.join("workspace/tree-pids.json")).await?;
        if !timed_out {
            tree.session.request_terminate();
        }
        let exit = tokio::time::timeout(Duration::from_secs(45), &mut tree.exit_rx)
            .await
            .context("tree exit")??;
        anyhow::ensure!(
            exit != 0 && (!timed_out || exit == 124),
            "unexpected tree exit {exit}"
        );
        exited(&handles)?;
    }

    let case = base.join("owner-crash");
    println!("PHASE owner crash");
    std::fs::create_dir_all(&case)?;
    let mut owner = std::process::Command::new(std::env::current_exe()?)
        .arg("--owner")
        .arg(&case)
        .stdout(std::process::Stdio::null())
        .stderr(std::fs::File::create(case.join("owner.log"))?)
        .spawn()?;
    let handles = track(&case.join("workspace/tree-pids.json")).await?;
    owner.kill()?;
    let _ = owner.wait()?;
    exited(&handles)?;
    println!(
        "{}",
        serde_json::json!({"passed":true,"exit":exit,"restricted_token":true,"outside_write_denied":true,"unicode_input":true,"line_editing":true,"resize":"120x35","ctrl_c":true,"cancel_tree":true,"timeout_tree":true,"owner_crash_tree":true,"output":String::from_utf8_lossy(&output)})
    );
    Ok(())
}
