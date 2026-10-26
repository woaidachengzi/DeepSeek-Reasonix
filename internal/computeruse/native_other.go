//go:build (!darwin && !linux) || (darwin && !cgo)

package computeruse

import (
	"context"
	"errors"
)

type nativeBackend struct{}

func Available() bool { return false }
func (nativeBackend) call(context.Context, Request) (nativeResult, error) {
	return nativeResult{}, errors.New("computer_use requires macOS and a build with CGO_ENABLED=1")
}
