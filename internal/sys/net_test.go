package sys

import "testing"

func TestIPv4NetworkOrder(t *testing.T) {
	// 127.0.0.1 stored in the TCP table has network byte order in memory;
	// read as uint32 on little-endian it becomes 0x0100007F.
	if got := ipv4(0x0100007F); got != "127.0.0.1" {
		t.Fatalf("ipv4 order: got %s", got)
	}
	if got := ipv4(0x04030201); got != "1.2.3.4" {
		t.Fatalf("ipv4 order: got %s", got)
	}
}

func TestPortByteSwap(t *testing.T) {
	if got := ntohs(0x901F); got != 0x1F90 {
		t.Fatalf("ntohs: got 0x%X", got)
	}
	if got := ntohs(0x1F90); got != 0x901F {
		t.Fatalf("ntohs reverse: got 0x%X", got)
	}
}
