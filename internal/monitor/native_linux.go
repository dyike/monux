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
	bus             int
	displayIdentity string
	drmRoot         string
	i2cRoot         string
	mu              sync.Mutex
	sleep           func(time.Duration)
}

func NewNativeBackend(id string) (Backend, error) {
	return newNativeBackend(id, "/sys/class/drm", "/sys/class/i2c-dev")
}

func newNativeBackend(id, drmRoot, i2cRoot string) (*NativeBackend, error) {
	bus := -1
	if strings.TrimSpace(id) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(id))
		if err != nil || parsed < 0 {
			return nil, fmt.Errorf("invalid Linux monitor id %q: use the I2C bus number from monux detect", id)
		}
		bus = parsed
	}
	backend := &NativeBackend{
		bus:     bus,
		drmRoot: drmRoot,
		i2cRoot: i2cRoot,
		sleep:   time.Sleep,
	}
	backend.captureDisplayIdentity()
	return backend, nil
}

func (b *NativeBackend) Detect() ([]Display, error) {
	return discoverLinuxDisplays(b.drmRoot, b.i2cRoot)
}

func (b *NativeBackend) CurrentInput() (Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	originalBus := b.bus
	input, err := b.currentInput()
	if err == nil {
		return input, nil
	}
	if rediscoveryErr := b.rediscoverBus(); rediscoveryErr != nil {
		return 0, fmt.Errorf("%w; automatic monitor bus rediscovery failed: %v", err, rediscoveryErr)
	}
	if b.bus == originalBus {
		return 0, err
	}
	input, retryErr := b.currentInput()
	if retryErr != nil {
		return 0, fmt.Errorf("read input source failed on configured bus %d and rediscovered bus %d: %w", originalBus, b.bus, retryErr)
	}
	return input, nil
}

func (b *NativeBackend) currentInput() (Input, error) {
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

	originalBus := b.bus
	err := b.setInput(input)
	if err == nil {
		return nil
	}
	if rediscoveryErr := b.rediscoverBus(); rediscoveryErr != nil {
		return fmt.Errorf("%w; automatic monitor bus rediscovery failed: %v", err, rediscoveryErr)
	}
	if b.bus == originalBus {
		return err
	}
	if retryErr := b.setInput(input); retryErr != nil {
		return fmt.Errorf("set input source failed on configured bus %d and rediscovered bus %d: %w", originalBus, b.bus, retryErr)
	}
	return nil
}

func (b *NativeBackend) setInput(input Input) error {
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

	originalBus := b.bus
	inputs, err := b.supportedInputs()
	if err == nil {
		return inputs, nil
	}
	if rediscoveryErr := b.rediscoverBus(); rediscoveryErr != nil {
		return nil, fmt.Errorf("%w; automatic monitor bus rediscovery failed: %v", err, rediscoveryErr)
	}
	if b.bus == originalBus {
		return nil, err
	}
	inputs, retryErr := b.supportedInputs()
	if retryErr != nil {
		return nil, fmt.Errorf("read input capabilities failed on configured bus %d and rediscovered bus %d: %w", originalBus, b.bus, retryErr)
	}
	return inputs, nil
}

func (b *NativeBackend) supportedInputs() ([]Input, error) {
	file, err := b.openBus()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	readFragment := func(offset uint16) ([]byte, error) {
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
		b.sleep(200 * time.Millisecond)
		return nil, fmt.Errorf("read complete monitor capabilities after 2 attempts: %w", err)
	}
	inputs, err := inputsFromCapabilities(capabilities)
	if err != nil {
		return nil, fmt.Errorf("parse input capabilities from bus %d: %w", b.bus, err)
	}
	return inputs, nil
}

func (b *NativeBackend) captureDisplayIdentity() {
	if b.bus < 0 {
		return
	}
	for _, candidate := range discoverLinuxDisplayCandidates(b.drmRoot) {
		if candidate.ID == strconv.Itoa(b.bus) {
			b.displayIdentity = candidate.identity
			return
		}
	}
}

// rediscoverBus refreshes the volatile Linux I2C adapter number after a DRM
// hotplug. DisplayPort MST commonly destroys and recreates its connector-owned
// AUX adapter, so a bus number that was valid when Monux started can later
// point at a dead transport. Prefer an EDID-identical display and only fall
// back to an unambiguous single connected display.
func (b *NativeBackend) rediscoverBus() error {
	candidates := discoverLinuxDisplayCandidates(b.drmRoot)
	if len(candidates) == 0 {
		return errors.New("no connected DRM display with an I2C adapter")
	}

	matches := candidates
	if b.displayIdentity != "" {
		matches = nil
		for _, candidate := range candidates {
			if candidate.identity == b.displayIdentity {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 0 {
			return errors.New("configured monitor is not among the connected DRM displays")
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("monitor I2C bus changed, but %d candidate displays matched; rerun monux init --monitor <id>", len(matches))
	}

	bus, err := strconv.Atoi(matches[0].ID)
	if err != nil {
		return fmt.Errorf("invalid rediscovered I2C bus %q: %w", matches[0].ID, err)
	}
	b.bus = bus
	if b.displayIdentity == "" {
		b.displayIdentity = matches[0].identity
	}
	return nil
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
	candidates := discoverLinuxDisplayCandidates(drmRoot)
	displays := make([]Display, 0, len(candidates))
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		displays = append(displays, candidate.Display)
		seen[candidate.ID] = true
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

type linuxDisplayCandidate struct {
	Display
	identity string
}

func discoverLinuxDisplayCandidates(drmRoot string) []linuxDisplayCandidate {
	connectors, err := filepath.Glob(filepath.Join(drmRoot, "card*-*"))
	if err != nil {
		return nil
	}
	displays := make([]linuxDisplayCandidate, 0, len(connectors))
	seen := make(map[string]bool)
	for _, connectorDir := range connectors {
		status, err := os.ReadFile(filepath.Join(connectorDir, "status"))
		if err == nil && strings.TrimSpace(string(status)) != "connected" {
			continue
		}
		id, ok := linuxI2CBusForConnector(connectorDir)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		name := filepath.Base(connectorDir)
		identity := ""
		if edid, err := os.ReadFile(filepath.Join(connectorDir, "edid")); err == nil {
			identity = string(edid)
			if model := monitorNameFromEDID(edid); model != "" {
				name += " (" + model + ")"
			}
		}
		displays = append(displays, linuxDisplayCandidate{
			Display:  Display{ID: id, Name: name},
			identity: identity,
		})
	}
	sort.Slice(displays, func(i, j int) bool {
		left, _ := strconv.Atoi(displays[i].ID)
		right, _ := strconv.Atoi(displays[j].ID)
		return left < right
	})
	return displays
}

func linuxI2CBusForConnector(connectorDir string) (string, bool) {
	// DisplayPort exposes its working DDC/CI transport as an I2C-over-AUX
	// adapter directly below the connector. Some drivers also publish a ddc
	// symlink to the matching hardware I2C bus, but that adapter can read EDID
	// while rejecting DDC/CI writes. Prefer the connector-owned AUX adapter and
	// retain the ddc path as the fallback used by HDMI and older drivers.
	patterns := []string{
		filepath.Join(connectorDir, "i2c-*", "i2c-dev", "i2c-*"),
		filepath.Join(connectorDir, "ddc", "i2c-dev", "i2c-*"),
	}
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		sort.Strings(paths)
		for _, path := range paths {
			id := strings.TrimPrefix(filepath.Base(path), "i2c-")
			if _, err := strconv.Atoi(id); err == nil {
				return id, true
			}
		}
	}
	return "", false
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
