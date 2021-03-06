// +build linux

package mptcp

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

const (
	// procMPTCP is the location of the Linux-specific file which contains
	// the active MPTCP connections table.
	procMPTCP = "/proc/net/mptcp"

	// mptcpTableColumns is the number of columns in a valid Linux MPTCP
	// connections table.
	mptcpTableColumns = 10
)

var (
	// mptcpTableHeader is the header from the top of a MPTCP connections table.
	mptcpTableHeader = []byte(`  sl  loc_tok  rem_tok  v6 local_address                         remote_address                        st ns tx_queue rx_queue inode`)
)

var (
	// errInvalidMPTCPEntry is returned when an input MPTCP connection
	// entry is not in the expected format.
	errInvalidMPTCPEntry = errors.New("invalid MPTCP connection entry")

	// errInvalidMPTCPTable is returned when an input MPTCP connection
	// table is not in the expected format.
	errInvalidMPTCPTable = errors.New("invalid MPTCP connections table")
)

// checkMPTCP checks if an input host string and uint16 port are present
// in this Linux machine's MPTCP active connections.
var checkMPTCP = func(host string, port uint16) (bool, error) {
	// Get hex representation of host
	hexHost, err := hostToHex(host)
	if err != nil {
		return false, err
	}

	// Combine hex host and port, convert to uppercase
	hexHostPort := strings.ToUpper(net.JoinHostPort(hexHost, u16PortToHex(port)))

	// Use lookup function to check for results
	return lookupMPTCPLinux(hexHostPort)
}

// mptcpEnabled uses the Linux /proc filesystem to determine if
// the current host supports MPTCP.
var mptcpEnabled = func() (bool, error) {
	// Check for presence of MPTCP connections table
	_, err := os.Stat(procMPTCP)
	if err == nil {
		// MPTCP capable
		return true, nil
	}

	// If table does not exist, return false, but do not return
	// the accompanying error
	if os.IsNotExist(err) {
		return false, nil
	}

	// Return any other error
	return false, err
}

// hostToHex converts an input host IP address into its equivalent hex form,
// for use with MPTCP connection lookup.
func hostToHex(host string) (string, error) {
	// Parse IP address from host
	ip := net.ParseIP(host)

	// If result is not nil, we assume this is IPv4
	if ip4 := ip.To4(); ip4 != nil && len(ip4) == net.IPv4len {
		// For IPv4, grab the IPv4 hex representation of the address
		return fmt.Sprintf("%02x%02x%02x%02x", ip4[3], ip4[2], ip4[1], ip4[0]), nil
	}

	// Check for IPv6 address
	if ip6 := ip.To16(); ip6 != nil && len(ip6) == net.IPv6len {
		// TODO(mdlayher): attempt to check for IPv6 address
		return "", ErrIPv6NotImplemented
	}

	// IP address is not valid
	return "", ErrInvalidIPAddress
}

// u16PortToHex converts an input uint16 port into its equivalent hex form,
// for use with MPTCP connection lookup.
func u16PortToHex(port uint16) string {
	// Store uint16 in buffer using little endian byte order
	portBuf := [2]byte{}
	binary.LittleEndian.PutUint16(portBuf[:], port)

	// Retrieve hex representation of uint16 port
	return fmt.Sprintf("%02x%02x", portBuf[1], portBuf[0])
}

// lookupMPTCPLinux uses the Linux /proc filesystem to attempt to detect
// active MPTCP connections matching the input hex host:port pair.
//
// This implementation is swappable for testing with a mock data source.
var lookupMPTCPLinux = func(hexHostPort string) (bool, error) {
	// Open Linux MPTCP table
	mptcpFile, err := os.Open(procMPTCP)
	if err != nil {
		return false, err
	}
	defer mptcpFile.Close()

	// Read from input stream
	return mptcpTableReaderLinux(mptcpFile, hexHostPort)
}

// mptcpTableReaderLinux reads a MPTCP connections table from an input stream.
// This function allows easier testability with table parsing.
func mptcpTableReaderLinux(r io.Reader, hexHostPort string) (bool, error) {
	// Open text scanner to split lines, skip header line
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	if !scanner.Scan() {
		// If file was empty, return unexpected EOF
		return false, io.ErrUnexpectedEOF
	}

	// Ensure first line was valid MPTCP connections table header
	if !bytes.Equal(scanner.Bytes(), mptcpTableHeader) {
		return false, errInvalidMPTCPTable
	}

	// Iterate until EOF or entry found
	for scanner.Scan() {
		// Scan fields into mptcpTableEntry
		mptcpEntry, err := newMPTCPTableEntry(strings.Fields(scanner.Text()))
		if err != nil {
			return false, err
		}

		// Check for remote address which matches input
		if mptcpEntry.RemoteAddr == hexHostPort {
			return true, nil
		}
	}

	// No result found
	return false, nil
}

// mptcpTableEntry contains parsed information from a Linux MPTCP connections
// table entry.  While numerous fields are available, we only make use of
// a couple of them.
type mptcpTableEntry struct {
	IsIPv6     bool
	RemoteAddr string
}

// newMPTCPTableEntry creates a new mptcpTableEntry from a slice of strings.
func newMPTCPTableEntry(fields []string) (*mptcpTableEntry, error) {
	// Check for proper number of fields, though most of them will not be
	// kept for this library's purposes.
	if len(fields) != mptcpTableColumns {
		return nil, errInvalidMPTCPEntry
	}

	// Check for IPv6 connectivity
	m := &mptcpTableEntry{}
	if fields[3] == "1" {
		m.IsIPv6 = true
	}

	// Scan hex encoded remote address
	m.RemoteAddr = fields[5]

	return m, nil
}
