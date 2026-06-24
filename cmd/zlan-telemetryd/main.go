package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Wagnee/Tailscale-ZLAN9809M-Hybrid/internal/uci"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/goburrow/modbus"
)

const (
	modbusConfigPath = "/etc/config/zlan_modbus"
	mqttConfigPath   = "/etc/config/zlan_mqtt"
	statePath        = "/tmp/zlan-telemetry/modbus-state.json"
	statusPath       = "/tmp/zlan-telemetry/status.json"
)

type TagConfig struct {
	Name    string  `json:"name"`
	Kind    string  `json:"type"`
	Address uint16  `json:"address"`
	Scale   float64 `json:"scale"`
	Offset  float64 `json:"offset"`
}

type DeviceConfig struct {
	Key          string
	Name         string
	Address      string
	SlaveID      byte
	PollInterval time.Duration
	Timeout      time.Duration
	Tags         []TagConfig
}

type MQTTConfig struct {
	Enabled         bool
	Broker          string
	Port            int
	Username        string
	Password        string
	ClientID        string
	TopicPrefix     string
	KeepAlive       time.Duration
	PublishInterval time.Duration
	SystemInterval  time.Duration
	QoS             byte
	Retain          bool
}

type TagValue struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
	Quality   string  `json:"quality"`
	Error     string  `json:"error,omitempty"`
}

type DeviceState struct {
	Name     string     `json:"name"`
	Address  string     `json:"address"`
	Status   string     `json:"status"`
	LastRead int64      `json:"last_read"`
	Values   []TagValue `json:"values"`
}

type StateDocument struct {
	UpdatedAt int64                  `json:"updated_at"`
	Devices   map[string]DeviceState `json:"devices"`
}

type RuntimeStatus struct {
	MQTTEnabled    bool   `json:"mqtt_enabled"`
	MQTTConnected  bool   `json:"mqtt_connected"`
	Broker         string `json:"broker"`
	LastMQTTError  string `json:"last_mqtt_error,omitempty"`
	Published      uint64 `json:"published"`
	LastPublish    int64  `json:"last_publish"`
	ModbusDevices  int    `json:"modbus_devices"`
	LastStateWrite int64  `json:"last_state_write"`
}

var state = struct {
	sync.RWMutex
	doc StateDocument
}{doc: StateDocument{Devices: make(map[string]DeviceState)}}

var runtime = struct {
	sync.RWMutex
	status RuntimeStatus
}{}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if handled, err := runCommand(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERRO: %v\n", err)
			os.Exit(1)
		}
		return
	}
	devices, err := loadModbusConfig(modbusConfigPath)
	if err != nil {
		log.Fatalf("configuração Modbus inválida: %v", err)
	}
	mqttConfig, err := loadMQTTConfig(mqttConfigPath)
	if err != nil {
		log.Fatalf("configuração MQTT inválida: %v", err)
	}
	if len(devices) == 0 && !mqttConfig.Enabled {
		log.Println("nenhum dispositivo Modbus ou MQTT habilitado")
		return
	}

	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		log.Fatal(err)
	}
	runtime.status.MQTTEnabled = mqttConfig.Enabled
	runtime.status.Broker = mqttConfig.Broker
	runtime.status.ModbusDevices = len(devices)
	writeRuntimeStatus()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	for _, device := range devices {
		device := device
		wg.Add(1)
		go func() {
			defer wg.Done()
			pollDevice(ctx, device)
		}()
	}

	if mqttConfig.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runMQTT(ctx, mqttConfig)
		}()
	}

	<-ctx.Done()
	wg.Wait()
	log.Println("zlan-telemetryd encerrado")
}

