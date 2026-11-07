package panel

import "net"

// defaultResolver is the production DNS resolver used by ParseTarget callers
// that operate on hostnames (and by the live integration test).
func defaultResolver(host string) ([]net.IP, error) {
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	return addrs, nil
}
