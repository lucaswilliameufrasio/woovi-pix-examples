package main

import "testing"

func TestLoopbackAddress(t *testing.T) {
	tests := []struct {
		address string
		valid   bool
	}{{"127.0.0.1:8080", true}, {"[::1]:8080", true}, {"0.0.0.0:8080", false}, {":8080", false}, {"example.com:8080", false}, {"127.0.0.1", false}}
	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			if got := loopbackAddress(tt.address); got != tt.valid {
				t.Fatalf("loopbackAddress(%q)=%v want %v", tt.address, got, tt.valid)
			}
		})
	}
}
