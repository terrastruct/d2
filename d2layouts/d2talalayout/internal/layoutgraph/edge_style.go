package layoutgraph

// Arrowhead is TALA's layout-time arrowhead value. The adapter translates
// between D2 renderer arrowheads and this owned string type at the engine
// boundary; layout algorithms only need stable value equality.
type Arrowhead string

const (
	NoArrowhead       Arrowhead = "none"
	TriangleArrowhead Arrowhead = "triangle"
)

// StyleScalar carries the value of one edge style attribute. Parser
// provenance belongs to the D2 adapter and must not enter the layout engine.
type StyleScalar struct {
	Value string
}

// EdgeStyle is the complete style record carried through TALA's layout graph.
// Routing currently compares only a subset of these fields, but every value
// must survive cloning and fixture round-trips unchanged.
type EdgeStyle struct {
	Opacity       *StyleScalar
	Stroke        *StyleScalar
	Fill          *StyleScalar
	FillPattern   *StyleScalar
	StrokeWidth   *StyleScalar
	StrokeDash    *StyleScalar
	BorderRadius  *StyleScalar
	Shadow        *StyleScalar
	ThreeDee      *StyleScalar
	Multiple      *StyleScalar
	Font          *StyleScalar
	FontSize      *StyleScalar
	FontColor     *StyleScalar
	Animated      *StyleScalar
	Bold          *StyleScalar
	Italic        *StyleScalar
	Underline     *StyleScalar
	Filled        *StyleScalar
	DoubleBorder  *StyleScalar
	TextTransform *StyleScalar
}
