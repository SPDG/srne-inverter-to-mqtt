package modbus

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	gomodbus "github.com/goburrow/modbus"
)

const (
	rtuMinFrameSize       = 4
	rtuMaxFrameSize       = 256
	rtuExceptionFrameSize = 5
)

type rtuOverTCPClientHandler struct {
	address string
	slaveID byte
	timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
}

func newRTUOverTCPClientHandler(address string, slaveID byte, timeout time.Duration) *rtuOverTCPClientHandler {
	return &rtuOverTCPClientHandler{
		address: address,
		slaveID: slaveID,
		timeout: timeout,
	}
}

func (h *rtuOverTCPClientHandler) Connect() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.connectLocked()
}

func (h *rtuOverTCPClientHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closeLocked()
}

func (h *rtuOverTCPClientHandler) Encode(pdu *gomodbus.ProtocolDataUnit) ([]byte, error) {
	frameLen := len(pdu.Data) + 4
	if frameLen > rtuMaxFrameSize {
		return nil, fmt.Errorf("modbus: RTU frame length %d exceeds %d", frameLen, rtuMaxFrameSize)
	}

	frame := make([]byte, frameLen)
	frame[0] = h.slaveID
	frame[1] = pdu.FunctionCode
	copy(frame[2:], pdu.Data)
	appendCRC(frame)
	return frame, nil
}

func (h *rtuOverTCPClientHandler) Verify(request, response []byte) error {
	if len(response) < rtuMinFrameSize {
		return fmt.Errorf("modbus: response length %d is shorter than RTU minimum %d", len(response), rtuMinFrameSize)
	}
	if response[0] != request[0] {
		return fmt.Errorf("modbus: response slave id %d does not match request %d", response[0], request[0])
	}
	return nil
}

func (h *rtuOverTCPClientHandler) Decode(frame []byte) (*gomodbus.ProtocolDataUnit, error) {
	if len(frame) < rtuMinFrameSize {
		return nil, fmt.Errorf("modbus: response length %d is shorter than RTU minimum %d", len(frame), rtuMinFrameSize)
	}
	if got, want := frameCRC(frame), calculateCRC(frame[:len(frame)-2]); got != want {
		return nil, fmt.Errorf("modbus: response crc %d does not match expected %d", got, want)
	}
	return &gomodbus.ProtocolDataUnit{
		FunctionCode: frame[1],
		Data:         frame[2 : len(frame)-2],
	}, nil
}

func (h *rtuOverTCPClientHandler) Send(request []byte) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.connectLocked(); err != nil {
		return nil, err
	}
	if h.timeout > 0 {
		if err := h.conn.SetDeadline(time.Now().Add(h.timeout)); err != nil {
			return nil, err
		}
	}
	if _, err := h.conn.Write(request); err != nil {
		_ = h.closeLocked()
		return nil, err
	}

	responseLen := rtuResponseLength(request)
	var data [rtuMaxFrameSize]byte
	n, err := io.ReadAtLeast(h.conn, data[:], rtuMinFrameSize)
	if err != nil {
		_ = h.closeLocked()
		return nil, err
	}

	function := request[1]
	exceptionFunction := function | 0x80
	switch data[1] {
	case function:
		if responseLen > n {
			nn, readErr := io.ReadFull(h.conn, data[n:responseLen])
			n += nn
			if readErr != nil {
				_ = h.closeLocked()
				return nil, readErr
			}
		}
	case exceptionFunction:
		if rtuExceptionFrameSize > n {
			nn, readErr := io.ReadFull(h.conn, data[n:rtuExceptionFrameSize])
			n += nn
			if readErr != nil {
				_ = h.closeLocked()
				return nil, readErr
			}
		}
	default:
		_ = h.closeLocked()
		return nil, fmt.Errorf("modbus: unexpected response function 0x%02X for request 0x%02X", data[1], function)
	}

	return data[:n], nil
}

func (h *rtuOverTCPClientHandler) connectLocked() error {
	if h.conn != nil {
		return nil
	}
	dialer := net.Dialer{Timeout: h.timeout}
	conn, err := dialer.Dial("tcp", h.address)
	if err != nil {
		return err
	}
	h.conn = conn
	return nil
}

func (h *rtuOverTCPClientHandler) closeLocked() error {
	if h.conn == nil {
		return nil
	}
	err := h.conn.Close()
	h.conn = nil
	return err
}

func rtuResponseLength(request []byte) int {
	length := rtuMinFrameSize
	switch request[1] {
	case gomodbus.FuncCodeReadDiscreteInputs,
		gomodbus.FuncCodeReadCoils:
		count := int(binary.BigEndian.Uint16(request[4:]))
		length += 1 + count/8
		if count%8 != 0 {
			length++
		}
	case gomodbus.FuncCodeReadInputRegisters,
		gomodbus.FuncCodeReadHoldingRegisters,
		gomodbus.FuncCodeReadWriteMultipleRegisters:
		count := int(binary.BigEndian.Uint16(request[4:]))
		length += 1 + count*2
	case gomodbus.FuncCodeWriteSingleCoil,
		gomodbus.FuncCodeWriteMultipleCoils,
		gomodbus.FuncCodeWriteSingleRegister,
		gomodbus.FuncCodeWriteMultipleRegisters:
		length += 4
	case gomodbus.FuncCodeMaskWriteRegister:
		length += 6
	}
	return length
}

func appendCRC(frame []byte) {
	crc := calculateCRC(frame[:len(frame)-2])
	frame[len(frame)-2] = byte(crc)
	frame[len(frame)-1] = byte(crc >> 8)
}

func frameCRC(frame []byte) uint16 {
	return uint16(frame[len(frame)-2]) | uint16(frame[len(frame)-1])<<8
}

func calculateCRC(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for range 8 {
			if crc&1 == 0 {
				crc >>= 1
				continue
			}
			crc = (crc >> 1) ^ 0xA001
		}
	}
	return crc
}
