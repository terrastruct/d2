package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
)

const (
	SQUARE_TYPE        = "Square"
	REAL_SQUARE_TYPE   = "RealSquare"
	PARALLELOGRAM_TYPE = "Parallelogram"
	DOCUMENT_TYPE      = "Document"
	CYLINDER_TYPE      = "Cylinder"
	QUEUE_TYPE         = "Queue"
	PAGE_TYPE          = "Page"
	PACKAGE_TYPE       = "Package"
	STEP_TYPE          = "Step"
	CALLOUT_TYPE       = "Callout"
	STORED_DATA_TYPE   = "StoredData"
	PERSON_TYPE        = "Person"
	C4_PERSON_TYPE     = "C4Person"
	DIAMOND_TYPE       = "Diamond"
	OVAL_TYPE          = "Oval"
	CIRCLE_TYPE        = "Circle"
	HEXAGON_TYPE       = "Hexagon"
	CLOUD_TYPE         = "Cloud"

	TABLE_TYPE = "Table"
	CLASS_TYPE = "Class"
	TEXT_TYPE  = "Text"
	CODE_TYPE  = "Code"
	IMAGE_TYPE = "Image"

	defaultPadding = 40.
)

type Shape interface {
	Is(shape string) bool
	GetType() string

	AspectRatio1() bool
	IsRectangular() bool

	GetBox() *geo.Box
	GetInnerBox() *geo.Box
	// cloud shape has different innerBoxes depending on content's aspect ratio
	GetInnerBoxForContent(width, height float64) *geo.Box
	SetInnerBoxAspectRatio(aspectRatio float64)

	// placing a rectangle of the given size and padding inside the shape, return the position relative to the shape's TopLeft
	GetInsidePlacement(width, height, paddingX, paddingY float64) geo.Point

	GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64)
	GetDefaultPadding() (paddingX, paddingY float64)

	// Perimeter returns a slice of geo.Intersectables that together constitute the shape border
	Perimeter() []geo.Intersectable

	GetSVGPathData() []string
}

type baseShape struct {
	Type      string
	Box       *geo.Box
	FullShape *Shape
}

func (s baseShape) Is(shapeType string) bool {
	return s.Type == shapeType
}

func (s baseShape) GetType() string {
	return s.Type
}

func (s baseShape) AspectRatio1() bool {
	return false
}

func (s baseShape) IsRectangular() bool {
	return false
}

func (s baseShape) GetBox() *geo.Box {
	return s.Box
}

func (s baseShape) GetInnerBox() *geo.Box {
	return s.Box
}

// only cloud shape needs this right now
func (s baseShape) GetInnerBoxForContent(width, height float64) *geo.Box {
	return nil
}

func (s baseShape) SetInnerBoxAspectRatio(aspectRatio float64) {
	// only used for cloud
}

func (s baseShape) GetInsidePlacement(_, _, paddingX, paddingY float64) geo.Point {
	innerTL := (*s.FullShape).GetInnerBox().TopLeft
	return *geo.NewPoint(innerTL.X+paddingX/2, innerTL.Y+paddingY/2)
}

// return the minimum shape dimensions needed to fit content (width x height)
// in the shape's innerBox with padding
func (s baseShape) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	return math.Ceil(width + paddingX), math.Ceil(height + paddingY)
}

func (s baseShape) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding, defaultPadding
}

func (s baseShape) Perimeter() []geo.Intersectable {
	return nil
}

func (s baseShape) GetSVGPathData() []string {
	return nil
}

func NewShape(shapeType string, box *geo.Box) Shape {
	switch shapeType {
	case CALLOUT_TYPE:
		return NewCallout(box)
	case CIRCLE_TYPE:
		return NewCircle(box)
	case CLASS_TYPE:
		return NewClass(box)
	case CLOUD_TYPE:
		return NewCloud(box)
	case CODE_TYPE:
		return NewCode(box)
	case CYLINDER_TYPE:
		return NewCylinder(box)
	case DIAMOND_TYPE:
		return NewDiamond(box)
	case DOCUMENT_TYPE:
		return NewDocument(box)
	case HEXAGON_TYPE:
		return NewHexagon(box)
	case IMAGE_TYPE:
		return NewImage(box)
	case OVAL_TYPE:
		return NewOval(box)
	case PACKAGE_TYPE:
		return NewPackage(box)
	case PAGE_TYPE:
		return NewPage(box)
	case PARALLELOGRAM_TYPE:
		return NewParallelogram(box)
	case PERSON_TYPE:
		return NewPerson(box)
	case C4_PERSON_TYPE:
		return NewC4Person(box)
	case QUEUE_TYPE:
		return NewQueue(box)
	case REAL_SQUARE_TYPE:
		return NewRealSquare(box)
	case STEP_TYPE:
		return NewStep(box)
	case STORED_DATA_TYPE:
		return NewStoredData(box)
	case SQUARE_TYPE:
		return NewSquare(box)
	case TABLE_TYPE:
		return NewTable(box)
	case TEXT_TYPE:
		return NewText(box)

	default:
		shape := shapeSquare{
			baseShape: &baseShape{
				Type: shapeType,
				Box:  box,
			},
		}
		shape.FullShape = go2.Pointer(Shape(shape))
		return shape
	}
}

// TraceToShapeBorder takes the point on the rectangular border
// r here is the point on rectangular border
// p is the prev point (used to calculate slope)
// s is the point on the actual shape border that'll be returned
//
// .      p
// .      │
// .      │
// .      ▼
// . ┌────r─────────────────────────┐
// . │                              │
// . │    │                         │
// . │    │      xxxxxxxx           │
// . │    ▼  xxxxx       xxxx       │
// . │    sxxx               xx     │
// . │   x                    xx    │
// . │  xx                     xx   │
// . │  x                      xx   │
// . │  xx                   xxx    │
// . │   xxxx             xxxx      │
// . └──────xxxxxxxxxxxxxx──────────┘
func TraceToShapeBorder(shape Shape, rectBorderPoint, prevPoint *geo.Point) *geo.Point {
	if shape.Is("") || shape.IsRectangular() {
		return rectBorderPoint
	}

	// We want to extend the line all the way through to the other end of the shape to get the intersections
	scaleSize := shape.GetBox().Width
	if prevPoint.X == rectBorderPoint.X {
		scaleSize = shape.GetBox().Height
	}
	vector := prevPoint.VectorTo(rectBorderPoint)
	vector = vector.AddLength(scaleSize)
	extendedSegment := geo.Segment{Start: prevPoint, End: prevPoint.AddVector(vector)}

	closestD := math.Inf(1)
	closestPoint := rectBorderPoint

	for _, perimeterSegment := range shape.Perimeter() {
		for _, intersectingPoint := range perimeterSegment.Intersections(extendedSegment) {
			d := geo.EuclideanDistance(rectBorderPoint.X, rectBorderPoint.Y, intersectingPoint.X, intersectingPoint.Y)
			if d < closestD {
				closestD = d
				closestPoint = intersectingPoint
			}
		}
	}

	closestPoint.TruncateFloat32()
	return geo.NewPoint(math.Round(closestPoint.X), math.Round(closestPoint.Y))
}

func boxPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, 0))
	pc.L(false, box.Width, 0)
	pc.L(false, box.Width, box.Height)
	pc.L(false, 0, box.Height)
	pc.Z()
	return pc
}

func LimitAR(width, height, aspectRatio float64) (float64, float64) {
	if width > aspectRatio*height {
		height = math.Round(width / aspectRatio)
	} else if height > aspectRatio*width {
		width = math.Round(height / aspectRatio)
	}
	return width, height
}
