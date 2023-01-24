package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapePerson struct {
	*baseShape
}

func NewPerson(box *geo.Box) Shape {
	return shapePerson{
		baseShape: &baseShape{
			Type: PERSON_TYPE,
			Box:  box,
		},
	}
}

const (
	personShoulderWidthFactor = 20.2 / 68.3
)

func (s shapePerson) GetInnerBox() *geo.Box {
	width := s.Box.Width
	tl := s.Box.TopLeft.Copy()
	shoulderWidth := personShoulderWidthFactor * width
	tl.X += shoulderWidth
	width -= shoulderWidth * 2
	return geo.NewBox(tl, width, s.Box.Height)
}

func personPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width/68.3, box.Height/77.4)

	// Bottom side
	pc.StartAt(pc.Absolute(68.3, 77.4))
	pc.H(false, 0)
	pc.V(true, -1.1)
	pc.C(true, 0, -13.2, 7.5, -25.1, 19.3, -30.8)
	pc.C(false, 12.8, 40.9, 8.9, 33.4, 8.9, 25.2)
	pc.C(false, 8.9, 11.3, 20.2, 0, 34.1, 0)

	// TODO: implement s command with mirroring last control point
	// s 			25.2,11.3, 	25.2,25.2
	// mirroring last control point (20.2,0) -> (34.1,0) = relative(13.9,0)
	pc.C(true, 13.9, 0, 25.2, 11.3, 25.2, 25.2)

	pc.C(true, 0, 8.2, -3.8, 15.6, -10.4, 20.4)
	pc.C(true, 11.8, 5.7, 19.3, 17.6, 19.3, 30.8)
	pc.V(true, 1)
	pc.H(false, 68.3)
	pc.Z()
	return pc
}

func (s shapePerson) Perimeter() []geo.Intersectable {
	return personPath(s.Box).Path

}

func (s shapePerson) GetSVGPathData() []string {
	return []string{
		personPath(s.Box).PathData(),
	}
}

func (s shapePerson) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	totalWidth := width + paddingX
	// see shapePackage
	shoulderWidth := totalWidth * personShoulderWidthFactor / (1 - 2*personShoulderWidthFactor)
	totalWidth += 2 * shoulderWidth
	totalHeight := height + paddingY
	return totalWidth, totalHeight
}

func (s shapePerson) GetDefaultPadding() (paddingX, paddingY float64) {
	return 10, defaultPadding
}
