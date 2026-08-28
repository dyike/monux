//go:build darwin && !cgo

package monitor

import "errors"

func NewNativeBackend(string) (Backend, error) {
	return nil, errors.New("native macOS monitor control requires CGO and the Apple Command Line Tools")
}
