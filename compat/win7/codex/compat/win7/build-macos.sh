#!/bin/sh
set -eu
export PATH="/opt/homebrew/bin:$PATH"
FORK_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
WIN7_BUILD_DIR="$FORK_DIR/win7-build"
XWIN_CACHE=${XWIN_CACHE:-"$HOME/Library/Caches/cargo-xwin/xwin"}
CARGO_XWIN=${CARGO_XWIN:-cargo-xwin}
WIN7_PROFILE=${WIN7_PROFILE:-release}
mkdir -p "$WIN7_BUILD_DIR/bin"
python3 "$FORK_DIR/compat/win7/build-winpty.py"
LLVM_DIR=$(/opt/homebrew/bin/brew --prefix llvm)
LLD_DIR=$(/opt/homebrew/bin/brew --prefix lld)
"$LLVM_DIR/bin/clang-cl" --target=x86_64-pc-windows-msvc /c /O2 /W4 /WX \
 "/imsvc$XWIN_CACHE/crt/include" "/imsvc$XWIN_CACHE/sdk/include/ucrt" "/imsvc$XWIN_CACHE/sdk/include/um" "/imsvc$XWIN_CACHE/sdk/include/shared" \
 "/Fo$WIN7_BUILD_DIR/imports.obj" -- "$FORK_DIR/compat/win7/imports.c"
cat > "$WIN7_BUILD_DIR/bin/lld-link" <<WRAPPER
#!/bin/sh
if [ "\${1:-}" = "-flavor" ]; then shift 2; fi
exec "$LLD_DIR/bin/lld-link" -flavor link "$WIN7_BUILD_DIR/imports.obj" "\$@"
WRAPPER
cat > "$WIN7_BUILD_DIR/host-clang" <<WRAPPER
#!/bin/sh
exec "$LLVM_DIR/bin/clang" --ld-path="$LLD_DIR/bin/ld64.lld" "\$@"
WRAPPER
chmod +x "$WIN7_BUILD_DIR/bin/lld-link" "$WIN7_BUILD_DIR/host-clang"
export PATH="$WIN7_BUILD_DIR/bin:$LLVM_DIR/bin:$LLD_DIR/bin:/opt/homebrew/bin:$PATH"
export RUSTUP_TOOLCHAIN=1.95.0 RUSTC_BOOTSTRAP=1 CARGO_BUILD_JOBS=6
export CARGO_TARGET_AARCH64_APPLE_DARWIN_LINKER="$WIN7_BUILD_DIR/host-clang"
cd "$FORK_DIR/codex-rs"
if [ -z "${LIB:-}" ]; then
    for WIN7_IMPORT_LIB in "${CARGO_HOME:-$HOME/.cargo}"/registry/src/*/windows_x86_64_msvc-0.48.5/lib; do
        if [ -d "$WIN7_IMPORT_LIB" ]; then export LIB="$WIN7_IMPORT_LIB"; break; fi
    done
fi
"$CARGO_XWIN" rustc --target x86_64-win7-windows-msvc -Z build-std=std,panic_abort \
 -p codex-cli --bin codex --profile "$WIN7_PROFILE" --config profile.dev.debug=0 --config profile.dev.strip='"none"' \
 -- -C link-arg=/INCREMENTAL:NO
WIN7_TARGET_DIR=${CARGO_TARGET_DIR:-"$FORK_DIR/codex-rs/target"}
WIN7_OUTPUT_PROFILE=$WIN7_PROFILE
if [ "$WIN7_OUTPUT_PROFILE" = dev ]; then WIN7_OUTPUT_PROFILE=debug; fi
WIN7_OUTPUT_DIR="$WIN7_TARGET_DIR/x86_64-win7-windows-msvc/$WIN7_OUTPUT_PROFILE"
cp "$WIN7_BUILD_DIR/winpty/winpty.dll" "$WIN7_BUILD_DIR/winpty/winpty-agent.exe" "$WIN7_BUILD_DIR/winpty/LICENSE.winpty" "$WIN7_OUTPUT_DIR/"
