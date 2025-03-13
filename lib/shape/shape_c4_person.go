package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
)

// Optimal value for circular arc approximation with cubic bezier curves
const kCircleApprox = 0.5522847498307936 // 4*(math.Sqrt(2)-1)/3

type shapeC4Person struct {
	*baseShape
}

func NewC4Person(box *geo.Box) Shape {
	shape := shapeC4Person{
		baseShape: &baseShape{
			Type: C4_PERSON_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

const (
	C4_PERSON_AR_LIMIT = 1.5
)

func (s shapeC4Person) GetInnerBox() *geo.Box {
	width := s.Box.Width

	// Reduce head radius from 22% to 18% of width
	headRadius := width * 0.18
	headCenterY := headRadius
	bodyTop := headCenterY + headRadius*0.8

	// Horizontal padding = 5% of width
	horizontalPadding := width * 0.05

	// Vertical padding = 3% of width (not height)
	vertPadding := width * 0.03

	tl := s.Box.TopLeft.Copy()
	tl.X += horizontalPadding
	tl.Y += bodyTop + vertPadding

	innerWidth := width - (horizontalPadding * 2)
	innerHeight := s.Box.Height - bodyTop - (vertPadding * 2)

	return geo.NewBox(tl, innerWidth, innerHeight)
}

func bodyPath(box *geo.Box) *svg.SvgPathContext {
	width := box.Width
	height := box.Height

	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)

	// Reduce head radius from 22% to 18% of width
	headRadius := width * 0.18
	// Adjust headCenterY to ensure head fits within box
	headCenterY := headRadius
	bodyTop := headCenterY + headRadius*0.8
	bodyWidth := width
	bodyHeight := height - bodyTop
	bodyLeft := 0
	// Ensure cornerRadius is constrained to a portion of the shortest dimension
	// This prevents distorted corners when width is large compared to height
	cornerRadius := math.Min(width*0.175, bodyHeight*0.25)

	pc.StartAt(pc.Absolute(float64(bodyLeft), bodyTop+cornerRadius))

	pc.C(true, 0, -kCircleApprox*cornerRadius, kCircleApprox*cornerRadius, -cornerRadius, cornerRadius, -cornerRadius)
	pc.H(true, bodyWidth-2*cornerRadius)
	pc.C(true, kCircleApprox*cornerRadius, 0, cornerRadius, kCircleApprox*cornerRadius, cornerRadius, cornerRadius)
	pc.V(true, bodyHeight-2*cornerRadius)
	pc.C(true, 0, kCircleApprox*cornerRadius, -kCircleApprox*cornerRadius, cornerRadius, -cornerRadius, cornerRadius)
	pc.H(true, -(bodyWidth - 2*cornerRadius))
	pc.C(true, -kCircleApprox*cornerRadius, 0, -cornerRadius, -kCircleApprox*cornerRadius, -cornerRadius, -cornerRadius)
	pc.Z()

	return pc
}

func headPath(box *geo.Box) *svg.SvgPathContext {
	width := box.Width
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)

	// Reduce head radius from 22% to 18% of width
	headRadius := width * 0.18
	headCenterX := width / 2
	// Adjust headCenterY to ensure head fits within box
	headCenterY := headRadius

	pc.StartAt(pc.Absolute(headCenterX, headCenterY-headRadius))

	pc.C(false,
		headCenterX+headRadius*kCircleApprox, headCenterY-headRadius,
		headCenterX+headRadius, headCenterY-headRadius*kCircleApprox,
		headCenterX+headRadius, headCenterY)

	pc.C(false,
		headCenterX+headRadius, headCenterY+headRadius*kCircleApprox,
		headCenterX+headRadius*kCircleApprox, headCenterY+headRadius,
		headCenterX, headCenterY+headRadius)

	pc.C(false,
		headCenterX-headRadius*kCircleApprox, headCenterY+headRadius,
		headCenterX-headRadius, headCenterY+headRadius*kCircleApprox,
		headCenterX-headRadius, headCenterY)

	pc.C(false,
		headCenterX-headRadius, headCenterY-headRadius*kCircleApprox,
		headCenterX-headRadius*kCircleApprox, headCenterY-headRadius,
		headCenterX, headCenterY-headRadius)

	return pc
}

func (s shapeC4Person) Perimeter() []geo.Intersectable {
	width := s.Box.Width

	bodyPerimeter := bodyPath(s.Box).Path

	// Reduce head radius from 22% to 18% of width
	headRadius := width * 0.18
	headCenterX := s.Box.TopLeft.X + width/2
	headCenterY := s.Box.TopLeft.Y + headRadius
	headCenter := geo.NewPoint(headCenterX, headCenterY)

	headEllipse := geo.NewEllipse(headCenter, headRadius, headRadius)

	return append(bodyPerimeter, headEllipse)
}

func (s shapeC4Person) GetSVGPathData() []string {
	return []string{
		bodyPath(s.Box).PathData(),
		headPath(s.Box).PathData(),
	}
}

func (s shapeC4Person) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	// Start with content dimensions + padding
	totalWidth := width + paddingX
	totalHeight := height + paddingY

	// Reduce head radius from 22% to 18% of width
	headRadius := totalWidth * 0.18

	// Head center is now at headRadius from the top
	headCenterY := headRadius

	// Body starts at headCenterY + headRadius*0.8
	bodyTopPosition := headCenterY + headRadius*0.8

	// Add space for body (head is now fully contained)
	totalHeight += bodyTopPosition

	// Horizontal padding is handled in GetInnerBox (5% on each side)
	totalWidth /= 0.9

	// Prevent the shape's aspect ratio from becoming too extreme
	totalWidth, totalHeight = LimitAR(totalWidth, totalHeight, C4_PERSON_AR_LIMIT)

	return math.Ceil(totalWidth), math.Ceil(totalHeight)
}

func (s shapeC4Person) GetDefaultPadding() (paddingX, paddingY float64) {
	return 10, defaultPadding
}
