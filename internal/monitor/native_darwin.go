//go:build darwin && cgo

package monitor

/*
#cgo LDFLAGS: -framework CoreDisplay -framework CoreGraphics -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdint.h>

typedef CFTypeRef IOAVServiceRef;

extern IOAVServiceRef IOAVServiceCreate(CFAllocatorRef allocator);
extern int32_t IOAVServiceReadI2C(IOAVServiceRef service, uint32_t chipAddress,
    uint32_t offset, void *outputBuffer, uint32_t outputBufferSize);
extern int32_t IOAVServiceWriteI2C(IOAVServiceRef service, uint32_t chipAddress,
    uint32_t dataAddress, void *inputBuffer, uint32_t inputBufferSize);

static int monux_first_external_display(uint32_t *displayID, uint32_t *vendor,
    uint32_t *model) {
    CGDirectDisplayID displays[32];
    uint32_t count = 0;
    if (CGGetOnlineDisplayList(32, displays, &count) != kCGErrorSuccess) {
        return 0;
    }
    for (uint32_t i = 0; i < count; i++) {
        if (!CGDisplayIsBuiltin(displays[i])) {
            *displayID = displays[i];
            *vendor = CGDisplayVendorNumber(displays[i]);
            *model = CGDisplayModelNumber(displays[i]);
            return 1;
        }
    }
    return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/dyike/monux/internal/ddc"
)

type NativeBackend struct {
	selected bool
	mu       sync.Mutex
	sleep    func(time.Duration)
}

func NewNativeBackend(id string) (Backend, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed != "" && trimmed != "1" {
		return nil, fmt.Errorf("native macOS backend currently supports one external monitor; use monitor.id %q", "1")
	}
	return &NativeBackend{selected: trimmed != "", sleep: time.Sleep}, nil
}

func (b *NativeBackend) Detect() ([]Display, error) {
	var displayID, vendor, model C.uint32_t
	if C.monux_first_external_display(&displayID, &vendor, &model) == 0 {
		return nil, nil
	}
	return []Display{{
		ID:   "1",
		Name: fmt.Sprintf("display %d (vendor 0x%04x, model 0x%04x)", uint32(displayID), uint32(vendor), uint32(model)),
	}}, nil
}

func (b *NativeBackend) CurrentInput() (Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.requireSelection(); err != nil {
		return 0, err
	}
	service, err := openDarwinDisplayService()
	if err != nil {
		return 0, err
	}
	defer C.CFRelease(service)

	request := ddc.GetVCPRequest(ddc.VCPInputSource)
	if code := darwinWrite(service, request); code != 0 {
		return 0, fmt.Errorf("write DDC/CI request: IOKit error 0x%08x", uint32(code))
	}
	b.sleep(40 * time.Millisecond)
	reply := make([]byte, 12)
	code := C.IOAVServiceReadI2C(
		service,
		C.uint32_t(ddc.DisplayAddress),
		C.uint32_t(ddc.HostSourceAddress),
		unsafe.Pointer(&reply[0]),
		C.uint32_t(len(reply)),
	)
	if code != 0 {
		return 0, fmt.Errorf("read DDC/CI reply: IOKit error 0x%08x", uint32(code))
	}
	value, err := ddc.ParseVCPReply(reply[:11], ddc.VCPInputSource)
	if err != nil {
		return 0, err
	}
	return Input(value.Current), nil
}

func (b *NativeBackend) SetInput(input Input) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.requireSelection(); err != nil {
		return err
	}
	service, err := openDarwinDisplayService()
	if err != nil {
		return err
	}
	defer C.CFRelease(service)

	request := ddc.SetVCPRequest(ddc.VCPInputSource, uint16(input))
	if code := darwinWrite(service, request); code != 0 {
		return fmt.Errorf("set input source: IOKit error 0x%08x", uint32(code))
	}
	b.sleep(50 * time.Millisecond)
	return nil
}

func (b *NativeBackend) requireSelection() error {
	if !b.selected {
		return errors.New("monitor.id is required; run monux detect and configure the external monitor id")
	}
	return nil
}

func openDarwinDisplayService() (C.IOAVServiceRef, error) {
	service := C.IOAVServiceCreate(nil)
	if service == nil {
		return nil, errors.New("CoreDisplay could not open an external display DDC service")
	}
	return service, nil
}

func darwinWrite(service C.IOAVServiceRef, request []byte) C.int32_t {
	return C.IOAVServiceWriteI2C(
		service,
		C.uint32_t(ddc.DisplayAddress),
		C.uint32_t(request[0]),
		unsafe.Pointer(&request[1]),
		C.uint32_t(len(request)-1),
	)
}
