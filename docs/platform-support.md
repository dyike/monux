# Platform support

This document separates implemented code from hardware-validated behavior.
DDC/CI support varies across GPUs, cables, docks, adapters, and monitor
firmware, so compilation alone is not considered platform validation.

## Linux

Status: implemented and unit-tested. Detection plus VCP `0x60` read/write have
been validated against a Dell P2415Q on the current Linux development device.
Native capabilities discovery reports `0x0f` (DisplayPort 1), `0x10`
(DisplayPort 2), and `0x11` (HDMI 1) without invoking an external
monitor-control binary.

- Enumerates connected DRM connectors and prefers their direct
  `i2c-*/i2c-dev/i2c-*` DisplayPort AUX adapter. It falls back to the connector
  `ddc/i2c-dev/i2c-*` path used by HDMI and older drivers.
- Reads the EDID monitor-name descriptor when available.
- Falls back to `/sys/class/i2c-dev/i2c-*` if DRM connector mapping is absent.
- Opens `/dev/i2c-N`, selects the 7-bit DDC address `0x37`, and sends native
  DDC/CI frames.
- Reads and validates Get VCP replies, including checksum and requested code.
- Reads offset-based DDC/CI capabilities fragments and extracts the monitor's
  declared VCP `0x60` input-source values.
- Writes VCP input source `0x60` directly.
- On the validated AMD/P2415Q setup, the hardware-I2C `ddc` symlink can read
  EDID but rejects DDC/CI writes; the connector-owned AUX adapter is required.
- The P2415Q rejects Linux DisplayPort DDC after HDMI becomes active, so
  switching back requires the active Mac peer rather than another local retry.

Risks and follow-up work:

- Add adapter capability checks and configurable retry/delay policies.
- Probe fallback I2C adapters safely instead of presenting every adapter as a
  probable monitor.
- Validate AMD, Intel, NVIDIA, DisplayLink, docks, and MST topologies.

## Windows

Status: implemented and successfully cross-compiled for amd64 and arm64;
physical Windows validation pending.

- Uses `EnumDisplayMonitors` to enumerate logical monitor handles.
- Resolves physical monitors with
  `GetNumberOfPhysicalMonitorsFromHMONITOR` and
  `GetPhysicalMonitorsFromHMONITOR`.
- Reads VCP `0x60` with `GetVCPFeatureAndVCPFeatureReply`.
- Reads the capabilities string with
  `GetCapabilitiesStringLength` and
  `CapabilitiesRequestAndCapabilitiesReply`.
- Writes VCP `0x60` with `SetVCPFeature`.
- Releases every physical monitor handle with `DestroyPhysicalMonitor`.

Risks and follow-up work:

- Validate duplicate physical monitors behind one logical display.
- Add stable identifiers beyond the current one-based detected index.
- Test Windows display-driver differences and sleep/wake handle changes.

## macOS

Status: hardware-validated on Apple Silicon with an external USB-C/DisplayPort
monitor connection.

- Uses CoreGraphics for external display detection.
- Matches the selected CoreGraphics display to its IOMobileFramebuffer and
  external `DCPAVServiceProxy` in the IOKit registry.
- Uses a small project-owned CGO bridge to IOKit `IOAVService` I2C calls.
- Uses the shared Go DDC/CI encoder and parser; it does not execute `m1ddc`.
- Reads DDC/CI capabilities fragments through the same IOKit transport.
- Enumerates every online external display and selects one by its CoreGraphics
  display ID as reported by `monux detect`.
- Requires `CGO_ENABLED=1` and Apple Command Line Tools to build.

Important constraints:

- `IOAVService` is not a stable public Apple API even though the transport is
  provided by the operating system. macOS updates can change its behavior.
- USB-C/DisplayPort Alt Mode is the first validation target. HDMI, docks, and
  adapters require separate testing.
- CoreGraphics display IDs can change after reconnecting a display or changing
  the display topology; rerun `monux detect` and update `monitor.id` when that
  happens.
- Intel macOS needs a separate IOFramebuffer/IOI2C implementation.

## Protocol core

`internal/ddc` is platform-independent and currently covers:

- Get VCP Feature request construction.
- Set VCP Feature request construction.
- XOR checksum generation.
- Get VCP Feature Reply validation and parsing.
- Capabilities Request construction, fragmented-reply validation, and
  capabilities-string reassembly.
- Extraction of supported values declared for VCP input source `0x60`.
- Known VESA packet examples as unit-test vectors.

Future protocol work includes EDID parsing beyond the Linux monitor-name
descriptor, retry classification, and monitor-specific quirks.
