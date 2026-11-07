// Package panel will contain the 3X-UI HTTP adapter.
//
// Stage B ships the boundary only. The contract it must implement is verified
// in docs/3XUI_API_CONTRACT.md (release v3.10.2); every endpoint is listed there
// with its method, path and response shape, and nothing outside that list may
// be called.
//
// Planned contents (Stage E3):
//   - Bearer-token client over one reused http.Transport, per-request context
//     timeouts, bounded retries on idempotent GETs only
//   - SSRF guard: https-only, DNS resolution + private/link-local/metadata
//     blocking, redirect re-validation, rebinding protection
//   - capability detection: GET /panel/api/server/status + openapi.json
//   - status mapping: 401/403 -> REAUTH_REQUIRED, transport/5xx ->
//     CONNECTION_UNAVAILABLE; never maps an outage to REVOKED
package panel

// Version is the adapter contract version, bumped when the endpoint set changes.
const Version = 1
