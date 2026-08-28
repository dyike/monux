//go:build darwin && cgo

package monitor

/*
#cgo LDFLAGS: -framework IOKit -framework CoreGraphics -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdint.h>

typedef CFTypeRef IOAVServiceRef;

extern IOAVServiceRef IOAVServiceCreate(CFAllocatorRef allocator);
extern int32_t IOAVServiceReadI2C(IOAVServiceRef service, uint32_t chipAddress,
    uint32_t offset, void *outputBuffer, uint32_t outputBufferSize);
extern int32_t IOAVServiceWriteI2C(IOAVServiceRef service, uint32_t chipAddress,
    uint32_t dataAddress, void *inputBuffer, uint32_t inputBufferSize);

static IOAVServiceRef monux_open_display_service(void) {
    return IOAVServiceCreate(NULL);
}

static int monux_display_service_is_null(IOAVServiceRef service) {
    return service == NULL;
}

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

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		request := ddc.GetVCPRequest(ddc.VCPInputSource)
		if code := b.writeRequest(service, request); code != 0 {
			lastErr = fmt.Errorf("write request: IOKit error 0x%08x", uint32(code))
			continue
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
			lastErr = fmt.Errorf("read reply: IOKit error 0x%08x", uint32(code))
			continue
		}
		value, err := ddc.ParseVCPReply(reply[:11], ddc.VCPInputSource)
		if err == nil {
			return Input(value.Current), nil
		}
		lastErr = err
		b.sleep(100 * time.Millisecond)
	}
	return 0, fmt.Errorf("read input source failed after 3 attempts: %w", lastErr)
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
	if code := b.writeRequest(service, request); code != 0 {
		return fmt.Errorf("set input source: IOKit error 0x%08x", uint32(code))
	}
	b.sleep(50 * time.Millisecond)
	return nil
}

func (b *NativeBackend) SupportedInputs() ([]Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.requireSelection(); err != nil {
		return nil, err
	}
	service, err := openDarwinDisplayService()
	if err != nil {
		return nil, err
	}
	defer C.CFRelease(service)

	readFragment := func(offset uint16) ([]byte, error) {
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			request := ddc.CapabilitiesRequest(offset)
			if code := b.writeRequest(service, request); code != 0 {
				lastErr = fmt.Errorf("write request: IOKit error 0x%08x", uint32(code))
				continue
			}
			b.sleep(200 * time.Millisecond)

			reply := make([]byte, 38)
			code := C.IOAVServiceReadI2C(
				service,
				C.uint32_t(ddc.DisplayAddress),
				C.uint32_t(ddc.HostSourceAddress),
				unsafe.Pointer(&reply[0]),
				C.uint32_t(len(reply)),
			)
			if code != 0 {
				lastErr = fmt.Errorf("read reply: IOKit error 0x%08x", uint32(code))
				continue
			}
			fragment, err := ddc.ParseCapabilitiesReply(reply, offset)
			if err == nil {
				b.sleep(50 * time.Millisecond)
				return fragment, nil
			}
			lastErr = err
			b.sleep(100 * time.Millisecond)
		}
		return nil, fmt.Errorf("capabilities offset %d failed after 3 attempts: %w", offset, lastErr)
	}
	var capabilities string
	for attempt := 1; attempt <= 2; attempt++ {
		capabilities, err = ddc.ReadCapabilities(readFragment)
		if err == nil {
			break
		}
		b.sleep(500 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("read complete monitor capabilities after 2 attempts: %w", err)
	}
	inputs, err := inputsFromCapabilities(capabilities)
	if err != nil {
		return nil, fmt.Errorf("parse monitor input capabilities: %w", err)
	}
	return inputs, nil
}

func (b *NativeBackend) requireSelection() error {
	if !b.selected {
		return errors.New("monitor.id is required; run monux detect and configure the external monitor id")
	}
	return nil
}

func openDarwinDisplayService() (C.IOAVServiceRef, error) {
	service := C.monux_open_display_service()
	if C.monux_display_service_is_null(service) != 0 {
		var zero C.IOAVServiceRef
		return zero, errors.New("IOKit could not open an external display DDC service")
	}
	return service, nil
}

func darwinWrite(service C.IOAVServiceRef, request []byte) C.int32_t {
	payload := darwinRequestPayload(request)
	return C.IOAVServiceWriteI2C(
		service,
		C.uint32_t(ddc.DisplayAddress),
		C.uint32_t(request[0]),
		unsafe.Pointer(&payload[0]),
		C.uint32_t(len(payload)),
	)
}

func (b *NativeBackend) writeRequest(service C.IOAVServiceRef, request []byte) C.int32_t {
	for range 2 {
		b.sleep(10 * time.Millisecond)
		if code := darwinWrite(service, request); code != 0 {
			return code
		}
	}
	return 0
}

func darwinRequestPayload(request []byte) []byte {
	payload := append([]byte(nil), request[1:]...)
	// IOAVService receives the host source address separately. For a Get VCP
	// request, its transport expects the checksum to exclude that address.
	if len(request) == 5 && request[2] == 0x01 {
		payload[len(payload)-1] ^= request[0]
	}
	return payload
}
