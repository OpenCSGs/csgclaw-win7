#!/usr/bin/env python3
"""Build the pinned WinPTY console backend with the restricted-token extension."""
import ast
import concurrent.futures
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import urllib.request

if sys.version_info < (3, 12):
    raise SystemExit("WinPTY build requires Python 3.12 or later")

HERE = Path(__file__).resolve().parent
BUILD = HERE.parents[1] / "win7-build" / "winpty"
COMMIT = "3e1ab962d5262dd76159870c6dc0724927ca6a9d"
ARCHIVE_SHA256 = "9451dc39727d77156b6827358c7e4cd7385465f2b8a17b61c251ce80837b494f"
BUILD.mkdir(parents=True, exist_ok=True)
archive = BUILD / "source.tar.gz"
if not archive.exists():
    urllib.request.urlretrieve(
        f"https://api.github.com/repos/rprichard/winpty/tarball/{COMMIT}", archive
    )
if hashlib.file_digest(archive.open("rb"), "sha256").hexdigest() != ARCHIVE_SHA256:
    raise SystemExit("WinPTY source checksum mismatch")
source = BUILD / "source"
if source.exists():
    shutil.rmtree(source)
source.mkdir()
with tarfile.open(archive) as stream:
    for member in stream.getmembers():
        member.name = "/".join(member.name.split("/")[1:])
        if member.name:
            stream.extract(member, source, filter="data")
subprocess.run(["patch", "-p1", "-i", str(HERE / "winpty-sandbox.patch")], cwd=source, check=True)
# Use the upstream generator rule rather than editing its generated header.
(source / "config.mk").touch()
subprocess.run(["make", f"COMMIT_HASH={COMMIT}", "build/gen/GenVersion.h"], cwd=source, check=True)
sdk = Path(os.environ.get("XWIN_CACHE", Path.home() / "Library/Caches/cargo-xwin/xwin"))
compiler = "/opt/homebrew/opt/llvm/bin/clang-cl"
linker = "/opt/homebrew/opt/lld/bin/lld-link"
includes = [sdk / "crt/include", sdk / "sdk/include/ucrt", sdk / "sdk/include/um", sdk / "sdk/include/shared"]
targets = ast.literal_eval((source / "src/winpty.gyp").read_text())["targets"]
for target in targets:
    name = target["target_name"]
    if name not in {"winpty", "winpty-agent"}:
        continue
    files = [source / "src" / path for path in target["sources"] if path.endswith(".cc")]
    objects = BUILD / "obj" / name
    objects.mkdir(parents=True, exist_ok=True)
    defines = ["UNICODE", "_UNICODE", "NOMINMAX", "_WIN32_WINNT=0x0601", "_CRT_SECURE_NO_WARNINGS"] + target.get("defines", [])

    def compile_file(path):
        obj = objects / (path.stem + ".obj")
        args = [compiler, "--target=x86_64-pc-windows-msvc", "/nologo", "/c", "/O2", "/EHsc", "/MT", "/std:c++14", "/W3"]
        args += [f"/D{value}" for value in defines]
        args += [f"/imsvc{value}" for value in includes]
        args += [f"/I{source / 'src/include'}", f"/I{source / 'build/gen'}", f"/Fo{obj}", "--", str(path)]
        subprocess.run(args, check=True)
        return obj

    print(f"Building {name}", flush=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
        compiled = list(pool.map(compile_file, files))
    output = BUILD / (name + (".dll" if name == "winpty" else ".exe"))
    args = [linker, "/nologo", "/machine:x64", "/subsystem:console,6.01", "/timestamp:0", f"/out:{output}"]
    if name == "winpty":
        args += ["/dll"]
    args += [str(path) for path in compiled]
    args += [f"/libpath:{path}" for path in [sdk / "crt/lib/x86_64", sdk / "sdk/lib/ucrt/x86_64", sdk / "sdk/lib/um/x86_64"]]
    args += ["kernel32.lib", "advapi32.lib", "user32.lib", "shell32.lib"]
    subprocess.run(args, check=True)
shutil.copy2(source / "LICENSE", BUILD / "LICENSE.winpty")
print(f"WinPTY outputs: {BUILD}")