func loadModbusConfig(path string) ([]DeviceConfig, error) {
	sections, err := uci.Parse(path)
	if err != nil {
		return nil, err
	}
	tagsByDevice := make(map[string][]TagConfig)
	for _, section := range sections {
		if section.Type != "tag" || !enabled(section.Options, true) {
			continue
		}
		deviceKey := section.Options["device"]
		address, err := strconv.Atoi(option(section.Options, "address", "0"))
		if err != nil || address < 0 || address > 65535 {
			return nil, fmt.Errorf("tag %s: address inválido", section.Name)
		}
		kind := option(section.Options, "type", "holding")
		normalized, err := normalizeAddress(kind, address)
		if err != nil {
			return nil, fmt.Errorf("tag %s: %w", section.Name, err)
		}
		tagsByDevice[deviceKey] = append(tagsByDevice[deviceKey], TagConfig{
			Name:    option(section.Options, "name", section.Name),
			Kind:    kind,
			Address: normalized,
			Scale:   floatOption(section.Options, "scale", 1),
			Offset:  floatOption(section.Options, "offset", 0),
		})
	}

	var devices []DeviceConfig
	for _, section := range sections {
		if section.Type != "device" || !enabled(section.Options, false) {
			continue
		}
		ip := section.Options["ip"]
		port := intOption(section.Options, "port", 502)
		if ip == "" || port < 1 || port > 65535 {
			return nil, fmt.Errorf("device %s: IP/porta inválidos", section.Name)
		}
		slave := intOption(section.Options, "slave_id", 1)
		if slave < 0 || slave > 247 {
			return nil, fmt.Errorf("device %s: slave_id inválido", section.Name)
		}
		pollSeconds := intOption(section.Options, "poll_interval", 30)
		timeoutSeconds := intOption(section.Options, "timeout", 5)
		if pollSeconds < 1 {
			pollSeconds = 1
		}
		if timeoutSeconds < 1 {
			timeoutSeconds = 1
		}
		devices = append(devices, DeviceConfig{
			Key:          section.Name,
			Name:         option(section.Options, "name", section.Name),
			Address:      fmt.Sprintf("%s:%d", ip, port),
			SlaveID:      byte(slave),
			PollInterval: time.Duration(pollSeconds) * time.Second,
			Timeout:      time.Duration(timeoutSeconds) * time.Second,
			Tags:         tagsByDevice[section.Name],
		})
	}
	return devices, nil
}

func loadMQTTConfig(path string) (MQTTConfig, error) {
	sections, err := uci.Parse(path)
	if err != nil {
		return MQTTConfig{}, err
	}
	for _, section := range sections {
		if section.Type != "mqtt" {
			continue
		}
		keepAlive := intOption(section.Options, "keepalive", 60)
		publishInterval := intOption(section.Options, "publish_interval", 5)
		systemInterval := intOption(section.Options, "system_interval", 600)
		port := intOption(section.Options, "port", 1883)
		qos := intOption(section.Options, "qos", 0)
		if keepAlive < 1 {
			keepAlive = 60
		}
		if publishInterval < 1 {
			publishInterval = 5
		}
		if systemInterval < 1 {
			systemInterval = 600
		}
		if port < 1 || port > 65535 {
			return MQTTConfig{}, fmt.Errorf("porta MQTT invalida: %d", port)
		}
		if qos < 0 || qos > 2 {
			qos = 0
		}
		return MQTTConfig{
			Enabled:         enabled(section.Options, false),
			Broker:          option(section.Options, "broker", "mqtt.eclipseprojects.io"),
			Port:            port,
			Username:        section.Options["username"],
			Password:        section.Options["password"],
			ClientID:        option(section.Options, "client_id", "zlan9809m"),
			TopicPrefix:     option(section.Options, "topic_prefix", "zlan9809m"),
			KeepAlive:       time.Duration(keepAlive) * time.Second,
			PublishInterval: time.Duration(publishInterval) * time.Second,
			SystemInterval:  time.Duration(systemInterval) * time.Second,
			QoS:             byte(qos),
			Retain:          option(section.Options, "retain", "0") == "1",
		}, nil
	}
	return MQTTConfig{}, errors.New("seção config mqtt não encontrada")
}

func pollDevice(ctx context.Context, device DeviceConfig) {
	pollOnce(device)
	ticker := time.NewTicker(device.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOnce(device)
		}
	}
}

