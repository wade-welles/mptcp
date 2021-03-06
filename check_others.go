// +build !linux

package mptcp

// checkMPTCP is not currently implemented on non-Linux platforms.
var checkMPTCP = func(host string, port uint16) (bool, error) {
	return false, ErrNotImplemented
}

// mptcpEnabled always returns false unless explicitly supported by a platform.
var mptcpEnabled = func() (bool, error) {
	return false, nil
}
