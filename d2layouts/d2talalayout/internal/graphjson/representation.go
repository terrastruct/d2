package graphjson

// SerializedLabel is the fixture representation of label geometry.
type SerializedLabel struct {
	Text     string
	Position string
	Width    float64
	Height   float64
}

// SerializedIcon is the fixture representation of icon placement.
type SerializedIcon struct {
	Position string
}

// SerializedPoint is the fixed JSON representation of a geometric point.
type SerializedPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// SerializedScalar is the JSON shape of one D2 style value. Parser provenance
// does not belong in the layout-engine representation.
type SerializedScalar struct {
	Value string `json:"value"`
}

// SerializedStyle is the edge-style portion of graph JSON. Adding a host
// d2graph.Style field must not silently change the fixture representation.
type SerializedStyle struct {
	Opacity       *SerializedScalar `json:"opacity,omitempty"`
	Stroke        *SerializedScalar `json:"stroke,omitempty"`
	Fill          *SerializedScalar `json:"fill,omitempty"`
	FillPattern   *SerializedScalar `json:"fillPattern,omitempty"`
	StrokeWidth   *SerializedScalar `json:"strokeWidth,omitempty"`
	StrokeDash    *SerializedScalar `json:"strokeDash,omitempty"`
	BorderRadius  *SerializedScalar `json:"borderRadius,omitempty"`
	Shadow        *SerializedScalar `json:"shadow,omitempty"`
	ThreeDee      *SerializedScalar `json:"3d,omitempty"`
	Multiple      *SerializedScalar `json:"multiple,omitempty"`
	Font          *SerializedScalar `json:"font,omitempty"`
	FontSize      *SerializedScalar `json:"fontSize,omitempty"`
	FontColor     *SerializedScalar `json:"fontColor,omitempty"`
	Animated      *SerializedScalar `json:"animated,omitempty"`
	Bold          *SerializedScalar `json:"bold,omitempty"`
	Italic        *SerializedScalar `json:"italic,omitempty"`
	Underline     *SerializedScalar `json:"underline,omitempty"`
	Filled        *SerializedScalar `json:"filled,omitempty"`
	DoubleBorder  *SerializedScalar `json:"doubleBorder,omitempty"`
	TextTransform *SerializedScalar `json:"textTransform,omitempty"`
}

// SerializedNode is the fixture representation of one layout node.
type SerializedNode struct {
	ID             int64            `json:"id"`
	D2ID           *string          `json:"d2id,omitempty"`
	IsInvisible    bool             `json:"isInvisible,omitempty"`
	Width          float64          `json:"width"`
	Height         float64          `json:"height"`
	TopLeft        *SerializedPoint `json:"topLeft"`
	FixedTopLeft   *SerializedPoint `json:"fixedTopLeft,omitempty"`
	DesiredWidth   *float64         `json:"desiredWidth,omitempty"`
	DesiredHeight  *float64         `json:"desiredHeight,omitempty"`
	Nears          []int64          `json:"nears,omitempty"`
	FontSize       *int             `json:"fontSize,omitempty"`
	ShapeType      string           `json:"shapeType"`
	Label          *SerializedLabel `json:"label,omitempty"`
	Icon           *SerializedIcon  `json:"icon,omitempty"`
	NumColumns     int              `json:"numColumns,omitempty"`
	Is3D           bool             `json:"is3D,omitempty"`
	IsMultiple     bool             `json:"isMultiple,omitempty"`
	ForceHierarchy bool             `json:"forceHierarchy,omitempty"`
}

// SerializedEdge is the fixture representation of one layout edge.
type SerializedEdge struct {
	ID int64 `json:"id"`
	// OriginalID preserves Edge.ID when ID is a serialization-only
	// reference allocated for an otherwise ambiguous unset ID.
	OriginalID           *int64             `json:"originalId,omitempty"`
	D2ID                 *string            `json:"d2id,omitempty"`
	FromNode             int64              `json:"from"`
	IsInvisible          bool               `json:"isInvisible,omitempty"`
	ToNode               int64              `json:"to"`
	Points               []*SerializedPoint `json:"points"`
	MinWidth             int                `json:"minWidth"`
	MinHeight            int                `json:"minHeight"`
	LabelPercentage      float64            `json:"labelPercentage"`
	Label                *SerializedLabel   `json:"label,omitempty"`
	FromTableColumnIndex *int               `json:"fromTableColumnIndex,omitempty"`
	ToTableColumnIndex   *int               `json:"toTableColumnIndex,omitempty"`
	Style                SerializedStyle    `json:"style,omitempty"`

	SourceArrowhead string `json:"sourceArrowhead"`
	TargetArrowhead string `json:"targetArrowhead"`

	SourceArrowheadLabel *SerializedLabel `json:"sourceArrowheadLabel,omitempty"`
	TargetArrowheadLabel *SerializedLabel `json:"targetArrowheadLabel,omitempty"`
}

// SerializedEdgeAbduction records an edge's temporary grouping endpoints.
type SerializedEdgeAbduction struct {
	Edge               int64
	OriginallyFromNode int64
	OriginallyToNode   int64
	CurrentFromNode    int64
	CurrentToNode      int64
}

// SerializedCluster is the fixture representation of a cluster.
type SerializedCluster struct {
	Nodes              []int64
	Arrangement        string
	DesiredArrangement string
	EdgeAbductions     []SerializedEdgeAbduction
	Container          int64
	FixedSize          bool
	Padding            float64
}

// SerializedSequence is the fixture representation of a sequence.
type SerializedSequence struct {
	Nodes          []int64
	EdgeAbductions []SerializedEdgeAbduction
	Container      int64
}

// SerializedTree is the recursive fixture representation of a layout tree.
type SerializedTree struct {
	Node         SerializedNode
	Children     []SerializedTree
	SentinelEdge SerializedEdge
	Orientation  int
}

// SerializedHierarchy records node ranks in one hierarchy.
type SerializedHierarchy struct {
	Levels map[int64]int
}

// SerializedGraph is the complete graph fixture representation.
type SerializedGraph struct {
	Nodes    []SerializedNode `json:"nodes"`
	Edges    []SerializedEdge `json:"edges"`
	CellSize float64

	IsRootHierarchy bool                         `json:"isRootHierarchy,omitempty"`
	Containers      map[int64][]int64            `json:"containers"`
	Hubs            map[int64][]int64            `json:"hubs,omitempty"`
	Clusters        map[int64]SerializedCluster  `json:"clusters,omitempty"`
	Sequences       map[int64]SerializedSequence `json:"sequences,omitempty"`
	ClusterVessels  map[int64]SerializedNode     `json:"clusterVessels,omitempty"`
	SequenceVessels map[int64]SerializedNode     `json:"sequenceVessels,omitempty"`
	Trees           map[int64][]SerializedTree   `json:"trees,omitempty"`
	Hierarchies     []SerializedHierarchy        `json:"hierarchies,omitempty"`
	Directions      map[int64]string             `json:"directions,omitempty"`

	CrossingCost      float64 `json:"crossingCost"`
	TurnCost          float64 `json:"turnCost"`
	NonCenterPortCost float64 `json:"nonCenterPortCost"`
}
