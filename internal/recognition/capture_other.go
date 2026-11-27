//go:build !windows || (!amd64 && !arm64)

package recognition

import (
	"context"
	"errors"
)

const Available = false

func Capture(context.Context) ([]float32, int, error) {
	return nil, 0, errors.New("system audio capture is unavailable on this platform")
}
