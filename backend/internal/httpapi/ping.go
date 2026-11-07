package httpapi

import (
	"context"
	"net"
	"net/url"
	"time"

	"veylix/backend/internal/provisioning"
)

// DialPinger measures a TCP connect to the panel host.
//
// It is deliberately a plain TCP dial rather than ICMP: ICMP needs raw sockets
// (root), and a TCP handshake on the panel's own port is the number that
// actually decides whether the panel will answer. The result is therefore a
// *server reachability* measurement, which is why `PingResult.Scope` labels it —
// it is never the user's own connection quality (D-026).
//
// `lookup` resolves a server id to its panel URL, so the pinger does not need
// the whole integrations service.
func DialPinger(lookup func(ctx context.Context, serverID int64) (string, error), timeout time.Duration) provisioning.PanelPinger {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return func(ctx context.Context, serverID int64) (bool, int64) {
		raw, err := lookup(ctx, serverID)
		if err != nil {
			return false, -1
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return false, -1
		}
		host := u.Host
		if _, _, splitErr := net.SplitHostPort(host); splitErr != nil {
			// No explicit port: fall back to the scheme's default.
			if u.Scheme == "https" {
				host = net.JoinHostPort(host, "443")
			} else {
				host = net.JoinHostPort(host, "80")
			}
		}
		dctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		start := time.Now()
		conn, err := (&net.Dialer{}).DialContext(dctx, "tcp", host)
		if err != nil {
			return false, -1
		}
		_ = conn.Close()
		return true, time.Since(start).Milliseconds()
	}
}
