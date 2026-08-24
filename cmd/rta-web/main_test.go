package main

import "testing"

func TestIsLoopbackHost(t *testing.T) {
	for _, host := range []string{"", "localhost", "127.0.0.1", "::1", "127.0.0.8"} {
		if !isLoopbackHost(host) {
			t.Fatalf("%q should be loopback", host)
		}
	}
	for _, host := range []string{"0.0.0.0", "192.168.1.10", "example.internal"} {
		if isLoopbackHost(host) {
			t.Fatalf("%q should not be loopback", host)
		}
	}
}
