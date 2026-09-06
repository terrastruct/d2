package d2scene

import (
	"image/color"
)

type AssetID string

// Asset is a closed set of fully resolved, network-free scene resources.
// Asset byte slices are retained by the Document and must not be mutated after
// construction. Builders copy mutable caller data before retention; immutable
// process-owned resources may be shared across Documents.
type Asset interface {
	isAsset()
}

type FontAsset struct {
	MIMEType string
	Data     []byte
	// FaceIndex selects one face from an OpenType collection. It is zero for a
	// single-face TTF/OTF and for the first face of a TTC. Keeping the index in
	// the resolved asset lets a scene retain host fallback fonts without asking
	// the network-free rasterizer to rediscover which collection face was used.
	FaceIndex uint16
}

func (FontAsset) isAsset() {}

// RasterAsset retains one encoded raster resource. Native rendering uses the
// first animation frame of GIF, APNG, and WebP data on its logical canvas and
// normalizes JPEG pixels and dimensions using EXIF Orientation.
type RasterAsset struct {
	MIMEType    string
	Data        []byte
	PixelWidth  int
	PixelHeight int
	// DecodedBytes is the resolver-proven decoded first-frame canvas footprint.
	// A zero value uses a conservative 4-byte-per-pixel estimate for callers
	// that have already normalized the asset to 8-bit RGBA. Values below that
	// minimum are invalid.
	DecodedBytes int64
}

func (RasterAsset) isAsset() {}

type VectorAsset struct {
	ViewBox Box
	Root    *Node
}

func (VectorAsset) isAsset() {}

type LinkRegion struct {
	// Box is expressed in the same logical coordinate space as Document.ViewBox.
	Box Box
	// URL is an external or opaque link destination. Target instead names a
	// D2 board destination. At most one of URL and Target may be set; a region
	// with neither is valid when it carries tooltip-only metadata.
	URL     string
	Tooltip string
	Target  string
}

type BlendMode uint8

const (
	BlendNormal BlendMode = iota
	BlendMultiply
	BlendDarken
	BlendColorBurn
	BlendOverlay
	BlendLighten
)

type Clip struct {
	Path      Path
	Transform Matrix
}

type MaskType uint8

const (
	MaskAlpha MaskType = iota
	MaskLuminance
)

type Mask struct {
	Type      MaskType
	Root      *Node
	Transform Matrix
}

type Filter interface {
	isFilter()
}

type GaussianBlur struct {
	SigmaX float64
	SigmaY float64
}

func (GaussianBlur) isFilter() {}

type DropShadow struct {
	OffsetX float64
	OffsetY float64
	SigmaX  float64
	SigmaY  float64
	Color   color.NRGBA
}

func (DropShadow) isFilter() {}

// Node is one immutable-ish scene layer. Transform maps node-local coordinates
// into its parent's coordinates. Callers constructing nodes directly must use
// Identity for an untransformed node; NewNode supplies that default.
type Node struct {
	ID         string
	Classes    []string
	Transform  Matrix
	Opacity    float64
	Blend      BlendMode
	Clip       *Clip
	Mask       *Mask
	Filters    []Filter
	Primitive  Primitive
	Children   []*Node
	Animations []Track
}

func NewNode(primitive Primitive) *Node {
	return &Node{
		Transform: Identity(),
		Opacity:   1,
		Primitive: primitive,
	}
}

// ViewportFit controls how a document viewbox maps into its output viewport.
// The zero value selects independent-axis stretching.
type ViewportFit uint8

const (
	ViewportStretch ViewportFit = iota
	ViewportMeet
)

// ViewportAlign controls placement when a uniform viewport fit leaves
// letterbox space. The zero value anchors content at the viewport origin.
type ViewportAlign uint8

const (
	ViewportAlignXMinYMin ViewportAlign = iota
	ViewportAlignXMidYMid
)

type Document struct {
	ViewBox       Box
	LogicalWidth  float64
	LogicalHeight float64
	ViewportFit   ViewportFit
	ViewportAlign ViewportAlign
	Root          *Node
	Assets        map[AssetID]Asset
	Links         []LinkRegion
}

func NewDocument(viewBox Box, root *Node) *Document {
	return &Document{
		ViewBox:       viewBox,
		LogicalWidth:  viewBox.Width,
		LogicalHeight: viewBox.Height,
		Root:          root,
		Assets:        make(map[AssetID]Asset),
	}
}
