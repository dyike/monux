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