func pollOnce(device DeviceConfig) {
	handler := modbus.NewTCPClientHandler(device.Address)
	handler.Timeout = device.Timeout
	handler.SlaveId = device.SlaveID
	values := make([]TagValue, 0, len(device.Tags))
	status := "connected"

	if err := handler.Connect(); err != nil {
		status = "error"
		for _, tag := range device.Tags {
			values = append(values, failedValue(tag.Name, err))
		}
	} else {
		client := modbus.NewClient(handler)
		for _, tag := range device.Tags {
			raw, err := readTag(client, tag)
			if err != nil {
				values = append(values, failedValue(tag.Name, err))
				continue
			}
			values = append(values, TagValue{
				Name: tag.Name, Value: float64(raw)*tag.Scale + tag.Offset,
				Timestamp: time.Now().Unix(), Quality: "good",
			})
		}
	}
	_ = handler.Close()

	state.Lock()
	state.doc.UpdatedAt = time.Now().Unix()
	state.doc.Devices[device.Key] = DeviceState{
		Name: device.Name, Address: device.Address, Status: status,
		LastRead: time.Now().Unix(), Values: values,
	}
	document := cloneStateLocked()
	state.Unlock()
	if err := writeJSONAtomic(statePath, document, 0644); err != nil {
		log.Printf("erro salvando estado Modbus: %v", err)
	} else {
		runtime.Lock()
		runtime.status.LastStateWrite = time.Now().Unix()
		runtime.Unlock()
		writeRuntimeStatus()
	}
}

func failedValue(name string, err error) TagValue {
	return TagValue{Name: name, Timestamp: time.Now().Unix(), Quality: "bad", Error: err.Error()}
}

func readTag(client modbus.Client, tag TagConfig) (uint16, error) {
	var data []byte
	var err error
	switch tag.Kind {
	case "coil":
		data, err = client.ReadCoils(tag.Address, 1)
	case "discrete":
		data, err = client.ReadDiscreteInputs(tag.Address, 1)
	case "holding":
		data, err = client.ReadHoldingRegisters(tag.Address, 1)
	case "input":
		data, err = client.ReadInputRegisters(tag.Address, 1)
	default:
		return 0, fmt.Errorf("tipo desconhecido: %s", tag.Kind)
	}
	if err != nil {
		return 0, err
	}
	if tag.Kind == "coil" || tag.Kind == "discrete" {
		if len(data) < 1 {
			return 0, errors.New("resposta Modbus vazia")
		}
		if data[0]&1 == 1 {
			return 1, nil
		}
		return 0, nil
	}
	if len(data) < 2 {
		return 0, errors.New("resposta Modbus curta")
	}
	return uint16(data[0])<<8 | uint16(data[1]), nil
}

func normalizeAddress(kind string, address int) (uint16, error) {
	switch kind {
	case "holding":
		if address >= 40001 && address <= 49999 {
			address -= 40001
		}
	case "input":
		if address >= 30001 && address <= 39999 {
			address -= 30001
		}
	case "discrete":
		if address >= 10001 && address <= 19999 {
			address -= 10001
		}
	case "coil":
		if address >= 1 && address <= 9999 {
			address--
		}
	default:
		return 0, fmt.Errorf("tipo inválido: %s", kind)
	}
	if address < 0 || address > 65535 {
		return 0, errors.New("endereço fora do intervalo")
	}
	return uint16(address), nil
}

