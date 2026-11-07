package auth

import "errors"

// Shared sentinel errors. Transport maps these to HTTP codes; messages are
// always generic so they cannot be used to probe the system.
var (
	// ErrUnauthenticated → 401 (no/expired session).
	ErrUnauthenticated = errors.New("auth: not authenticated")
	// ErrForbidden → 403 (authenticated but not permitted).
	ErrForbidden = errors.New("auth: forbidden")
	// ErrNotFound → 404 for a resource the caller may not see: a 403 on an
	// object id would confirm that the object exists (IDOR leak).
	ErrNotFound = errors.New("auth: resource not found")
)
