# Platform support

This document separates implemented code from hardware-validated behavior.
DDC/CI support varies across GPUs, cables, docks, adapters, and monitor
firmware, so compilation alone is not considered platform validation.

## Linux

Status: implemented and unit-tested; physical monitor validation pending in
the current development environment.

- Enumerates connected DRM connectors through
  `/sys/class/drm/card*-*/ddc/i2c-dev/i2c-*`.
- Reads the EDID monitor-name descriptor when available.
- Falls back to `/sys/class/i2c-dev/i2c-*` if DRM connector mapping is absent.
- Opens `/dev/i2c-N`, selects the 7-bit DDC address `0x37`, and sends native
  DDC/CI frames.
- Reads and validates Get VCP replies, including checksum and requested code.
- Writes VCP input source `0x60` directly.

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
- Writes VCP `0x60` with `SetVCPFeature`.
- Releases every physical monitor handle with `DestroyPhysicalMonitor`.

Risks and follow-up work:

- Validate duplicate physical monitors behind one logical display.
- Add stable identifiers beyond the current one-based detected index.
- Test Windows display-driver differences and sleep/wake handle changes.

## macOS

Status: source implementation present; it must be compiled and validated on an
Apple Silicon Mac with the actual monitor connection.

- Uses CoreGraphics for external display detection.
- Uses a small project-owned CGO bridge to CoreDisplay `IOAVService` I2C calls.
- Uses the shared Go DDC/CI encoder and parser; it does not execute `m1ddc`.
- Currently supports a single external monitor selected as ID `1`.
- Requires `CGO_ENABLED=1` and Apple Command Line Tools to build.

Important constraints:

- `IOAVService` is not a stable public Apple API even though the transport is
  provided by the operating system. macOS updates can change its behavior.
- USB-C/DisplayPort Alt Mode is the first validation target. HDMI, docks, and
  adapters require separate testing.
- Multiple-display service-to-display matching requires additional IOKit
  registry work before it can be considered supported.
- Intel macOS needs a separate IOFramebuffer/IOI2C implementation.

## Protocol core

`internal/ddc` is platform-independent and currently covers:

- Get VCP Feature request construction.
- Set VCP Feature request construction.
- XOR checksum generation.
- Get VCP Feature Reply validation and parsing.
- Known VESA packet examples as unit-test vectors.

Future protocol work includes capabilities requests, EDID parsing beyond the
Linux monitor-name descriptor, retry classification, and monitor-specific
quirks.
