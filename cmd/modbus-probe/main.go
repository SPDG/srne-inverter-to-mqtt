package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	gomodbus "github.com/goburrow/modbus"
)

func main() {
	port := flag.String("port", "", "Serial port path")
	slave := flag.Int("slave", 1, "Modbus slave ID")
	baud := flag.Int("baud", 9600, "Baud rate")
	timeout := flag.Duration("timeout", 2*time.Second, "Serial timeout")
	address := flag.Int("address", 0x0100, "Holding register address")
	count := flag.Int("count", 1, "Holding register count")
	flag.Parse()

	if strings.TrimSpace(*port) == "" {
		log.Fatal("port is required")
	}

	handler := gomodbus.NewRTUClientHandler(*port)
	handler.BaudRate = *baud
	handler.DataBits = 8
	handler.Parity = "N"
	handler.StopBits = 1
	handler.Timeout = *timeout
	handler.SlaveId = byte(*slave)

	if err := handler.Connect(); err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer handler.Close()

	client := gomodbus.NewClient(handler)
	start := time.Now()
	data, err := client.ReadHoldingRegisters(uint16(*address), uint16(*count))
	elapsed := time.Since(start)
	if err != nil {
		log.Fatalf("read failed after %s: %v", elapsed.Round(time.Millisecond), err)
	}

	fmt.Printf("ok port=%s slave=%d address=0x%04X count=%d elapsed=%s bytes=% X\n",
		*port, *slave, *address, *count, elapsed.Round(time.Millisecond), data)
}
