package panel

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Target is a validated panel endpoint: scheme, host:port and the web base path.
type Target struct {
	BaseURL string // e.g. https://panel.example.com:2053/QpKFsYpDlLFdRsOoW7
	Host    string // hostname only (no port)

	// AllowPrivate is the per-integration private-network policy this target was
	// validated under. It is carried forward because the dial-time guard (B-4)
	// must apply the SAME policy the registration-time check used — otherwise a
	// self-hosted panel on a private network would be refused on every call, or a
	// public one would silently become exempt.
	AllowPrivate bool
}

// SSRFError explains why a panel URL was refused. Messages are safe to return
// to an authenticated Super Admin, and never echo credentials.
type SSRFError struct {
	Reason string
}

func (e *SSRFError) Error() string { return "panel: url rejected: " + e.Reason }

func reject(reason string) error { return &SSRFError{Reason: reason} }

// ParseTarget validates a Super-Admin supplied panel URL (Bible §8, REQUIREMENTS §8).
//
// Rules:
//   - https only — panels must not be managed over cleartext;
//   - no userinfo (credentials embedded in a URL would leak into logs);
//   - host must resolve, and every resolved address must be public
//     unless allowPrivate is explicitly enabled for this integration;
//   - the query string is refused (base paths are path segments only).
//
// allowPrivate is the documented per-integration policy: panel endpoints may
// legitimately sit on a private network, but that is an explicit opt-in,
// never a default (REQUIREMENTS §8, Bible §16).
func ParseTarget(raw string, allowPrivate bool, resolve func(host string) ([]net.IP, error)) (*Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, reject("empty url")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, reject("not a valid URL")
	}

	if !strings.EqualFold(u.Scheme, "https") {
		return nil, reject("scheme must be https")
	}
	if u.User != nil {
		return nil, reject("credentials must not be embedded in the URL")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, reject("query string and fragment are not allowed")
	}

	host := u.Hostname()
	if host == "" {
		return nil, reject("missing host")
	}
	// An IP literal bypasses DNS, so validate it directly.
	if ip := net.ParseIP(host); ip != nil {
		if err := checkIP(ip, allowPrivate); err != nil {
			return nil, err
		}
	} else {
		if resolve == nil {
			return nil, errors.New("panel: resolver required")
		}
		ips, err := resolve(host)
		if err != nil {
			return nil, fmt.Errorf("panel: cannot resolve host: %w", err)
		}
		if len(ips) == 0 {
			return nil, reject("host did not resolve")
		}
		for _, ip := range ips {
			if err := checkIP(ip, allowPrivate); err != nil {
				return nil, err
			}
		}
	}

	base := strings.TrimRight(u.String(), "/")
	return &Target{BaseURL: base, Host: host, AllowPrivate: allowPrivate}, nil
}

// checkIP blocks loopback, private, link-local, multicast and unspecified
// destinations unless private networking has been explicitly allowed.
//
// The per-integration `allowPrivate` policy exists because a self-hosted panel
// legitimately sits on a private network or on loopback behind a tunnel
// (REQUIREMENTS §8). It therefore lifts the loopback, RFC1918 and link-local
// rules together — a policy that permitted `10.0.0.5` but refused `127.0.0.1`
// would refuse every panel reached through an SSH tunnel, which is a normal
// deployment.
//
// What it NEVER lifts: the cloud-metadata address. IMDS credentials are not panel
// configuration, and no deployment needs Veylix to fetch them.
func checkIP(ip net.IP, allowPrivate bool) error {
	if ip.IsUnspecified() {
		return reject("unspecified address")
	}
	// IMDS is checked FIRST and is never lifted by any policy; a later branch that
	// returned nil early would otherwise make it reachable.
	if isCloudMetadata(ip) {
		return reject("cloud metadata address")
	}
	if ip.IsLoopback() {
		if allowPrivate {
			return nil
		}
		return reject("loopback address (enable the private-network policy for this integration)")
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		if allowPrivate {
			return nil
		}
		return reject("link-local address")
	}
	if ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return reject("multicast address")
	}
	if ip.IsPrivate() && !allowPrivate {
		return reject("private address (enable the private-network policy for this integration)")
	}
	return nil
}

// isCloudMetadata reports addresses used by cloud metadata services.
//
// Every one of these must be listed EXPLICITLY, even when another rule would also
// refuse it. `169.254.169.254` is link-local, and the link-local rule is lifted by
// the private-network policy — so relying on "link-local covers it" made IMDS
// reachable for exactly the integrations whose policy was relaxed. Found by
// `TestDialGuardRefusesCloudMetadataEvenWithThePrivatePolicy`.
func isCloudMetadata(ip net.IP) bool {
	for _, m := range metadataAddresses {
		if ip.Equal(m) {
			return true
		}
	}
	return false
}

var metadataAddresses = []net.IP{
	net.ParseIP("169.254.169.254"), // AWS / GCP / Azure / DigitalOcean IMDS
	net.ParseIP("169.254.170.2"),   // AWS ECS task metadata
	net.ParseIP("100.100.100.200"), // Alibaba Cloud
	net.ParseIP("fd00:ec2::254"),   // AWS IMDS over IPv6
}
