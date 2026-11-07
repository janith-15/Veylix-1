package panel

import (
	"net/url"
	"strings"
)

// ValidateEmail enforces the shape of a panel client email.
//
// Client emails double as remote identifiers and are placed in URL paths, so
// this is a security boundary, not cosmetic validation: dots alone are legal in
// a path segment, which means `..` would survive url.PathEscape and traverse
// (caught by TestClientEmailCannotInjectPath).
func ValidateEmail(email string) error {
	if email == "" {
		return errBadEmail("empty")
	}
	if len(email) > 190 {
		return errBadEmail("too long")
	}
	for _, r := range email {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '@', r == '+':
		default:
			return errBadEmail("character not allowed")
		}
	}
	if strings.Contains(email, "..") {
		return errBadEmail("consecutive dots are not allowed")
	}
	return nil
}

type badEmailError struct{ why string }

func (e *badEmailError) Error() string { return "panel: invalid client email: " + e.why }

func errBadEmail(why string) error { return &badEmailError{why: why} }

// emailPathSegment validates then escapes a client email for use in a path.
func emailPathSegment(email string) (string, error) {
	if err := ValidateEmail(email); err != nil {
		return "", err
	}
	return url.PathEscape(email), nil
}
