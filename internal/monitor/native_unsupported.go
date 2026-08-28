//go:build !linux && !windows && !darwin

package monitor

import "fmt"

func NewNativeBackend(string) (Backend, error) {
	return nil, fmt.Errorf("native monitor control is not supported on this platform")
}
