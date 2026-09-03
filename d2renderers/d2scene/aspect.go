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
	if aspect.Align == AlignNone {
		matrix := Translate(destination.X, destination.Y).
			Mul(Scale(scaleX, scaleY)).
			Mul(Translate(-source.X, -source.Y))
		if !matrix.IsFinite() {
			return Matrix{}, fmt.Errorf("d2scene: aspect-ratio transform is non-finite")
		}
		return matrix, nil
	}

	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if aspect.Fit == AspectSlice && scaleY > scaleX {
		scale = scaleY
	} else if aspect.Fit == AspectSlice {
		scale = scaleX
	}
	alignX, alignY := aspectAlignmentFractions(aspect.Align)
	extraX := destination.Width - source.Width*scale
	extraY := destination.Height - source.Height*scale
	matrix := Translate(destination.X+extraX*alignX, destination.Y+extraY*alignY).
		Mul(Scale(scale, scale)).
		Mul(Translate(-source.X, -source.Y))
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
