package main

import "testing"

func TestAddrPort(t *testing.T) {
	t.Parallel()
	tests := map[string]int{
		"127.0.0.1:8080":  8080,
		"localhost:19009": 19009,
		":3000":           3000,
		"[::1]:8443":      8443,
	}
	for addr, want := range tests {
		got, err := addrPort(addr)
		if err != nil {
			t.Fatalf("addrPort(%q): %v", addr, err)
		}
		if got != want {
			t.Fatalf("addrPort(%q) = %d, want %d", addr, got, want)
		}
	}
	for _, addr := range []string{"localhost", "127.0.0.1:0", "127.0.0.1:70000", "127.0.0.1:abc"} {
		if _, err := addrPort(addr); err == nil {
			t.Fatalf("addrPort(%q) should fail", addr)
		}
	}
}
