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

func TestMQTTBrokerURL(t *testing.T) {
	config := MQTTConfig{Broker: "broker.example", Port: 1883}
	if got := mqttBrokerURL(config); got != "tcp://broker.example:1883" {
		t.Fatalf("mqttBrokerURL() = %q", got)
	}
	config.Broker = "ssl://broker.example:8883"
	if got := mqttBrokerURL(config); got != config.Broker {
		t.Fatalf("mqttBrokerURL() changed explicit scheme: %q", got)
	}
}

func TestValidateMQTTTopic(t *testing.T) {
	if err := validateMQTTTopic("factory/device/test", false); err != nil {
		t.Fatal(err)
	}
	if err := validateMQTTTopic("factory/+/test", false); err == nil {
		t.Fatal("publish wildcard should be rejected")
	}
	if err := validateMQTTTopic("factory/+/test", true); err != nil {
		t.Fatal(err)
	}
}

func TestLoadModbusDeviceIncludesDisabledDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modbus")
	data := []byte("config device 'plc1'\n" +
		" option enabled '0'\n option name 'PLC Teste'\n option ip '192.0.2.10'\n" +
		" option port '1502'\n option slave_id '7'\n option timeout '30'\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	device, err := loadModbusDevice(path, "plc1")
	if err != nil {
		t.Fatal(err)
	}
	if device.Address != "192.0.2.10:1502" || device.SlaveID != 7 || device.Timeout != 10*time.Second {
		t.Fatalf("unexpected device: %#v", device)
	}
}
