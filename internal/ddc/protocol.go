// Package ddc implements the platform-independent parts of the VESA DDC/CI
// protocol. Platform backends are responsible only for transporting frames.
package ddc

import "fmt"

const (
	DisplayAddress     = 0x37
	HostSourceAddress  = 0x51
	VirtualHostAddress = 0x50
	VCPInputSource     = 0x60

	opGetVCP      = 0x01
	opGetVCPReply = 0x02
	opSetVCP      = 0x03
)

type VCPValue struct {
	Code    byte
	Type    byte
	Maximum uint16
	Current uint16
}

// GetVCPRequest returns the bytes written to the display's 7-bit I2C address.
func GetVCPRequest(code byte) []byte {
	return hostMessage([]byte{opGetVCP, code})
}

// SetVCPRequest returns the bytes written to the display's 7-bit I2C address.
func SetVCPRequest(code byte, value uint16) []byte {
	return hostMessage([]byte{opSetVCP, code, byte(value >> 8), byte(value)})
}

func hostMessage(data []byte) []byte {
	message := make([]byte, 0, len(data)+3)
	message = append(message, HostSourceAddress, 0x80|byte(len(data)))
	message = append(message, data...)
	message = append(message, checksum(0x6e, message))
	return message
}

// ParseVCPReply parses the 11-byte Get VCP Feature Reply returned by a display.
func ParseVCPReply(reply []byte, requestedCode byte) (VCPValue, error) {
	if len(reply) < 11 {
		return VCPValue{}, fmt.Errorf("DDC/CI reply is too short: got %d bytes, want at least 11", len(reply))
	}
	length := int(reply[1] & 0x7f)
	if length != 8 {
		return VCPValue{}, fmt.Errorf("unexpected DDC/CI reply payload length %d", length)
	}
	if reply[0] != 0x6e || reply[2] != opGetVCPReply {
		return VCPValue{}, fmt.Errorf("unexpected DDC/CI Get VCP reply header % x", reply[:3])
	}
	if reply[3] != 0 {
		return VCPValue{}, fmt.Errorf("display reports unsupported VCP code 0x%02x (result 0x%02x)", requestedCode, reply[3])
	}
	if reply[4] != requestedCode {
		return VCPValue{}, fmt.Errorf("reply is for VCP code 0x%02x, requested 0x%02x", reply[4], requestedCode)
	}
	if checksum(VirtualHostAddress, reply[:11]) != 0 {
		return VCPValue{}, fmt.Errorf("invalid DDC/CI reply checksum")
	}
	return VCPValue{
		Code:    reply[4],
		Type:    reply[5],
		Maximum: uint16(reply[6])<<8 | uint16(reply[7]),
		Current: uint16(reply[8])<<8 | uint16(reply[9]),
	}, nil
}

func checksum(initial byte, data []byte) byte {
	result := initial
	for _, value := range data {
		result ^= value
	}
	return result
}
