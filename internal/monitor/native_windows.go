//go:build windows

package monitor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/dyike/monux/internal/ddc"
)

var (
	user32DLL = syscall.NewLazyDLL("user32.dll")
	dxva2DLL  = syscall.NewLazyDLL("dxva2.dll")

	procEnumDisplayMonitors             = user32DLL.NewProc("EnumDisplayMonitors")
	procGetNumberOfPhysicalMonitors     = dxva2DLL.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitors             = dxva2DLL.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procDestroyPhysicalMonitor          = dxva2DLL.NewProc("DestroyPhysicalMonitor")
	procGetVCPFeatureAndVCPFeatureReply = dxva2DLL.NewProc("GetVCPFeatureAndVCPFeatureReply")
	procSetVCPFeature                   = dxva2DLL.NewProc("SetVCPFeature")
	procGetCapabilitiesStringLength     = dxva2DLL.NewProc("GetCapabilitiesStringLength")
	procCapabilitiesRequestAndReply     = dxva2DLL.NewProc("CapabilitiesRequestAndCapabilitiesReply")
)

type physicalMonitor struct {
	handle      syscall.Handle
	description [128]uint16
}

type NativeBackend struct {
	index int
}

func NewNativeBackend(id string) (Backend, error) {
	index := 0
	if strings.TrimSpace(id) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(id))
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid Windows monitor id %q: use the number from monux detect", id)
		}
		index = parsed
	}
	return &NativeBackend{index: index}, nil
}

func (b *NativeBackend) Detect() ([]Display, error) {
	monitors, err := enumeratePhysicalMonitors()
	if err != nil {
		return nil, err
	}
	defer destroyPhysicalMonitors(monitors)

	displays := make([]Display, 0, len(monitors))
	for i := range monitors {
		displays = append(displays, Display{
			ID:   strconv.Itoa(i + 1),
			Name: syscall.UTF16ToString(monitors[i].description[:]),
		})
	}
	return displays, nil
}

func (b *NativeBackend) CurrentInput() (Input, error) {
	var input Input
	err := b.withMonitor(func(monitor physicalMonitor) error {
		var current, maximum uint32
		result, _, callErr := procGetVCPFeatureAndVCPFeatureReply.Call(
			uintptr(monitor.handle),
			uintptr(ddc.VCPInputSource),
			0,
			uintptr(unsafe.Pointer(&current)),
			uintptr(unsafe.Pointer(&maximum)),
		)
		if result == 0 {
			return windowsCallError("read VCP input source", callErr)
		}
		if current > 0xffff {
			return fmt.Errorf("monitor returned out-of-range input value %d", current)
		}
		input = Input(current)
		return nil
	})
	return input, err
}

func (b *NativeBackend) SetInput(input Input) error {
	return b.withMonitor(func(monitor physicalMonitor) error {
		result, _, callErr := procSetVCPFeature.Call(
			uintptr(monitor.handle),
			uintptr(ddc.VCPInputSource),
			uintptr(input),
		)
		if result == 0 {
			return windowsCallError("set VCP input source", callErr)
		}
		return nil
	})
}

func (b *NativeBackend) SupportedInputs() ([]Input, error) {
	var inputs []Input
	err := b.withMonitor(func(monitor physicalMonitor) error {
		var length uint32
		result, _, callErr := procGetCapabilitiesStringLength.Call(
			uintptr(monitor.handle),
			uintptr(unsafe.Pointer(&length)),
		)
		if result == 0 {
			return windowsCallError("get monitor capabilities string length", callErr)
		}
		if length == 0 || length > 64*1024 {
			return fmt.Errorf("monitor returned invalid capabilities string length %d", length)
		}
		buffer := make([]byte, length)
		result, _, callErr = procCapabilitiesRequestAndReply.Call(
			uintptr(monitor.handle),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(length),
		)
		if result == 0 {
			return windowsCallError("read monitor capabilities string", callErr)
		}
		if end := strings.IndexByte(string(buffer), 0); end >= 0 {
			buffer = buffer[:end]
		}
		parsed, err := inputsFromCapabilities(string(buffer))
		if err != nil {
			return fmt.Errorf("parse monitor input capabilities: %w", err)
		}
		inputs = parsed
		return nil
	})
	return inputs, err
}

func (b *NativeBackend) withMonitor(action func(physicalMonitor) error) error {
	if b.index <= 0 {
		return errors.New("monitor.id is required; run monux detect and configure its monitor number")
	}
	monitors, err := enumeratePhysicalMonitors()
	if err != nil {
		return err
	}
	defer destroyPhysicalMonitors(monitors)
	if b.index > len(monitors) {
		return fmt.Errorf("monitor id %d not found (detected %d physical monitors)", b.index, len(monitors))
	}
	return action(monitors[b.index-1])
}

func enumeratePhysicalMonitors() ([]physicalMonitor, error) {
	var monitors []physicalMonitor
	var callbackErr error
	callback := syscall.NewCallback(func(hMonitor, _ uintptr, _ uintptr, _ uintptr) uintptr {
		var count uint32
		result, _, callErr := procGetNumberOfPhysicalMonitors.Call(
			hMonitor,
			uintptr(unsafe.Pointer(&count)),
		)
		if result == 0 {
			callbackErr = windowsCallError("get physical monitor count", callErr)
			return 0
		}
		if count == 0 {
			return 1
		}
		batch := make([]physicalMonitor, count)
		result, _, callErr = procGetPhysicalMonitors.Call(
			hMonitor,
			uintptr(count),
			uintptr(unsafe.Pointer(&batch[0])),
		)
		if result == 0 {
			callbackErr = windowsCallError("get physical monitors", callErr)
			return 0
		}
		monitors = append(monitors, batch...)
		return 1
	})

	result, _, callErr := procEnumDisplayMonitors.Call(0, 0, callback, 0)
	if callbackErr != nil {
		destroyPhysicalMonitors(monitors)
		return nil, callbackErr
	}
	if result == 0 {
		destroyPhysicalMonitors(monitors)
		return nil, windowsCallError("enumerate display monitors", callErr)
	}
	return monitors, nil
}

func destroyPhysicalMonitors(monitors []physicalMonitor) {
	for _, monitor := range monitors {
		if monitor.handle != 0 {
			procDestroyPhysicalMonitor.Call(uintptr(monitor.handle))
		}
	}
}

func windowsCallError(action string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return errors.New(action + " failed")
	}
	return fmt.Errorf("%s: %w", action, err)
}
