// Package nodeshape defines the shape-specific port and label-placement policy
// used by TALA nodes.
package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

// LabelTier groups label positions by how suitable they are for a shape.
type LabelTier int

// Label preference tiers run from Good to Bad.
const (
	Good LabelTier = iota
	OK
	Unideal
	Bad
)

// Kind identifies the TALA behavior associated with a node shape.
type Kind uint8

// Shape kinds identify the policy wrapper returned by New.
const (
	Square Kind = iota
	RealSquare
	Parallelogram
	Document
	Cylinder
	Queue
	Page
	Package
	Step
	Callout
	StoredData
	Person
	C4Person
	Diamond
	Oval
	Circle
	Hexagon
	Cloud
	Table
	Class
	Text
	Code
	Image
)

// String returns the D2 shape type represented by kind.
func (kind Kind) String() string {
	switch kind {
	case Callout:
		return shape.CALLOUT_TYPE
	case Circle:
		return shape.CIRCLE_TYPE
	case Cloud:
		return shape.CLOUD_TYPE
	case Cylinder:
		return shape.CYLINDER_TYPE
	case Diamond:
		return shape.DIAMOND_TYPE
	case Document:
		return shape.DOCUMENT_TYPE
	case Hexagon:
		return shape.HEXAGON_TYPE
	case Image:
		return shape.IMAGE_TYPE
	case Oval:
		return shape.OVAL_TYPE
	case Package:
		return shape.PACKAGE_TYPE
	case Page:
		return shape.PAGE_TYPE
	case Parallelogram:
		return shape.PARALLELOGRAM_TYPE
	case Person:
		return shape.PERSON_TYPE
	case C4Person:
		return shape.C4_PERSON_TYPE
	case Queue:
		return shape.QUEUE_TYPE
	case RealSquare:
		return shape.REAL_SQUARE_TYPE
	case Square:
		return shape.SQUARE_TYPE
	case Step:
		return shape.STEP_TYPE
	case StoredData:
		return shape.STORED_DATA_TYPE
	case Text:
		return shape.TEXT_TYPE
	case Class:
		return shape.CLASS_TYPE
	case Table:
		return shape.TABLE_TYPE
	case Code:
		return shape.CODE_TYPE
	default:
		return ""
	}
}

// Shape extends D2's geometric shape with the ports and label-position policy
// needed by TALA.
type Shape interface {
	shape.Shape

	// SnapPointPercentages returns port groups in top, left, bottom, and right order.
	SnapPointPercentages() [][]*geo.RelativePoint
	// MirroredPortIndices maps each port index to its reflected counterpart.
	MirroredPortIndices() map[int]int
	// CenterPortIndices returns the center port on each side.
	CenterPortIndices() []int
	// PortIndices returns the ports available for an orientation.
	PortIndices(orientation geo.Orientation) []int
	// CenterPortIndex returns the center port for an orientation, or -1 if the
	// shape does not expose one.
	CenterPortIndex(orientation geo.Orientation) int
	// LabelPositionPreferences returns the positions in a preference tier.
	LabelPositionPreferences(tier LabelTier) map[label.Position]struct{}
}

// New constructs TALA's policy wrapper around a D2 shape. An empty type uses
// square behavior while retaining the empty D2 type. Unsupported nonempty types
// return ok=false so callers can preserve their existing shape.
func New(typeName string, box *geo.Box) (Shape, Kind, bool) {
	base := shape.NewShape(typeName, box)
	switch typeName {
	case shape.CALLOUT_TYPE:
		return &shapeCallout{Shape: base}, Callout, true
	case shape.CIRCLE_TYPE:
		return &shapeCircle{shapeSquare: shapeSquare{Shape: base}}, Circle, true
	case shape.CLOUD_TYPE:
		return &shapeCloud{Shape: base}, Cloud, true
	case shape.CYLINDER_TYPE:
		return &shapeCylinder{shapeSquare: shapeSquare{Shape: base}}, Cylinder, true
	case shape.DIAMOND_TYPE:
		return &shapeDiamond{Shape: base}, Diamond, true
	case shape.DOCUMENT_TYPE:
		return &shapeDocument{shapeSquare: shapeSquare{Shape: base}}, Document, true
	case shape.HEXAGON_TYPE:
		return &shapeHexagon{shapeSquare: shapeSquare{Shape: base}}, Hexagon, true
	case shape.IMAGE_TYPE:
		return &shapeImage{shapeSquare: shapeSquare{Shape: base}}, Image, true
	case shape.OVAL_TYPE:
		return &shapeOval{shapeSquare: shapeSquare{Shape: base}}, Oval, true
	case shape.PACKAGE_TYPE:
		return &shapePackage{Shape: base}, Package, true
	case shape.PAGE_TYPE:
		return &shapePage{shapeSquare: shapeSquare{Shape: base}}, Page, true
	case shape.PARALLELOGRAM_TYPE:
		return &shapeParallelogram{shapeSquare: shapeSquare{Shape: base}}, Parallelogram, true
	case shape.PERSON_TYPE:
		return &shapePerson{Shape: base}, Person, true
	case shape.C4_PERSON_TYPE:
		return &shapeC4Person{Shape: base}, C4Person, true
	case shape.QUEUE_TYPE:
		return &shapeQueue{shapeSquare: shapeSquare{Shape: base}}, Queue, true
	case shape.REAL_SQUARE_TYPE:
		return &shapeSquare{Shape: base}, RealSquare, true
	case shape.SQUARE_TYPE:
		return &shapeSquare{Shape: base}, Square, true
	case shape.STEP_TYPE:
		return &shapeStep{Shape: base}, Step, true
	case shape.STORED_DATA_TYPE:
		return &shapeStoredData{shapeSquare: shapeSquare{Shape: base}}, StoredData, true
	case shape.TEXT_TYPE:
		return &shapeSquare{Shape: base}, Text, true
	case shape.CLASS_TYPE:
		return &shapeSquare{Shape: base}, Class, true
	case shape.TABLE_TYPE:
		return &shapeTable{shapeSquare: &shapeSquare{Shape: base}}, Table, true
	case shape.CODE_TYPE:
		return &shapeSquare{Shape: base}, Code, true
	case "":
		return &shapeSquare{Shape: base}, Square, true
	default:
		return nil, 0, false
	}
}
