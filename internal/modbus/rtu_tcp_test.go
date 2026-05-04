package modbus

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	gomodbus "github.com/goburrow/modbus"
)

func TestRTUOverTCPReadHoldingRegisters(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	errs := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errs <- err
			return
		}
		defer conn.Close()

		request := make([]byte, 8)
		if _, err := io.ReadFull(conn, request); err != nil {
			errs <- err
			return
		}
		if got, want := frameCRC(request), calculateCRC(request[:len(request)-2]); got != want {
			errs <- &crcMismatch{got: got, want: want}
			return
		}
		if want := []byte{1, 3, 1, 0, 0, 1}; !bytes.Equal(request[:6], want) {
			errs <- &requestMismatch{got: append([]byte(nil), request[:6]...), want: want}
			return
		}

		response := []byte{1, 3, 2, 0, 42, 0, 0}
		appendCRC(response)
		_, err = conn.Write(response)
		errs <- err
	}()

	handler := newRTUOverTCPClientHandler(listener.Addr().String(), 1, time.Second)
	if err := handler.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer handler.Close()

	result, err := gomodbus.NewClient(handler).ReadHoldingRegisters(0x0100, 1)
	if err != nil {
		t.Fatalf("ReadHoldingRegisters() error = %v", err)
	}
	if want := []byte{0, 42}; !bytes.Equal(result, want) {
		t.Fatalf("ReadHoldingRegisters() = % X, want % X", result, want)
	}

	if err := <-errs; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

type crcMismatch struct {
	got  uint16
	want uint16
}

func (e *crcMismatch) Error() string {
	return fmt.Sprintf("crc mismatch: got %d want %d", e.got, e.want)
}

type requestMismatch struct {
	got  []byte
	want []byte
}

func (e *requestMismatch) Error() string {
	return fmt.Sprintf("request = % X, want % X", e.got, e.want)
}
