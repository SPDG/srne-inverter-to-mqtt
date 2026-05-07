package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	srnemodbus "github.com/tomasz/srne-inverter-to-mqtt/internal/modbus"
)

func main() {
	port := flag.String("port", "", "Serial port path or tcp://host:port")
	networkProtocol := flag.String("network-protocol", "rtu", "Modbus transport: rtu, rtu_over_tcp, or modbus_tcp")
	slave := flag.Int("slave", 1, "Modbus slave ID")
	baud := flag.Int("baud", 9600, "Baud rate")
	timeout := flag.Duration("timeout", 2*time.Second, "Serial timeout")
	address := flag.Int("address", 0x0100, "Holding register address")
	count := flag.Int("count", 1, "Holding register count")
	flag.Parse()

	if strings.TrimSpace(*port) == "" {
		log.Fatal("port is required")
	}
	if *slave < 1 || *slave > 247 {
		log.Fatal("slave must be between 1 and 247")
	}

	cfg := config.Default()
	cfg.Device.SlaveID = uint8(*slave)
	cfg.Serial.Port = *port
	cfg.Serial.NetworkProtocol = *networkProtocol
	cfg.Serial.BaudRate = *baud
	cfg.Serial.DataBits = 8
	cfg.Serial.Parity = "N"
	cfg.Serial.StopBits = 1
	cfg.Serial.Timeout = config.Duration{Duration: *timeout}

	client, closer, err := srnemodbus.OpenClient(cfg)
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer closer.Close()

	start := time.Now()
	data, err := client.ReadHoldingRegisters(uint16(*address), uint16(*count))
	elapsed := time.Since(start)
	if err != nil {
		log.Fatalf("read failed after %s: %v", elapsed.Round(time.Millisecond), err)
	}

	mode, _ := cfg.Serial.ConnectionMode()
	fmt.Printf("ok port=%s mode=%s slave=%d address=0x%04X count=%d elapsed=%s bytes=% X\n",
		*port, mode, *slave, *address, *count, elapsed.Round(time.Millisecond), data)
}
