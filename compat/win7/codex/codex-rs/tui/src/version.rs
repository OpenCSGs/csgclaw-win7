/// The current Codex CLI version as embedded at compile time.
#[cfg(not(test))]
pub const CODEX_CLI_VERSION: &str = env!("CARGO_PKG_VERSION");

// Snapshot layouts already redact the version text. Freeze its width before
// rendering too, so release-tag builds keep the same layout as source builds.
#[cfg(test)]
pub const CODEX_CLI_VERSION: &str = "0.0.0";
