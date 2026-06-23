package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeAddress(t *testing.T) {
	tests := []struct {
		kind     string
		input    int
		expected uint16
	}{
		{"holding", 40001, 0},
		{"holding", 0, 0},
		{"input", 30010, 9},
		{"discrete", 10001, 0},
		{"coil", 1, 0},
		{"coil", 25, 24},
	}
	for _, test := range tests {
		got, err := normalizeAddress(test.kind, test.input)
		if err != nil {
			t.Fatalf("normalizeAddress(%q, %d): %v", test.kind, test.input, err)
		}
		if got != test.expected {
			t.Errorf("normalizeAddress(%q, %d) = %d, want %d", test.kind, test.input, got, test.expected)
		}
	}
}

func TestNormalizeAddressRejectsInvalidType(t *testing.T) {
	if _, err := normalizeAddress("invalid", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadMQTTConfigUsesSafeIntervals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mqtt")
	data := []byte("config mqtt 'main'\n" +
		" option enabled '1'\n option port '1883'\n" +
		" option keepalive '0'\n option publish_interval '0'\n option system_interval '0'\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadMQTTConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.KeepAlive != 60*time.Second || config.PublishInterval != 5*time.Second || config.SystemInterval != 600*time.Second {
		t.Fatalf("unsafe defaults: %#v", config)
	}
}
