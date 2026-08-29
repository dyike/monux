//go:build darwin && cgo

package monitor

/*
#cgo LDFLAGS: -framework IOKit -framework CoreDisplay -framework CoreGraphics -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <IOKit/IOKitLib.h>
#include <stdint.h>
#include <string.h>

typedef CFTypeRef IOAVServiceRef;

extern IOAVServiceRef IOAVServiceCreate(CFAllocatorRef allocator);
extern IOAVServiceRef IOAVServiceCreateWithService(CFAllocatorRef allocator,
    io_service_t service);
extern int32_t IOAVServiceReadI2C(IOAVServiceRef service, uint32_t chipAddress,
    uint32_t offset, void *outputBuffer, uint32_t outputBufferSize);
extern int32_t IOAVServiceWriteI2C(IOAVServiceRef service, uint32_t chipAddress,
    uint32_t dataAddress, void *inputBuffer, uint32_t inputBufferSize);
extern CFDictionaryRef CoreDisplay_DisplayCreateInfoDictionary(
    CGDirectDisplayID displayID);

static IOAVServiceRef monux_open_display_service(CGDirectDisplayID displayID) {
    IOAVServiceRef result = NULL;
    CFDictionaryRef displayInfo =
        CoreDisplay_DisplayCreateInfoDictionary(displayID);
    if (displayInfo == NULL) {
        return IOAVServiceCreate(NULL);
    }

    CFStringRef displayLocation = (CFStringRef)CFDictionaryGetValue(
        displayInfo, CFSTR("IODisplayLocation"));
    if (displayLocation == NULL ||
        CFGetTypeID(displayLocation) != CFStringGetTypeID()) {
        CFRelease(displayInfo);
        return IOAVServiceCreate(NULL);
    }

    io_registry_entry_t adapter = IORegistryEntryCopyFromPath(
        kIOMainPortDefault, displayLocation);
    uint64_t adapterID = 0;
    if (adapter == IO_OBJECT_NULL ||
        IORegistryEntryGetRegistryEntryID(adapter, &adapterID) != KERN_SUCCESS) {
        if (adapter != IO_OBJECT_NULL) {
            IOObjectRelease(adapter);
        }
        CFRelease(displayInfo);
        return IOAVServiceCreate(NULL);
    }

    io_registry_entry_t root = IORegistryGetRootEntry(kIOMainPortDefault);
    io_iterator_t iterator = IO_OBJECT_NULL;
    kern_return_t iteratorResult = IORegistryEntryCreateIterator(
        root, kIOServicePlane, kIORegistryIterateRecursively, &iterator);
    IOObjectRelease(root);
    if (iteratorResult == KERN_SUCCESS) {
        Boolean framebufferMatchesDisplay = false;
        io_service_t service;
        while ((service = IOIteratorNext(iterator)) != IO_OBJECT_NULL) {
            if (IOObjectConformsTo(service, "IOMobileFramebuffer")) {
                uint64_t framebufferID = 0;
                framebufferMatchesDisplay =
                    IORegistryEntryGetRegistryEntryID(service, &framebufferID) == KERN_SUCCESS &&
                    framebufferID == adapterID;
                IOObjectRelease(service);
                continue;
            }

            io_name_t name = {0};
            IORegistryEntryGetName(service, name);
            if (!framebufferMatchesDisplay || strcmp(name, "DCPAVServiceProxy") != 0) {
                IOObjectRelease(service);
                continue;
            }

            CFTypeRef location = IORegistryEntrySearchCFProperty(
                service, kIOServicePlane, CFSTR("Location"),
                kCFAllocatorDefault, kIORegistryIterateRecursively);
            Boolean isExternal = location != NULL &&
                CFGetTypeID(location) == CFStringGetTypeID() &&
                CFStringCompare((CFStringRef)location, CFSTR("External"), 0) ==
                    kCFCompareEqualTo;
            if (location != NULL) {
                CFRelease(location);
            }
            if (isExternal) {
                result = IOAVServiceCreateWithService(NULL, service);
            }
            IOObjectRelease(service);
            if (result != NULL) {
                break;
            }
        }
        IOObjectRelease(iterator);
    }

    IOObjectRelease(adapter);
    CFRelease(displayInfo);
    return result != NULL ? result : IOAVServiceCreate(NULL);
}

static int monux_display_service_is_null(IOAVServiceRef service) {
    return service == NULL;
}

static int monux_external_display_at(uint32_t externalIndex,
    uint32_t *displayID, uint32_t *vendor, uint32_t *model) {
    CGDirectDisplayID displays[32];
    uint32_t count = 0;
    if (CGGetOnlineDisplayList(32, displays, &count) != kCGErrorSuccess) {
        return 0;
    }
    uint32_t currentIndex = 0;
    for (uint32_t i = 0; i < count; i++) {
        if (CGDisplayIsBuiltin(displays[i])) {
            continue;
        }
        if (currentIndex++ != externalIndex) {
            continue;
        }
        *displayID = displays[i];
        *vendor = CGDisplayVendorNumber(displays[i]);
        *model = CGDisplayModelNumber(displays[i]);
        return 1;
    }
    return 0;
}

static int monux_external_display_details(uint32_t displayID,
    uint32_t *vendor, uint32_t *model) {
    CGDirectDisplayID displays[32];
    uint32_t count = 0;
    if (CGGetOnlineDisplayList(32, displays, &count) != kCGErrorSuccess) {
        return 0;
    }
    for (uint32_t i = 0; i < count; i++) {
        if (displays[i] == displayID && !CGDisplayIsBuiltin(displays[i])) {
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/dyike/monux/internal/ddc"
)

type NativeBackend struct {
	selected  bool
	displayID uint32
	mu        sync.Mutex
	sleep     func(time.Duration)
}

func NewNativeBackend(id string) (Backend, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return &NativeBackend{sleep: time.Sleep}, nil
	}
	displayID, err := strconv.ParseUint(trimmed, 10, 32)
	if err != nil || displayID == 0 {
		return nil, fmt.Errorf("invalid macOS monitor id %q: use the display ID from monux detect", id)
	}
	return &NativeBackend{selected: true, displayID: uint32(displayID), sleep: time.Sleep}, nil
}

func (b *NativeBackend) Detect() ([]Display, error) {
	displays := make([]Display, 0)
	for index := uint32(0); index < 32; index++ {
		var displayID, vendor, model C.uint32_t
		if C.monux_external_display_at(C.uint32_t(index), &displayID, &vendor, &model) == 0 {
			break
		}
		displays = append(displays, Display{
			ID:   strconv.FormatUint(uint64(displayID), 10),
			Name: fmt.Sprintf("display %d (vendor 0x%04x, model 0x%04x)", uint32(displayID), uint32(vendor), uint32(model)),
		})
	}
	sort.Slice(displays, func(i, j int) bool {
		left, _ := strconv.ParseUint(displays[i].ID, 10, 32)
		right, _ := strconv.ParseUint(displays[j].ID, 10, 32)
		return left < right
	})
	return displays, nil
}

func (b *NativeBackend) CurrentInput() (Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.requireSelection(); err != nil {
		return 0, err
	}
	service, err := openDarwinDisplayService(b.displayID)
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
	service, err := openDarwinDisplayService(b.displayID)
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
	service, err := openDarwinDisplayService(b.displayID)
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

func openDarwinDisplayService(displayID uint32) (C.IOAVServiceRef, error) {
	var vendor, model C.uint32_t
	if C.monux_external_display_details(C.uint32_t(displayID), &vendor, &model) == 0 {
		var zero C.IOAVServiceRef
		return zero, fmt.Errorf("configured macOS display %d is not an online external display", displayID)
	}
	service := C.monux_open_display_service(C.uint32_t(displayID))
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
