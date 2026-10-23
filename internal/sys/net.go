package sys

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"
)

// Endpoint is one TCP connection owned by a process.
type Endpoint struct {
	LocalIP    string
	LocalPort  uint16
	RemoteIP   string
	RemotePort uint16
	State      uint32
}

type mibTCPRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

const tcpTableOwnerPIDAll = 5

// TCPEndpoints reads the current IPv4 TCP table and keeps rows for pid.
func TCPEndpoints(pid uint32) ([]Endpoint, error) {
	var size uint32
	r, _, _ := pGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, tcpTableOwnerPIDAll, 0)
	if r != 122 || size == 0 { // ERROR_INSUFFICIENT_BUFFER
		return nil, fmt.Errorf("GetExtendedTcpTable size: %d", r)
	}
	buf := make([]byte, size)
	r, _, _ = pGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, 2, tcpTableOwnerPIDAll, 0)
	if r != 0 {
		return nil, fmt.Errorf("GetExtendedTcpTable: %d", r)
	}
	if len(buf) < 4 {
		return nil, nil
	}
	n := *(*uint32)(unsafe.Pointer(&buf[0]))
	if n == 0 {
		return nil, nil
	}
	rows := unsafe.Slice((*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[4])), int(n))
	out := make([]Endpoint, 0, 8)
	for _, row := range rows {
		if row.OwningPID != pid {
			continue
		}
		out = append(out, Endpoint{
			LocalIP:    ipv4(row.LocalAddr),
			LocalPort:  ntohs(uint16(row.LocalPort)),
			RemoteIP:   ipv4(row.RemoteAddr),
			RemotePort: ntohs(uint16(row.RemotePort)),
			State:      row.State,
		})
	}
	return out, nil
}

func ipv4(v uint32) string {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v) // restores network byte order
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
}

func ntohs(v uint16) uint16 { return v<<8 | v>>8 }

// DNSLookup measures a resolver round trip and returns the addresses.
func DNSLookup(host string, timeout time.Duration) (time.Duration, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	elapsed := time.Since(start)
	return elapsed, addrs, err
}

// TCPRTT measures a TCP handshake against addr. It is read-only: no payload
// is ever sent and the socket is closed immediately.
func TCPRTT(addr string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return 0, err
	}
	c.Close()
	return time.Since(start), nil
}

var _ = syscall.Getpid
