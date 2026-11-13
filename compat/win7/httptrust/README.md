# Win7 HTTPS trust

The Win7 build embeds the [2026-11-05 Mozilla CA extract](https://curl.se/ca/cacert-2026-11-05.pem) published by curl.
The download is checked against curl's published SHA-256 and pinned with its MPL 2.0 license in `sources.lock.json`.

On Windows 7 / Server 2008 R2, the process reads the current-user and local-machine ROOT and Disallowed stores without modifying them.
The bundled public roots and locally trusted roots form an explicit Go certificate pool.
Explicitly distrusted certificates are excluded from roots and rejected throughout peer and verified chains.
HTTPS and the Feishu SDK's default WebSocket dialer use this pool and retain certificate-chain, hostname, validity, and existing callback checks.
Other Windows versions retain their native behavior.

This avoids the old system verifier's online root-discovery delays and missing public authorities.
The reproduced clean Win7 failures were TLS handshake timeouts and Feishu's `x509: certificate signed by unknown authority`.
The OS still needs its Go-runtime prerequisite updates, including KB3063858 followed by a restart.

To update roots, download a dated curl CA extract and its published digest, review the source, and update the pinned file hashes.
Keep the downloaded PEM unchanged and retain its license.
Rebuild both CSGClaw and the pinned Feishu CLI.
The installer refreshes a valid same-version cached CLI when the shipped package manifest changes, preserving separate authorization data.

Verification covers private authorities, invalid hostnames, expired certificates, unknown authorities, explicit distrust, HTTPS, and WebSockets.
Opt-in native API tests additionally exercise the real MCP marketplace, Feishu's verified WebSocket TLS, and authorization/cancel/retry without granting account access.
