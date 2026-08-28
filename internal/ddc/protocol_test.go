package ddc

import (
	"slices"
	"testing"
)

func TestGetVCPRequest(t *testing.T) {
	want := []byte{0x51, 0x82, 0x01, 0xdf, 0x63}
	if got := GetVCPRequest(0xdf); !slices.Equal(got, want) {
		t.Fatalf("GetVCPRequest() = % x, want % x", got, want)
	}
}

func TestSetVCPRequest(t *testing.T) {
	want := []byte{0x51, 0x84, 0x03, 0x60, 0x00, 0x11, 0xc9}
	if got := SetVCPRequest(VCPInputSource, 0x11); !slices.Equal(got, want) {
		t.Fatalf("SetVCPRequest() = % x, want % x", got, want)
	}
}

func TestCapabilitiesRequest(t *testing.T) {
	want := []byte{0x51, 0x83, 0xf3, 0x00, 0x00, 0x4f}
	if got := CapabilitiesRequest(0); !slices.Equal(got, want) {
		t.Fatalf("CapabilitiesRequest() = % x, want % x", got, want)
	}
}

func TestReadCapabilities(t *testing.T) {
	fragments := map[uint16][]byte{
		0:  []byte("(prot(monitor)vcp(60"),
		20: []byte("(0f 11)))\x00ignored"),
	}
	got, err := ReadCapabilities(func(offset uint16) ([]byte, error) {
		return fragments[offset], nil
	})
	if err != nil {
		t.Fatalf("ReadCapabilities() error = %v", err)
	}
	if want := "(prot(monitor)vcp(60(0f 11)))"; got != want {
		t.Fatalf("ReadCapabilities() = %q, want %q", got, want)
	}
}

func TestParseVCPReply(t *testing.T) {
	reply := []byte{0x6e, 0x88, 0x02, 0x00, 0xdf, 0x00, 0xff, 0xff, 0x02, 0x01, 0x68}
	got, err := ParseVCPReply(reply, 0xdf)
	if err != nil {
		t.Fatalf("ParseVCPReply() error = %v", err)
	}
	if got.Code != 0xdf || got.Type != 0 || got.Maximum != 0xffff || got.Current != 0x0201 {
		t.Fatalf("ParseVCPReply() = %#v", got)
	}
}

func TestParseVCPReplyRejectsInvalidMessages(t *testing.T) {
	valid := []byte{0x6e, 0x88, 0x02, 0x00, 0xdf, 0x00, 0xff, 0xff, 0x02, 0x01, 0x68}
	tests := [][]byte{
		valid[:10],
		append([]byte(nil), valid...),
		append([]byte(nil), valid...),
		append([]byte(nil), valid...),
	}
	tests[1][1] = 0x87
	tests[2][3] = 1
	tests[3][10] ^= 0xff
	for _, reply := range tests {
		if _, err := ParseVCPReply(reply, 0xdf); err == nil {
			t.Fatalf("ParseVCPReply(% x) error = nil", reply)
		}
	}
}

func TestParseCapabilitiesReply(t *testing.T) {
	// Source, length, opcode, offset, "vcp", checksum.
	reply := []byte{0x6e, 0x86, 0xe3, 0x00, 0x00, 'v', 'c', 'p', 0x00}
	reply[8] = checksum(VirtualHostAddress, reply[:8])
	got, err := ParseCapabilitiesReply(reply, 0)
	if err != nil {
		t.Fatalf("ParseCapabilitiesReply() error = %v", err)
	}
	if !slices.Equal(got, []byte("vcp")) {
		t.Fatalf("ParseCapabilitiesReply() = %q", got)
	}
}

func TestParseInputCapabilities(t *testing.T) {
	got, err := ParseInputCapabilities("(prot(monitor)type(lcd)vcp(10 60( 11 0f 12 11 1b ) d6))")
	if err != nil {
		t.Fatalf("ParseInputCapabilities() error = %v", err)
	}
	want := []uint16{0x0f, 0x11, 0x12, 0x1b}
	if !slices.Equal(got, want) {
		t.Fatalf("ParseInputCapabilities() = %x, want %x", got, want)
	}
}
