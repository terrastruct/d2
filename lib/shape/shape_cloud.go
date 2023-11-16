package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
)

// The percentage values of the cloud's wide inner box
const CLOUD_WIDE_INNER_X = 0.085
const CLOUD_WIDE_INNER_Y = 0.409
const CLOUD_WIDE_INNER_WIDTH = 0.819
const CLOUD_WIDE_INNER_HEIGHT = 0.548
const CLOUD_WIDE_ASPECT_BOUNDARY = (1 + CLOUD_WIDE_INNER_WIDTH/CLOUD_WIDE_INNER_HEIGHT) / 2

// The percentage values of the cloud's tall inner box
const CLOUD_TALL_INNER_X = 0.228
const CLOUD_TALL_INNER_Y = 0.179
const CLOUD_TALL_INNER_WIDTH = 0.549
const CLOUD_TALL_INNER_HEIGHT = 0.820
const CLOUD_TALL_ASPECT_BOUNDARY = (1 + CLOUD_TALL_INNER_WIDTH/CLOUD_TALL_INNER_HEIGHT) / 2

// The percentage values of the cloud's square inner box
const CLOUD_SQUARE_INNER_X = 0.167
const CLOUD_SQUARE_INNER_Y = 0.335
const CLOUD_SQUARE_INNER_WIDTH = 0.663
const CLOUD_SQUARE_INNER_HEIGHT = 0.663

type shapeCloud struct {
	*baseShape
}

func NewCloud(box *geo.Box) Shape {
	shape := shapeCloud{
		baseShape: &baseShape{
			Type: CLOUD_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

// TODO this isn't always accurate since the content aspect ratio might be different from the final shape's https://github.com/terrastruct/d2/issues/1735
func (s shapeCloud) GetInnerBox() *geo.Box {
	width := s.Box.Width
	height := s.Box.Height
	insideTL := s.GetInsidePlacement(width, height, 0, 0)
	aspectRatio := width / height
	if aspectRatio > CLOUD_WIDE_ASPECT_BOUNDARY {
		width *= CLOUD_WIDE_INNER_WIDTH
		height *= CLOUD_WIDE_INNER_HEIGHT
	} else if aspectRatio < CLOUD_TALL_ASPECT_BOUNDARY {
		width *= CLOUD_TALL_INNER_WIDTH
		height *= CLOUD_TALL_INNER_HEIGHT
	} else {
		width *= CLOUD_SQUARE_INNER_WIDTH
		height *= CLOUD_SQUARE_INNER_HEIGHT
	}
	return geo.NewBox(&insideTL, width, height)
}

func (s shapeCloud) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	width += paddingX
	height += paddingY
	aspectRatio := width / height
	// use the inner box with the closest aspect ratio (wide, tall, or square box)
	if aspectRatio > CLOUD_WIDE_ASPECT_BOUNDARY {
		return math.Ceil(width / CLOUD_WIDE_INNER_WIDTH), math.Ceil(height / CLOUD_WIDE_INNER_HEIGHT)
	} else if aspectRatio < CLOUD_TALL_ASPECT_BOUNDARY {
		return math.Ceil(width / CLOUD_TALL_INNER_WIDTH), math.Ceil(height / CLOUD_TALL_INNER_HEIGHT)
	} else {
		return math.Ceil(width / CLOUD_SQUARE_INNER_WIDTH), math.Ceil(height / CLOUD_SQUARE_INNER_HEIGHT)
	}
}

func (s shapeCloud) GetInsidePlacement(width, height, paddingX, paddingY float64) geo.Point {
	r := s.Box
	width += paddingX
	height += paddingY
	aspectRatio := width / height
	if aspectRatio > CLOUD_WIDE_ASPECT_BOUNDARY {
		return *geo.NewPoint(r.TopLeft.X+math.Ceil(r.Width*CLOUD_WIDE_INNER_X+paddingX/2), r.TopLeft.Y+math.Ceil(r.Height*CLOUD_WIDE_INNER_Y+paddingY/2))
	} else if aspectRatio < CLOUD_TALL_ASPECT_BOUNDARY {
		return *geo.NewPoint(r.TopLeft.X+math.Ceil(r.Width*CLOUD_TALL_INNER_X+paddingX/2), r.TopLeft.Y+math.Ceil(r.Height*CLOUD_TALL_INNER_Y+paddingY/2))
	} else {
		return *geo.NewPoint(r.TopLeft.X+math.Ceil(r.Width*CLOUD_SQUARE_INNER_X+paddingX/2), r.TopLeft.Y+math.Ceil(r.Height*CLOUD_SQUARE_INNER_Y+paddingY/2))
	}
}

func cloudPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width/834, box.Height/523)
	// Note: original path TopLeft=(83, 238), absolute values updated so top left is at 0,0
	pc.StartAt(pc.Absolute(137.833, 182.833))
	pc.C(true, 0, 5.556, -5.556, 11.111, -11.111, 11.111)
	pc.C(true, -70.833, 6.944, -126.389, 77.778, -126.389, 163.889)
	pc.C(true, 0, 91.667, 62.5, 165.278, 141.667, 165.278)
	pc.H(true, 537.5)
	pc.C(true, 84.723, 0, 154.167, -79.167, 154.167, -175)
	pc.C(true, 0, -91.667, -63.89, -168.056, -144.444, -173.611)
	pc.C(true, -5.556, 0, -11.111, -4.167, -12.5, -11.111)
	pc.C(true, -18.056, -93.055, -101.39, -162.5, -198.611, -162.5)
	pc.C(true, -63.889, 0, -120.834, 29.167, -156.944, 75)
	pc.C(true, -4.167, 5.556, -11.111, 6.945, -15.278, 5.556)
	pc.C(true, -13.889, -5.556, -29.166, -8.333, -45.833, -8.333)
	pc.C(false, 196.167, 71.722, 143.389, 120.333, 137.833, 182.833)
	pc.Z()
	return pc
}

func (s shapeCloud) Perimeter() []geo.Intersectable {
	return cloudPath(s.Box).Path
}

func (s shapeCloud) GetSVGPathData() []string {
	return []string{
		cloudPath(s.Box).PathData(),
	}
}

func (s shapeCloud) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding, defaultPadding / 2
}
