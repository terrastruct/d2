package d2scene

import "fmt"

// AspectRatioMatrix maps source coordinates into destination using SVG's
// preserveAspectRatio rules. Source must have positive dimensions;
// destination may be empty so zero-sized image primitives remain valid and
// paint no pixels.
func AspectRatioMatrix(source, destination Box, aspect AspectRatio) (Matrix, error) {
	if err := validateBox(source); err != nil || source.Width == 0 || source.Height == 0 {
		return Matrix{}, fmt.Errorf("d2scene: aspect-ratio source box must have finite positive dimensions")
	}
	if err := validateBox(destination); err != nil {
		return Matrix{}, fmt.Errorf("d2scene: aspect-ratio destination box must have finite non-negative dimensions")
	}
	if aspect.Align > AlignXMaxYMax || aspect.Fit > AspectSlice {
		return Matrix{}, fmt.Errorf("d2scene: invalid aspect-ratio policy align=%d fit=%d", aspect.Align, aspect.Fit)
	}

	scaleX := destination.Width / source.Width
	scaleY := destination.Height / source.Height
	if !finite(scaleX) || !finite(scaleY) {
		return Matrix{}, fmt.Errorf("d2scene: aspect-ratio scale is non-finite")
	}
	offsetX, offsetY := destination.X, destination.Y
	if aspect.Align != AlignNone {
		scale := scaleX
		if scaleY < scale {
			scale = scaleY
		}
		if aspect.Fit == AspectSlice && scaleY > scaleX {
			scale = scaleY
		} else if aspect.Fit == AspectSlice {
			scale = scaleX
		}
		scaleX, scaleY = scale, scale
		alignX, alignY := aspectAlignmentFractions(aspect.Align)
		// Viewport fitting commonly subtracts nearly equal dimensions. Round
		// each product explicitly so fused multiply-adds cannot introduce
		// CPU-dependent residuals into retained SVG transforms. This fixes the
		// arithmetic without quantizing authored nested transforms, whose tiny
		// coefficients can become significant under an enclosing scale.
		extraX := destination.Width - float64(source.Width*scale)
		extraY := destination.Height - float64(source.Height*scale)
		offsetX = float64(destination.X + float64(extraX*alignX))
		offsetY = float64(destination.Y + float64(extraY*alignY))
	}
	matrix := Matrix{
		A: scaleX,
		D: scaleY,
		E: offsetX - float64(source.X*scaleX),
		F: offsetY - float64(source.Y*scaleY),
	}
	if !matrix.IsFinite() {
		return Matrix{}, fmt.Errorf("d2scene: aspect-ratio transform is non-finite")
	}
	return matrix, nil
}

func aspectAlignmentFractions(align AspectAlign) (float64, float64) {
	switch align {
	case AlignXMinYMin:
		return 0, 0
	case AlignXMidYMin:
		return .5, 0
	case AlignXMaxYMin:
		return 1, 0
	case AlignXMinYMid:
		return 0, .5
	case AlignXMidYMid:
		return .5, .5
	case AlignXMaxYMid:
		return 1, .5
	case AlignXMinYMax:
		return 0, 1
	case AlignXMidYMax:
		return .5, 1
	case AlignXMaxYMax:
		return 1, 1
	default:
		return 0, 0
	}
}
