package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wagnee/Tailscale-ZLAN9809M-Hybrid/internal/uci"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/goburrow/modbus"
)

func runCommand(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case "mqtt-publish":
		if len(args) != 3 {
			return true, errors.New("uso: zlan-telemetryd mqtt-publish TOPICO MENSAGEM")
		}
		return true, commandMQTTPublish(args[1], args[2])
	case "mqtt-subscribe":
		if len(args) < 2 || len(args) > 3 {
			return true, errors.New("uso: zlan-telemetryd mqtt-subscribe TOPICO [TIMEOUT_SEGUNDOS]")
		}
		timeout := 10
		if len(args) == 3 {
			value, err := strconv.Atoi(args[2])
			if err != nil || value < 1 || value > 30 {
				return true, errors.New("timeout MQTT deve estar entre 1 e 30 segundos")
			}
			timeout = value
		}
		return true, commandMQTTSubscribe(args[1], time.Duration(timeout)*time.Second)
	case "modbus-write-coil":
		if len(args) != 4 {
			return true, errors.New("uso: zlan-telemetryd modbus-write-coil DISPOSITIVO ENDERECO 0|1")
		}
		address, err := strconv.Atoi(args[2])
		if err != nil || address < 0 || address > 65535 {
			return true, errors.New("endereco do coil deve estar entre 0 e 65535")
		}
		if args[3] != "0" && args[3] != "1" {
			return true, errors.New("valor do coil deve ser 0 ou 1")
		}
		return true, commandModbusWriteCoil(args[1], address, args[3] == "1")
	default:
		return true, fmt.Errorf("comando desconhecido: %s", args[0])
	}
}

func mqttBrokerURL(config MQTTConfig) string {
	if strings.Contains(config.Broker, "://") {
		return config.Broker
	}
	return fmt.Sprintf("tcp://%s:%d", config.Broker, config.Port)
}

func connectTestMQTT(config MQTTConfig, operation string) (mqtt.Client, error) {
	clientID := fmt.Sprintf("z%s-%06d-%d", operation[:1], time.Now().Unix()%1000000, os.Getpid())
	opts := mqtt.NewClientOptions().AddBroker(mqttBrokerURL(config)).SetClientID(clientID)
	opts.SetUsername(config.Username)
	opts.SetPassword(config.Password)
	opts.SetKeepAlive(config.KeepAlive)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetAutoReconnect(false)
	opts.SetConnectRetry(false)
	opts.SetCleanSession(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(12 * time.Second) {
		return nil, errors.New("timeout conectando ao broker MQTT")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("falha conectando ao broker MQTT: %w", err)
	}
	return client, nil
}

func validateMQTTTopic(topic string, allowWildcards bool) error {
	if topic == "" || len(topic) > 256 || strings.ContainsRune(topic, '\x00') {
		return errors.New("topico MQTT invalido")
	}
	if !allowWildcards && strings.ContainsAny(topic, "+#") {
		return errors.New("wildcards nao sao permitidos ao publicar")
	}
	return nil
}

func commandMQTTPublish(topic, message string) error {
	if err := validateMQTTTopic(topic, false); err != nil {
		return err
	}
	if len(message) > 2048 {
		return errors.New("mensagem MQTT excede 2048 bytes")
	}
	config, err := loadMQTTConfig(mqttConfigPath)
	if err != nil {
		return err
	}
	client, err := connectTestMQTT(config, "publish")
	if err != nil {
		return err
	}
	defer client.Disconnect(250)

	token := client.Publish(topic, config.QoS, false, message)
	if !token.WaitTimeout(10 * time.Second) {
		return errors.New("timeout publicando mensagem MQTT")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("falha publicando MQTT: %w", err)
	}
	fmt.Printf("OK: mensagem publicada em %s via %s\n", topic, mqttBrokerURL(config))
	return nil
}

func commandMQTTSubscribe(topic string, timeout time.Duration) error {
	if err := validateMQTTTopic(topic, true); err != nil {
		return err
	}
	config, err := loadMQTTConfig(mqttConfigPath)
	if err != nil {
		return err
	}
	client, err := connectTestMQTT(config, "subscribe")
	if err != nil {
		return err
	}
	defer client.Disconnect(250)

	messages := make(chan mqtt.Message, 1)
	token := client.Subscribe(topic, config.QoS, func(_ mqtt.Client, message mqtt.Message) {
		select {
		case messages <- message:
		default:
		}
	})
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if token.Error() != nil {
			return fmt.Errorf("falha subscrevendo MQTT: %w", token.Error())
		}
		return errors.New("timeout subscrevendo ao topico MQTT")
	}

	fmt.Printf("Aguardando mensagem em %s por %s...\n", topic, timeout)
	select {
	case message := <-messages:
		payload := message.Payload()
		if len(payload) > 4096 {
			payload = payload[:4096]
		}
		fmt.Printf("OK: topico=%s retained=%t payload=%s\n", message.Topic(), message.Retained(), string(payload))
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("nenhuma mensagem recebida em %s durante %s", topic, timeout)
	}
}

func loadModbusDevice(path, key string) (DeviceConfig, error) {
	sections, err := uci.Parse(path)
	if err != nil {
		return DeviceConfig{}, err
	}
	for _, section := range sections {
		if section.Type != "device" || section.Name != key {
			continue
		}
		ip := section.Options["ip"]
		port := intOption(section.Options, "port", 502)
		slave := intOption(section.Options, "slave_id", 1)
		timeout := intOption(section.Options, "timeout", 5)
		if ip == "" || port < 1 || port > 65535 || slave < 0 || slave > 247 {
			return DeviceConfig{}, fmt.Errorf("dispositivo Modbus %s possui IP, porta ou slave ID invalido", key)
		}
		if timeout < 1 {
			timeout = 5
		}
		if timeout > 10 {
			timeout = 10
		}
		return DeviceConfig{
			Key: key, Name: option(section.Options, "name", key),
			Address: fmt.Sprintf("%s:%d", ip, port), SlaveID: byte(slave),
			Timeout: time.Duration(timeout) * time.Second,
		}, nil
	}
	return DeviceConfig{}, fmt.Errorf("dispositivo Modbus nao encontrado: %s", key)
}

func commandModbusWriteCoil(deviceKey string, address int, value bool) error {
	device, err := loadModbusDevice(modbusConfigPath, deviceKey)
	if err != nil {
		return err
	}
	normalized, err := normalizeAddress("coil", address)
	if err != nil {
		return err
	}
	handler := modbus.NewTCPClientHandler(device.Address)
	handler.Timeout = device.Timeout
	handler.SlaveId = device.SlaveID
	if err := handler.Connect(); err != nil {
		return fmt.Errorf("falha conectando ao Modbus %s: %w", device.Address, err)
	}
	defer handler.Close()

	client := modbus.NewClient(handler)
	writeValue := uint16(0x0000)
	if value {
		writeValue = 0xFF00
	}
	if _, err := client.WriteSingleCoil(normalized, writeValue); err != nil {
		return fmt.Errorf("falha escrevendo coil: %w", err)
	}
	data, err := client.ReadCoils(normalized, 1)
	if err != nil || len(data) == 0 {
		return fmt.Errorf("coil escrito, mas a confirmacao de leitura falhou: %v", err)
	}
	readValue := data[0]&1 == 1
	if readValue != value {
		return fmt.Errorf("coil escrito como %t, mas lido como %t", value, readValue)
	}
	fmt.Printf("OK: dispositivo=%s endereco=%d offset=%d valor=%t confirmado\n", device.Name, address, normalized, value)
	return nil
}
