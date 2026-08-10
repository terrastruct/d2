package testutil

import (
	"bytes"
	"fmt"
	"image/gif"
)

const infiniteLoop = 0

// ValidateGIF checks the frame count, loop behavior, dimensions, and interval
// of gifBytes against D2's expected GIF output.
func ValidateGIF(gifBytes []byte, nFrames int, intervalMS int) error {
	anim, err := gif.DecodeAll(bytes.NewBuffer(gifBytes))
	if err != nil {
		return err
	}

	if nFrames > 1 && anim.LoopCount != infiniteLoop {
		return fmt.Errorf("expected infinite loop, got=%d", anim.LoopCount)
	} else if nFrames == 1 && anim.LoopCount != -1 {
		return fmt.Errorf("wrong loop count for single frame gif, got=%d", anim.LoopCount)
	}

	if len(anim.Image) != nFrames {
		return fmt.Errorf("expected %d frames, got=%d", nFrames, len(anim.Image))
	}

	interval := intervalMS / 10
	width, height := anim.Config.Width, anim.Config.Height
	for i, frame := range anim.Image {
		w := frame.Bounds().Dx()
		if w != width {
			return fmt.Errorf("expected all frames to have the same width=%d, got=%d at frame=%d", width, w, i)
		}
		h := frame.Bounds().Dy()
		if h != height {
			return fmt.Errorf("expected all frames to have the same height=%d, got=%d at frame=%d", height, h, i)
		}
		if anim.Delay[i] != interval {
			return fmt.Errorf("expected interval between frames to be %d, got=%d at frame=%d", interval, anim.Delay[i], i)
		}
	}

	return nil
}
