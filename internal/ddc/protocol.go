// Package ddc implements the platform-independent parts of the VESA DDC/CI
// protocol. Platform backends are responsible only for transporting frames.
package ddc

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	DisplayAddress     = 0x37
	HostSourceAddress  = 0x51
	VirtualHostAddress = 0x50
	VCPInputSource     = 0x60

	opGetVCP              = 0x01
	opGetVCPReply         = 0x02
	opSetVCP              = 0x03
	opCapabilitiesRequest = 0xf3
	opCapabilitiesReply   = 0xe3
)

var inputCapabilityPattern = regexp.MustCompile(`(?i)\b60\s*\(([^)]*)\)`)

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

// CapabilitiesRequest requests a fragment of the monitor capabilities string.
func CapabilitiesRequest(offset uint16) []byte {
	return hostMessage([]byte{opCapabilitiesRequest, byte(offset >> 8), byte(offset)})
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

// ParseCapabilitiesReply returns one capabilities-string fragment.
func ParseCapabilitiesReply(reply []byte, requestedOffset uint16) ([]byte, error) {
	if len(reply) < 6 {
		return nil, fmt.Errorf("DDC/CI capabilities reply is too short: got %d bytes", len(reply))
	}
	length := int(reply[1] & 0x7f)
	if length < 3 {
		return nil, fmt.Errorf("invalid capabilities payload length %d", length)
	}
	total := length + 3
	if len(reply) < total {
		return nil, fmt.Errorf("truncated capabilities reply: got %d bytes, want %d", len(reply), total)
	}
	if reply[0] != 0x6e || reply[2] != opCapabilitiesReply {
		return nil, fmt.Errorf("unexpected capabilities reply header % x", reply[:3])
	}
	offset := uint16(reply[3])<<8 | uint16(reply[4])
	if offset != requestedOffset {
		return nil, fmt.Errorf("capabilities reply offset %d, requested %d", offset, requestedOffset)
	}
	if checksum(VirtualHostAddress, reply[:total]) != 0 {
		return nil, fmt.Errorf("invalid DDC/CI capabilities reply checksum")
	}
	return append([]byte(nil), reply[5:total-1]...), nil
}

// ParseInputCapabilities extracts the values listed for VCP 0x60 from a
// monitor capabilities string.
func ParseInputCapabilities(capabilities string) ([]uint16, error) {
	matches := inputCapabilityPattern.FindStringSubmatch(capabilities)
	if len(matches) != 2 {
		return nil, fmt.Errorf("capabilities string does not report values for VCP 0x60")
	}
	seen := make(map[uint16]bool)
	values := make([]uint16, 0)
	for _, field := range strings.Fields(matches[1]) {
		field = strings.TrimPrefix(strings.ToLower(field), "0x")
		value, err := strconv.ParseUint(field, 16, 16)
		if err != nil {
			continue
		}
		input := uint16(value)
		if !seen[input] {
			seen[input] = true
			values = append(values, input)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("capabilities string reports an empty VCP 0x60 value list")
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values, nil
}

// ReadCapabilities joins the offset-based fragments returned by a display.
// The display terminates its capabilities string with a NUL byte.
func ReadCapabilities(readFragment func(offset uint16) ([]byte, error)) (string, error) {
	const maximumLength = 16 * 1024

	capabilities := make([]byte, 0, 512)
	for len(capabilities) < maximumLength {
		offset := uint16(len(capabilities))
		fragment, err := readFragment(offset)
		if err != nil {
			return "", err
		}
		if end := bytes.IndexByte(fragment, 0); end >= 0 {
			capabilities = append(capabilities, fragment[:end]...)
			return string(capabilities), nil
		}
		if len(fragment) == 0 {
			return "", fmt.Errorf("display returned an empty capabilities fragment at offset %d", offset)
		}
		if len(capabilities)+len(fragment) > maximumLength {
			return "", fmt.Errorf("monitor capabilities string exceeds %d bytes", maximumLength)
		}
		capabilities = append(capabilities, fragment...)
	}
	return "", fmt.Errorf("monitor capabilities string is not NUL-terminated")
}

func checksum(initial byte, data []byte) byte {
	result := initial
	for _, value := range data {
		result ^= value
	}
	return result
}
