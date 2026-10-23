#!/usr/bin/env python3
"""Run the full Rust suite without desktop-app state leaking into fixtures."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

root = Path(__file__).resolve().parents[2]
env = os.environ.copy()
original_home = Path.home()
just = shutil.which("just")
if not just:
    raise SystemExit("Install just before running the Rust tests")
for name in (
    "CODEX_APPLY_PATCH_PRESERVE_LINE_ENDINGS", "CODEX_APP_TOOLS_PIPE_PATH",
    "CODEX_CI", "CODEX_CLI_PATH", "CODEX_INTERNAL_ORIGINATOR_OVERRIDE",
    "CODEX_MCP_NODE_PATH", "CODEX_PERMISSION_PROFILE", "CODEX_VERSION",
    "CODEX_SAGE_BACKFILL_TRACKER_TAB_REUSE", "CODEX_SESSION_ID",
    "CODEX_TASK_WORKSPACE_VERIFYING_IDENTITY", "CODEX_TECTONIC_PATH", "CODEX_THREAD_ID",
):
    env.pop(name, None)
env.setdefault("CARGO_HOME", str(original_home / ".cargo"))
env.setdefault("RUSTUP_HOME", str(original_home / ".rustup"))
for name in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
    env.pop(name, None)
env["NO_PROXY"] = env["no_proxy"] = "127.0.0.1,localhost,::1"
paths = ["/usr/bin", "/bin", "/usr/sbin", "/sbin"]
if node_bin := env.get("CODEX_WIN7_TEST_NODE_BIN"):
    paths.insert(0, node_bin)
env["PATH"] = os.pathsep.join(paths + [env["PATH"]])
original_tmp = str(Path(tempfile.gettempdir()).resolve())
with tempfile.TemporaryDirectory(prefix="cw7-", dir="/private/var/tmp") as temporary:
    sandbox = Path(temporary)
    for name in ("home", "tmp", "codex-home"):
        (sandbox / name).mkdir()
    test_bin = sandbox / "bin"
    test_bin.mkdir()
    (test_bin / "python3").symlink_to(Path(sys.executable).resolve())
    env["PATH"] = str(test_bin) + os.pathsep + env["PATH"]
    env["HOME"] = str(sandbox / "home")
    env["TMPDIR"] = original_tmp
    env["CODEX_HOME"] = str(sandbox / "codex-home")
    result = subprocess.run(
        [just, "test", "--features", "codex-v8-poc/sandbox", *sys.argv[1:]], cwd=root, env=env,
    )
raise SystemExit(result.returncode)
