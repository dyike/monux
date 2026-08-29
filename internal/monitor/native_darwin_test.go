//go:build darwin && cgo

package monitor

import (
	"slices"
	"testing"

	"github.com/dyike/monux/internal/ddc"
)

func TestDarwinGetVCPRequestPayload(t *testing.T) {
	got := darwinRequestPayload(ddc.GetVCPRequest(ddc.VCPInputSource))
	want := []byte{0x82, 0x01, 0x60, 0x8d}
	if !slices.Equal(got, want) {
		t.Fatalf("darwinRequestPayload() = % x, want % x", got, want)
	}
}

func TestDarwinSetVCPRequestPayload(t *testing.T) {
	got := darwinRequestPayload(ddc.SetVCPRequest(ddc.VCPInputSource, 0x11))
	want := []byte{0x84, 0x03, 0x60, 0x00, 0x11, 0xc9}
	if !slices.Equal(got, want) {
		t.Fatalf("darwinRequestPayload() = % x, want % x", got, want)
	}
}

func TestNewDarwinNativeBackendAcceptsDisplayID(t *testing.T) {
	backend, err := NewNativeBackend(" 2 ")
	if err != nil {
		t.Fatalf("NewNativeBackend() error = %v", err)
	}
	native, ok := backend.(*NativeBackend)
	if !ok {
		t.Fatalf("NewNativeBackend() returned %T", backend)
	}
	if !native.selected || native.displayID != 2 {
		t.Fatalf("NewNativeBackend() selected = %t, displayID = %d", native.selected, native.displayID)
	}
}

func TestNewDarwinNativeBackendRejectsInvalidDisplayID(t *testing.T) {
	for _, id := range []string{"0", "not-a-display", "4294967296"} {
		if _, err := NewNativeBackend(id); err == nil {
			t.Errorf("NewNativeBackend(%q) error = nil", id)
		}
	}
}