func runMQTT(ctx context.Context, config MQTTConfig) {
	opts := mqtt.NewClientOptions().AddBroker(mqttBrokerURL(config)).SetClientID(config.ClientID)
	opts.SetKeepAlive(config.KeepAlive)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(10 * time.Second)
	opts.SetUsername(config.Username)
	opts.SetPassword(config.Password)
	opts.SetOnConnectHandler(func(mqtt.Client) {
		runtime.Lock()
		runtime.status.MQTTConnected = true
		runtime.status.LastMQTTError = ""
		runtime.Unlock()
		writeRuntimeStatus()
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		runtime.Lock()
		runtime.status.MQTTConnected = false
		runtime.status.LastMQTTError = err.Error()
		runtime.Unlock()
		writeRuntimeStatus()
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(30*time.Second) || token.Error() != nil {
		err := token.Error()
		if err == nil {
			err = errors.New("timeout conectando ao broker")
		}
		runtime.Lock()
		runtime.status.LastMQTTError = err.Error()
		runtime.Unlock()
		writeRuntimeStatus()
	}
	defer client.Disconnect(250)

	publishTicker := time.NewTicker(config.PublishInterval)
	systemTicker := time.NewTicker(config.SystemInterval)
	defer publishTicker.Stop()
	defer systemTicker.Stop()
	lastPublished := make(map[string]int64)

	for {
		select {
		case <-ctx.Done():
			return
		case <-publishTicker.C:
			publishTags(client, config, lastPublished)
		case <-systemTicker.C:
			publishSystem(client, config)
		}
	}
}

func publishTags(client mqtt.Client, config MQTTConfig, lastPublished map[string]int64) {
	document := snapshotState()
	for deviceKey, device := range document.Devices {
		for _, tag := range device.Values {
			if tag.Quality != "good" {
				continue
			}
			key := deviceKey + "/" + tag.Name
			if lastPublished[key] >= tag.Timestamp {
				continue
			}
			topic := strings.Join([]string{sanitize(config.TopicPrefix), sanitize(device.Name), sanitize(tag.Name)}, "/")
			if publishJSON(client, topic, tag, config) {
				lastPublished[key] = tag.Timestamp
			}
		}
	}
}

func publishSystem(client mqtt.Client, config MQTTConfig) {
	payload := map[string]any{
		"timestamp":        time.Now().Unix(),
		"hostname":         hostname(),
		"uptime":           firstField("/proc/uptime"),
		"loadavg":          firstField("/proc/loadavg"),
		"memory":           memorySummary(),
		"tailscale_socket": fileExists("/tmp/tailscale-runtime/tailscaled.sock"),
	}
	publishJSON(client, sanitize(config.TopicPrefix)+"/system/keepalive", payload, config)
}

func publishJSON(client mqtt.Client, topic string, payload any, config MQTTConfig) bool {
	if !client.IsConnected() {
		return false
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	token := client.Publish(topic, config.QoS, config.Retain, data)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		return false
	}
	runtime.Lock()
	runtime.status.Published++
	runtime.status.LastPublish = time.Now().Unix()
	runtime.Unlock()
	writeRuntimeStatus()
	return true
}

func snapshotState() StateDocument {
	state.RLock()
	defer state.RUnlock()
	return cloneStateLocked()
}

func cloneStateLocked() StateDocument {
	doc := StateDocument{UpdatedAt: state.doc.UpdatedAt, Devices: make(map[string]DeviceState, len(state.doc.Devices))}
	for key, device := range state.doc.Devices {
		device.Values = append([]TagValue(nil), device.Values...)
		doc.Devices[key] = device
	}
	return doc
}

func writeRuntimeStatus() {
	runtime.RLock()
	status := runtime.status
	runtime.RUnlock()
	_ = writeJSONAtomic(statusPath, status, 0644)
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, mode); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func option(options map[string]string, key, fallback string) string {
	if value := options[key]; value != "" {
		return value
	}
	return fallback
}

func enabled(options map[string]string, fallback bool) bool {
	value, ok := options["enabled"]
	if !ok {
		return fallback
	}
	return value == "1" || strings.EqualFold(value, "true")
}

func intOption(options map[string]string, key string, fallback int) int {
	value, err := strconv.Atoi(options[key])
	if err != nil {
		return fallback
	}
	return value
}

func floatOption(options map[string]string, key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(options[key], 64)
	if err != nil {
		return fallback
	}
	return value
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, " ", "_")
	if value == "" {
		return "unnamed"
	}
	return value
}

func hostname() string {
	name, _ := os.Hostname()
	return name
}

func firstField(path string) string {
	data, _ := os.ReadFile(path)
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func memorySummary() map[string]string {
	result := make(map[string]string)
	data, _ := os.ReadFile("/proc/meminfo")
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			if key == "MemTotal" || key == "MemAvailable" || key == "MemFree" {
				result[key+"KB"] = fields[1]
			}
		}
	}
	return result
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
