//go:build linux

package monitor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dyike/monux/internal/ddc"
)

const (
	i2cRetries = 0x0701
	i2cTimeout = 0x0702
	i2cSlave   = 0x0703
)

type NativeBackend struct {
	bus   int
	mu    sync.Mutex
	sleep func(time.Duration)
}

func NewNativeBackend(id string) (Backend, error) {
	bus := -1
	if strings.TrimSpace(id) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(id))
		if err != nil || parsed < 0 {
			return nil, fmt.Errorf("invalid Linux monitor id %q: use the I2C bus number from monux detect", id)
		}
		bus = parsed
	}
	return &NativeBackend{bus: bus, sleep: time.Sleep}, nil
}

func (b *NativeBackend) Detect() ([]Display, error) {
	return discoverLinuxDisplays("/sys/class/drm", "/sys/class/i2c-dev")
}

func (b *NativeBackend) CurrentInput() (Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	file, err := b.openBus()
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := file.Write(ddc.GetVCPRequest(ddc.VCPInputSource)); err != nil {
			lastErr = fmt.Errorf("write request: %w", err)
			continue
		}
		b.sleep(40 * time.Millisecond)

		reply := make([]byte, 11)
		if _, err := io.ReadFull(file, reply); err != nil {
			lastErr = fmt.Errorf("read reply: %w", err)
			continue
		}
		value, err := ddc.ParseVCPReply(reply, ddc.VCPInputSource)
		if err == nil {
			return Input(value.Current), nil
		}
		lastErr = err
		b.sleep(100 * time.Millisecond)
	}
	return 0, fmt.Errorf("read input source failed after 3 attempts on bus %d: %w", b.bus, lastErr)
}

func (b *NativeBackend) SetInput(input Input) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	file, err := b.openBus()
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(ddc.SetVCPRequest(ddc.VCPInputSource, uint16(input))); err != nil {
		return fmt.Errorf("write input-source request to bus %d: %w", b.bus, err)
	}
	b.sleep(50 * time.Millisecond)
	return nil
}

func (b *NativeBackend) SupportedInputs() ([]Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	file, err := b.openBus()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	capabilities, err := ddc.ReadCapabilities(func(offset uint16) ([]byte, error) {
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			if _, err := file.Write(ddc.CapabilitiesRequest(offset)); err != nil {
				lastErr = fmt.Errorf("write request: %w", err)
				continue
			}
			// Capabilities replies take substantially longer than ordinary VCP
			// replies on many monitors.
			b.sleep(200 * time.Millisecond)

			reply := make([]byte, 38)
			count, err := file.Read(reply)
			if err != nil {
				lastErr = fmt.Errorf("read reply: %w", err)
				continue
			}
			fragment, err := ddc.ParseCapabilitiesReply(reply[:count], offset)
			if err == nil {
				b.sleep(50 * time.Millisecond)
				return fragment, nil
			}
			lastErr = err
			b.sleep(100 * time.Millisecond)
		}
		return nil, fmt.Errorf("capabilities offset %d failed after 3 attempts on bus %d: %w", offset, b.bus, lastErr)
	})
	if err != nil {
		b.sleep(200 * time.Millisecond)
		return nil, err
	}
	inputs, err := inputsFromCapabilities(capabilities)
	if err != nil {
		return nil, fmt.Errorf("parse input capabilities from bus %d: %w", b.bus, err)
	}
	return inputs, nil
}

func (b *NativeBackend) openBus() (*os.File, error) {
	if b.bus < 0 {
		return nil, errors.New("monitor.id is required; run monux detect and configure its I2C bus number")
	}
	path := fmt.Sprintf("/dev/i2c-%d", b.bus)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open monitor I2C bus %d: %w", b.bus, err)
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), i2cSlave, uintptr(ddc.DisplayAddress))
	if errno != 0 {
		file.Close()
		return nil, fmt.Errorf("select DDC/CI address 0x%02x on %s: %w", ddc.DisplayAddress, path, errno)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), i2cTimeout, 100); errno != 0 {
		file.Close()
		return nil, fmt.Errorf("set 1 second I2C timeout on %s: %w", path, errno)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), i2cRetries, 2); errno != 0 {
		file.Close()
		return nil, fmt.Errorf("set I2C retries on %s: %w", path, errno)
	}
	return file, nil
}

func discoverLinuxDisplays(drmRoot, i2cRoot string) ([]Display, error) {
	paths, err := filepath.Glob(filepath.Join(drmRoot, "card*-*", "ddc", "i2c-dev", "i2c-*"))
	if err != nil {
		return nil, err
	}
	displays := make([]Display, 0, len(paths))
	seen := make(map[string]bool)
	for _, path := range paths {
		connectorDir := filepath.Dir(filepath.Dir(filepath.Dir(path)))
		status, err := os.ReadFile(filepath.Join(connectorDir, "status"))
		if err == nil && strings.TrimSpace(string(status)) != "connected" {
			continue
		}
		id := strings.TrimPrefix(filepath.Base(path), "i2c-")
		if _, err := strconv.Atoi(id); err != nil || seen[id] {
			continue
		}
		seen[id] = true
		name := filepath.Base(connectorDir)
		if edid, err := os.ReadFile(filepath.Join(connectorDir, "edid")); err == nil {
			if model := monitorNameFromEDID(edid); model != "" {
				name += " (" + model + ")"
			}
		}
		displays = append(displays, Display{ID: id, Name: name})
	}

	if len(displays) == 0 {
		fallback, err := filepath.Glob(filepath.Join(i2cRoot, "i2c-*"))
		if err != nil {
			return nil, err
		}
		for _, path := range fallback {
			id := strings.TrimPrefix(filepath.Base(path), "i2c-")
			if _, err := strconv.Atoi(id); err == nil && !seen[id] {
				displays = append(displays, Display{ID: id, Name: "I2C adapter " + id})
			}
		}
	}
	sort.Slice(displays, func(i, j int) bool {
		left, _ := strconv.Atoi(displays[i].ID)
		right, _ := strconv.Atoi(displays[j].ID)
		return left < right
	})
	return displays, nil
}

func monitorNameFromEDID(edid []byte) string {
	if len(edid) < 126 {
		return ""
	}
	for offset := 54; offset+18 <= 126; offset += 18 {
		descriptor := edid[offset : offset+18]
		if descriptor[0] == 0 && descriptor[1] == 0 && descriptor[2] == 0 && descriptor[3] == 0xfc {
			return strings.TrimSpace(strings.TrimRight(string(descriptor[5:18]), "\x00\n\r "))
		}
	}
	return ""
}
