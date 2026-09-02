// Package d2scenebuild converts D2's typed export model into the immutable
// scene consumed by d2raster.
package d2scenebuild

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/imageasset"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
	"github.com/d2lang/d2/lib/svg"
	"github.com/d2lang/d2/lib/textmeasure"
)

const DefaultPadding = 100

type AssetOptions struct {
	// Resolver owns bounded image loading for this scene build.
	// [imageasset.ErrUnavailable] substitutes the built-in missing-image
	// placeholder; every other error aborts the build.
	Resolver        *imageasset.Resolver
	SVGImportLimits d2svgimport.Limits
	SVGImportBudget SVGImportBudget
}

// SVGImportBudget is a document-wide ceiling on parse and emitted-scene work
// across all distinct imported SVG assets. SVGImportLimits independently
// bounds the transient parse/compile work of each asset.
type SVGImportBudget struct {
	MaxSourceBytes          int
	MaxElements             int
	MaxAttributes           int
	MaxAttributeBytes       int
	MaxPathCommands         int
	MaxTransformFunctions   int
	MaxDeclaredResources    int
	MaxExpandedUseInstances int
}

// LinkBudget is a document-wide ceiling on link/tooltip metadata and, when
// Appendix is enabled, its corresponding static badges and row strings.
// Callers rendering a diagram that contains such metadata must provide
// positive limits; diagrams without metadata require no link budget.
type LinkBudget struct {
	MaxRegions     int
	MaxStringBytes int
}

// FontFallbackOptions bounds all primary/custom and resolved fallback font
// assets retained by the scene. The resolver performs the explicit I/O phase
// for code points absent from D2's configured fonts; d2raster never receives
// it and performs no filesystem discovery while painting.
type FontFallbackOptions struct {
	Resolver          d2fonts.FallbackResolver
	MaxAssets         int
	MaxBytes          int64
	MaxRunesPerText   int
	MaxTotalRunes     int
	MaxCoverageChecks int64
	// MaxFontFacesPerText, MaxShapingRuns, and MaxShapedGlyphs bound the
	// pure-Go shaping pass that materializes renderer-independent Glyphs after
	// fallback resolution. Zero selects bounded defaults.
	MaxFontFacesPerText int
	MaxShapingRuns      int
	MaxShapedGlyphs     int
}

// SketchBudget is a document-wide ceiling on structured rough-go output.
// MaxOperationSets and MaxOperations bound the generated intermediate work;
// MaxPathCommands independently bounds the typed scene geometry retained by
// the document. Callers enabling Sketch for paintable sketch geometry must
// provide positive limits.
type SketchBudget struct {
	MaxOperationSets int
	MaxOperations    int
	MaxPathCommands  int
}

// Options configures scene construction. Scale changes the logical output
// dimensions, while the raster device scale is supplied to d2raster
// separately.
type Options struct {
	Pad            *int64
	Scale          *float64
	Center         *bool
	ThemeID        *int64
	ThemeOverrides *d2target.ThemeOverrides
	// MaxNodes and MaxPathCommands enable an early, target-level admission
	// guard when positive. The guard rejects only when the target already
	// proves that the eventual scene must exceed a downstream renderer limit;
	// d2raster remains responsible for exact accounting of expanded geometry.
	MaxNodes        int
	MaxPathCommands int
	Sketch          bool
	SketchBudget    SketchBudget
	LinkBudget      LinkBudget
	// Appendix paints the numbered tooltip/link badges and static appendix
	// rows used by PNG, GIF, PDF, and PPTX exports. Interactive SVG callers
	// leave it false and retain typed metadata without static footnotes. Its
	// work is bounded by LinkBudget.
	Appendix bool

	// Assets configures the explicit, bounded I/O phase for image and
	// icon sources. Build stores only immutable bytes or imported vector scene
	// nodes in the returned document; d2raster never receives this resolver.
	Assets *AssetOptions
	Fonts  *FontFallbackOptions
}

type builder struct {
	ctx                     context.Context
	diagram                 *d2target.Diagram
	options                 Options
	theme                   d2themes.Theme
	assets                  map[d2scene.AssetID]d2scene.Asset
	assetIDs                map[[32]byte]d2scene.AssetID
	sourceAssetIDs          map[string]d2scene.AssetID
	latexAssets             map[latexAssetKey]latexAsset
	markdownRuler           *textmeasure.Ruler
	markdownRulerErr        error
	svgSourceBytes          int
	svgElements             int
	svgAttributes           int
	svgAttributeBytes       int
	svgCommands             int
	svgTransforms           int
	svgDeclaredResources    int
	svgExpandedUseInstances int
	sketchOperationSets     int
	sketchOperations        int
	sketchPathCommands      int
	sketchStreakPatterns    map[string]d2scene.PatternPaint
	sketchStreakCommands    []d2scene.PathCommand
	idToShape               map[string]d2target.Shape
	// connectionMask is shared by every connection geometry node so every route
	// and marker is cut beneath every connection label and ordinary shape border
	// label.
	connectionMask      *d2scene.Mask
	links               []d2scene.LinkRegion
	linkBytes           int
	appendixItems       []appendixItem
	appendixStringBytes int
	fontRunes           int
	fontCoverageChecks  int64
	fontShapeRuns       int
	fontGlyphs          int
	fontFaces           map[d2scene.AssetID]*fontface.ParsedFace
	codeTokens          codeTokenBudget
}

// Build preflights diagram features and returns a fully resolved, network-free
// scene. It never emits a partial scene on error.
func Build(ctx context.Context, diagram *d2target.Diagram, options Options) (*d2scene.Document, error) {
	if diagram == nil {
		return nil, fmt.Errorf("scene: nil diagram")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.Scale != nil && (!finite(*options.Scale) || *options.Scale <= 0) {
		return nil, invalidField("options", "scale", *options.Scale, "must be finite and greater than zero")
	}
	if options.MaxNodes < 0 {
		return nil, invalidField("options", "maxNodes", options.MaxNodes, "must be non-negative")
	}
	if options.MaxPathCommands < 0 {
		return nil, invalidField("options", "maxPathCommands", options.MaxPathCommands, "must be non-negative")
	}
	if options.Sketch {
		if err := validateSketchBudget(options.SketchBudget); err != nil {
			return nil, err
		}
	}
	if len(diagram.Layers) != 0 || len(diagram.Scenarios) != 0 || len(diagram.Steps) != 0 {
		return nil, unsupported("diagram", "nested boards in a single scene")
	}

	themeID := d2themescatalog.NeutralDefault.ID
	if options.ThemeID != nil {
		themeID = *options.ThemeID
	}
	theme := d2themescatalog.Find(themeID)
	if theme == (d2themes.Theme{}) {
		return nil, fmt.Errorf("scene: unknown theme %d", themeID)
	}
	theme.ApplyOverrides(options.ThemeOverrides)
	if err := admitSceneWork(ctx, diagram, options); err != nil {
		return nil, err
	}

	b := &builder{
		ctx:            ctx,
		diagram:        diagram,
		options:        options,
		theme:          theme,
		assets:         make(map[d2scene.AssetID]d2scene.Asset),
		assetIDs:       make(map[[32]byte]d2scene.AssetID),
		sourceAssetIDs: make(map[string]d2scene.AssetID),
		latexAssets:    make(map[latexAssetKey]latexAsset),
		idToShape:      make(map[string]d2target.Shape, len(diagram.Shapes)),
		fontFaces:      make(map[d2scene.AssetID]*fontface.ParsedFace),
		codeTokens:     newCodeTokenBudget(options.MaxNodes),
	}
	for _, targetShape := range diagram.Shapes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b.idToShape[targetShape.ID] = targetShape
	}

	if err := b.preflight(); err != nil {
		return nil, err
	}
	if err := b.preflightAssets(); err != nil {
		return nil, err
	}
	document, err := b.buildDocument()
	if err != nil {
		return nil, err
	}
	return document, nil
}

// Every built document has a root and background; every target shape has a
// group; and every connection has both a group and route node. Checking that
// allocation-free lower bound, together with each route's minimum command
// count, keeps giant targets out of the shape index and the later diagramObject
// allocation/sort. More elaborate scene features are deliberately left to
// d2raster's exact traversal.
func admitSceneWork(ctx context.Context, diagram *d2target.Diagram, options Options) error {
	if options.MaxNodes > 0 {
		remaining := options.MaxNodes
		for _, count := range [...]int{2, len(diagram.Shapes), len(diagram.Connections), len(diagram.Connections)} {
			if count > remaining {
				return fmt.Errorf("scene: node count exceeds limit %d", options.MaxNodes)
			}
			remaining -= count
		}
	}
	if options.MaxPathCommands == 0 {
		return nil
	}
	remaining := options.MaxPathCommands
	for connectionIndex := range diagram.Connections {
		if err := ctx.Err(); err != nil {
			return err
		}
		connection := &diagram.Connections[connectionIndex]
		// Leave malformed route structure to the detailed preflight below. A
		// valid straight route emits at least one command per point; a valid
		// cubic route emits one move plus one command per three controls.
		if len(connection.Route) < 2 || connection.IsCurve && (len(connection.Route)-1)%3 != 0 {
			continue
		}
		commands := len(connection.Route)
		if connection.IsCurve {
			commands = 1 + (len(connection.Route)-1)/3
		}
		if commands > remaining {
			return fmt.Errorf("scene: path command count exceeds limit %d", options.MaxPathCommands)
		}
		remaining -= commands
	}
	return nil
}

func (b *builder) preflight() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	root := b.diagram.Root
	if err := validateShapeNumbers("root", root); err != nil {
		return err
	}
	if err := validateFillPattern("root", root.FillPattern); err != nil {
		return err
	}
	if root.ThreeDee {
		return unsupported("root", "3d effect")
	}
	if root.Multiple {
		return unsupported("root", "multiple effect")
	}
	if root.Shadow || root.Animated || root.Blend {
		return unsupported("root", "effect")
	}
	if root.Link != "" || root.Tooltip != "" || root.PrettyLink != "" || root.TooltipPosition != "" {
		return unsupported("root", "link/tooltip metadata")
	}
	for _, targetShape := range b.diagram.Shapes {
		if err := b.preflightShape(targetShape); err != nil {
			return err
		}
	}
	for _, connection := range b.diagram.Connections {
		if err := b.preflightConnection(connection); err != nil {
			return err
		}
	}
	if err := b.preflightLegend(); err != nil {
		return err
	}
	if err := b.preflightAppendix(); err != nil {
		return err
	}
	return b.compileLinkRegions()
}

func (b *builder) preflightShape(targetShape d2target.Shape) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	object := fmt.Sprintf("shape %q", targetShape.ID)
	if err := validateShapeNumbers(object, targetShape); err != nil {
		return err
	}
	if err := validateFillPattern(object, targetShape.FillPattern); err != nil {
		return err
	}
	if err := validateShapeEffects(object, targetShape); err != nil {
		return err
	}
	if targetShape.Type == d2target.ShapeImage && targetShape.Icon == nil {
		return invalidField(object, "icon", nil, "must identify an image source for an image shape")
	}
	// Keep every source-independent layout error ahead of preflightAssets. A
	// malformed target must not trigger local file or network reads merely
	// because it also carries an icon.
	if targetShape.Opacity != 0 {
		switch targetShape.Type {
		case d2target.ShapeClass, d2target.ShapeSQLTable:
			if err := validateStructuredShape(targetShape); err != nil {
				return err
			}
		case d2target.ShapeCode:
			if err := validateCodeShape(targetShape); err != nil {
				return err
			}
		}
		if targetShape.Icon != nil && targetShape.Type != d2target.ShapeImage {
			position := label.FromString(targetShape.IconPosition)
			if !position.IsShapePosition() {
				return invalidField(object, "iconPosition", targetShape.IconPosition, "must identify a supported icon position")
			}
		}
	}
	if shapeAssetIsEmitted(targetShape) && (b.options.Assets == nil || b.options.Assets.Resolver == nil) {
		return unsupported(object, "image/icon asset without a configured resolver")
	}
	if targetShape.TooltipPosition != "" {
		if targetShape.Tooltip == "" {
			return invalidField(object, "tooltipPosition", targetShape.TooltipPosition, "requires a non-empty tooltip")
		}
		if err := validateMarkdownLabel(object+" positioned tooltip", targetShape.Tooltip, d2fonts.FONT_SIZE_M, 1, 1); err != nil {
			return err
		}
	}
	if targetShape.Language != "" {
		switch targetShape.Language {
		case "latex":
			if targetShape.Type == d2target.ShapeCode || targetShape.Type == d2target.ShapeClass || targetShape.Type == d2target.ShapeSQLTable {
				return unsupported(object, "label language latex on a structured or code shape")
			}
			if err := b.validateLatexInput(object, targetShape.Label); err != nil {
				return err
			}
			if err := b.validateLatexImportOptions(object); err != nil {
				return err
			}
		case "markdown":
			if targetShape.Type == d2target.ShapeCode || targetShape.Type == d2target.ShapeClass || targetShape.Type == d2target.ShapeSQLTable {
				return unsupported(object, "label language markdown on a structured or code shape")
			}
			if err := validateMarkdownLabel(object, targetShape.Label, targetShape.FontSize, targetShape.LabelWidth, targetShape.LabelHeight); err != nil {
				return err
			}
		default:
			if targetShape.Type == d2target.ShapeClass || targetShape.Type == d2target.ShapeSQLTable {
				return unsupported(object, "label language "+targetShape.Language+" on a structured shape")
			}
			if err := validateCodeText(object, targetShape.Text); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *builder) preflightConnection(connection d2target.Connection) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	object := fmt.Sprintf("connection %q", connection.ID)
	if err := validateConnectionNumbers(object, connection); err != nil {
		return err
	}
	if len(connection.Route) < 2 {
		return invalidField(object, "route", len(connection.Route), "must contain at least two points")
	}
	if connection.IsCurve && (len(connection.Route)-1)%3 != 0 {
		return invalidField(object, "route", len(connection.Route), "must contain 1+3n points for a cubic connection")
	}
	for i, point := range connection.Route {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		field := fmt.Sprintf("route[%d]", i)
		if point == nil {
			return fmt.Errorf("scene: %s field %q must not be nil", object, field)
		}
		if !finite(point.X) {
			return invalidField(object, field+".x", point.X, "must be finite")
		}
		if !finite(point.Y) {
			return invalidField(object, field+".y", point.Y, "must be finite")
		}
		if err := validateRouteCoordinate(object, field+".x", point.X, connection.StrokeWidth); err != nil {
			return err
		}
		if err := validateRouteCoordinate(object, field+".y", point.Y, connection.StrokeWidth); err != nil {
			return err
		}
		if i > 0 {
			previous := connection.Route[i-1]
			dx, dy := point.X-previous.X, point.Y-previous.Y
			if !finite(dx) || !finite(dy) || !finite(geo.NewVector(dx, dy).Length()) {
				return invalidField(object, fmt.Sprintf("route[%d:%d]", i-1, i), nil, "must have a finite segment length")
			}
		}
	}
	if connection.SrcArrow != d2target.NoArrowhead && connection.Route[0].VectorTo(connection.Route[1]).Length() == 0 {
		return invalidField(object, "route", nil, "must have a non-zero source arrowhead tangent")
	}
	last := len(connection.Route) - 1
	if connection.DstArrow != d2target.NoArrowhead && connection.Route[last-1].VectorTo(connection.Route[last]).Length() == 0 {
		return invalidField(object, "route", nil, "must have a non-zero destination arrowhead tangent")
	}
	if connection.Animated {
		if err := validateAnimatedConnection(object, connection); err != nil {
			return err
		}
	}
	if (connection.Tooltip != "" || connection.Link != "") && connection.Label == "" {
		return unsupported(object, "link/tooltip metadata without a label (the box-only link representation cannot encode route hit geometry)")
	}
	if connection.Label != "" {
		position := label.FromString(connection.LabelPosition)
		if !position.IsEdgePosition() {
			return invalidField(object, "labelPosition", connection.LabelPosition, "must identify a connection label position")
		}
		zeroMarkdownViewport := connection.Language == "markdown" && (connection.LabelWidth == 0 || connection.LabelHeight == 0)
		if connection.Language != "latex" && connection.FontSize <= 0 && !zeroMarkdownViewport {
			return invalidField(object, "fontSize", connection.FontSize, "must be positive for a connection with a label")
		}
		if connection.Language != "markdown" && (connection.LabelWidth <= 0 || connection.LabelHeight <= 0) {
			return invalidField(object, "labelDimensions", fmt.Sprintf("%dx%d", connection.LabelWidth, connection.LabelHeight), "must be positive for a connection with a label")
		}
	}
	if connection.Icon != nil {
		if connection.Label == "" && !label.FromString(connection.IconPosition).IsEdgePosition() {
			return invalidField(object, "iconPosition", connection.IconPosition, "must identify a connection icon position")
		}
		topLeft := connection.GetIconPosition()
		if topLeft == nil || !finite(topLeft.X) || !finite(topLeft.Y) {
			return invalidField(object, "iconPosition", connection.IconPosition, "must resolve to a finite point on the route")
		}
		if b.options.Assets == nil || b.options.Assets.Resolver == nil {
			return unsupported(object, "icon asset without a configured resolver")
		}
	}
	for _, endpoint := range []struct {
		name string
		text *d2target.Text
	}{{name: "srcLabel", text: connection.SrcLabel}, {name: "dstLabel", text: connection.DstLabel}} {
		arrowLabel := endpoint.text
		if arrowLabel == nil || arrowLabel.Label == "" {
			continue
		}
		if connection.FontSize <= 0 {
			return invalidField(object, "fontSize", connection.FontSize, "must be positive for an arrowhead label")
		}
		if arrowLabel.LabelWidth <= 0 || arrowLabel.LabelHeight <= 0 {
			return invalidField(object, endpoint.name+"Dimensions", fmt.Sprintf("%dx%d", arrowLabel.LabelWidth, arrowLabel.LabelHeight), "must be positive for an arrowhead label")
		}
	}
	if connection.Language == "latex" {
		if err := b.validateLatexInput(object, connection.Label); err != nil {
			return err
		}
		if err := b.validateLatexImportOptions(object); err != nil {
			return err
		}
	} else if connection.Language == "markdown" {
		if err := validateMarkdownLabel(object, connection.Label, connection.FontSize, connection.LabelWidth, connection.LabelHeight); err != nil {
			return err
		}
	} else if connection.Language != "" {
		if err := validateCodeText(object, connection.Text); err != nil {
			return err
		}
	}
	for _, arrowhead := range []d2target.Arrowhead{connection.SrcArrow, connection.DstArrow} {
		switch arrowhead {
		case d2target.NoArrowhead,
			d2target.ArrowArrowhead,
			d2target.TriangleArrowhead,
			d2target.UnfilledTriangleArrowhead,
			d2target.LineArrowhead,
			d2target.FilledDiamondArrowhead,
			d2target.DiamondArrowhead,
			d2target.FilledCircleArrowhead,
			d2target.CircleArrowhead,
			d2target.CrossArrowhead,
			d2target.FilledBoxArrowhead,
			d2target.BoxArrowhead,
			d2target.CfOne,
			d2target.CfMany,
			d2target.CfOneRequired,
			d2target.CfManyRequired:
		default:
			return unsupported(object, "arrowhead "+string(arrowhead))
		}
	}
	return nil
}

func (b *builder) buildDocument() (*d2scene.Document, error) {
	diagramTL, diagramBR := b.diagram.BoundingBox()
	tl, br := b.visualBoundingBox()
	if b.options.Appendix {
		// On-shape badges are part of the base viewbox before the appendix block
		// is appended.
		tl, br = diagramTL, diagramBR
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	// Legend placement uses the unfiltered diagram bounds. visualBoundingBox
	// removes appendix-only metadata because scenes do not paint those icons.
	legendLayout, err := b.measureLegend(diagramTL, diagramBR)
	if err != nil {
		return nil, err
	}
	pad := int64(DefaultPadding)
	if b.options.Pad != nil {
		pad = *b.options.Pad
	}
	left, top, width, height, err := dimensions(tl, br, pad)
	if err != nil {
		return nil, err
	}
	left, top, width, height, err = legendLayout.expandViewBox(left, top, width, height, pad)
	if err != nil {
		return nil, err
	}

	rootFill, err := b.paint(b.diagram.Root.Fill, "root fill")
	if err != nil {
		return nil, err
	}
	rootStroke, err := b.stroke(b.diagram.Root.Stroke, b.diagram.Root.StrokeWidth, b.diagram.Root.StrokeDash, d2scene.CapButt, d2scene.JoinMiter, "root stroke")
	if err != nil {
		return nil, err
	}
	strokePadding := int(math.Ceil(float64(b.diagram.Root.StrokeWidth) / 2))
	backgroundLeft, backgroundTop, backgroundWidth, backgroundHeight, ok := expandDimensions(left, top, width, height, strokePadding)
	if !ok {
		return nil, invalidPaddingError(pad)
	}
	left, top, width, height, ok = expandDimensions(backgroundLeft, backgroundTop, backgroundWidth, backgroundHeight, strokePadding)
	if !ok || width < 0 || height < 0 {
		return nil, invalidPaddingError(pad)
	}
	rootPattern, err := b.builtinPattern(b.diagram.Root.FillPattern)
	if err != nil {
		return nil, fmt.Errorf("scene: root fill pattern: %w", err)
	}

	root := d2scene.NewNode(nil)
	root.ID = "root"
	backgroundFill := rootFill
	var appendixBackground *d2scene.Node
	if b.diagram.Root.DoubleBorder {
		outerLeft, outerTop, outerWidth, outerHeight, ok := expandDimensions(left, top, width, height, strokePadding+d2target.INNER_BORDER_OFFSET)
		if !ok {
			return nil, invalidPaddingError(pad)
		}
		left, top, width, height, ok = expandDimensions(outerLeft, outerTop, outerWidth, outerHeight, strokePadding)
		if !ok || width < 0 || height < 0 {
			return nil, invalidPaddingError(pad)
		}
		outerRadius := clampBorderRadius(float64(b.diagram.Root.BorderRadius), float64(outerWidth), float64(outerHeight))
		outer := d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{
				X:      float64(outerLeft),
				Y:      float64(outerTop),
				Width:  float64(outerWidth),
				Height: float64(outerHeight),
			},
			RadiusX: outerRadius,
			RadiusY: outerRadius,
			Fill:    rootFill,
			Stroke:  rootStroke,
		})
		outer.ID = "root:double-border:outer"
		root.Children = append(root.Children, outer)
		appendixBackground = outer
		if rootPattern != nil {
			overlay, err := overlayPatternNode(outer, rootPattern, "root:double-border:outer:fill-pattern")
			if err != nil {
				return nil, err
			}
			root.Children = append(root.Children, overlay)
		}
		backgroundFill, err = b.paint("transparent", "root double-border inner fill")
		if err != nil {
			return nil, err
		}
	}

	rootRadius := clampBorderRadius(float64(b.diagram.Root.BorderRadius), float64(backgroundWidth), float64(backgroundHeight))
	background := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{
			X:      float64(backgroundLeft),
			Y:      float64(backgroundTop),
			Width:  float64(backgroundWidth),
			Height: float64(backgroundHeight),
		},
		RadiusX: rootRadius,
		RadiusY: rootRadius,
		Fill:    backgroundFill,
		Stroke:  rootStroke,
	})
	background.ID = "root:background"
	root.Children = append(root.Children, background)
	if appendixBackground == nil {
		appendixBackground = background
	}
	if rootPattern != nil {
		overlay, err := overlayPatternNode(background, rootPattern, "root:background:fill-pattern")
		if err != nil {
			return nil, err
		}
		root.Children = append(root.Children, overlay)
	}

	connectionMask, err := b.buildConnectionMask(d2scene.Box{
		X: float64(left), Y: float64(top), Width: float64(width), Height: float64(height),
	})
	if err != nil {
		return nil, err
	}
	b.connectionMask = connectionMask

	objects := make([]diagramObject, 0, len(b.diagram.Shapes)+len(b.diagram.Connections))
	for i, targetShape := range b.diagram.Shapes {
		objects = append(objects, diagramObject{shape: &b.diagram.Shapes[i], z: targetShape.ZIndex, level: targetShape.Level, shapeObject: true, order: i})
	}
	for i, connection := range b.diagram.Connections {
		objects = append(objects, diagramObject{connection: &b.diagram.Connections[i], z: connection.ZIndex, order: len(b.diagram.Shapes) + i})
	}
	sort.SliceStable(objects, func(i, j int) bool {
		if objects[i].z != objects[j].z {
			return objects[i].z < objects[j].z
		}
		if objects[i].shapeObject && objects[j].shapeObject && objects[i].level != objects[j].level {
			return objects[i].level < objects[j].level
		}
		if objects[i].shapeObject != objects[j].shapeObject {
			return objects[i].shapeObject
		}
		return objects[i].order < objects[j].order
	})

	// Build ordinary objects before measuring the appendix. Markdown rendering
	// discovers link-title regions while it constructs its typed primitives;
	// delaying appendix measurement is what makes those titles part of the
	// static raster rather than silently dropping them from paged formats.
	renderedObjects := make([]*d2scene.Node, len(objects))
	positionedTooltips := make([]*d2scene.Node, len(objects))
	for index, object := range objects {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		var node *d2scene.Node
		if object.shape != nil {
			node, err = b.buildShape(*object.shape)
		} else {
			node, err = b.buildConnection(*object.connection)
		}
		if err != nil {
			return nil, err
		}
		renderedObjects[index] = node
		if object.shape != nil {
			positionedTooltips[index], err = b.buildPositionedTooltip(*object.shape)
			if err != nil {
				return nil, err
			}
		}
	}

	appendixLayout, err := b.measureAppendix(diagramTL, diagramBR)
	if err != nil {
		return nil, err
	}
	left, top, width, height, err = appendixLayout.expandViewBox(left, top, width, height)
	if err != nil {
		return nil, err
	}
	if appendixLayout != nil {
		// appendix.Append rewrites only the first rendered background rectangle's
		// width and height. Preserve its x/y and the svg double-border/pattern
		// quirk instead of resizing every root paint layer.
		backgroundRect, ok := appendixBackground.Primitive.(d2scene.Rect)
		if !ok {
			return nil, fmt.Errorf("scene: appendix background is not a rectangle")
		}
		backgroundRect.Box.Width = float64(width)
		backgroundRect.Box.Height = float64(height)
		appendixBackground.Primitive = backgroundRect
	}

	root.Children = append(root.Children, renderedObjects...)
	tooltipNumbers, linkNumbers := appendixIconNumbers(b.diagram)
	var metadataNodes []*d2scene.Node
	for index, object := range objects {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		if object.shape != nil {
			if positionedTooltips[index] != nil {
				metadataNodes = append(metadataNodes, positionedTooltips[index])
			}
			icons, err := b.buildAppendixIcons(
				appendixLayout, *object.shape, object.order,
				tooltipNumbers[object.order], linkNumbers[object.order],
			)
			if err != nil {
				return nil, err
			}
			metadataNodes = append(metadataNodes, icons...)
		}
	}
	// Paint positioned tooltips and appendix badges above every ordinary object,
	// preserving sorted shape order and each shape's tooltip-before-link order.
	root.Children = append(root.Children, metadataNodes...)
	legendNode, err := b.buildLegend(legendLayout)
	if err != nil {
		return nil, err
	}
	if legendNode != nil {
		// Paint the complete legend after ordinary objects and appendix visuals,
		// independently of object z-index.
		root.Children = append(root.Children, legendNode)
	}
	appendixNode, err := b.buildAppendix(appendixLayout)
	if err != nil {
		return nil, err
	}
	if appendixNode != nil {
		// Paint the appendix separator and rows above both badges and the legend.
		root.Children = append(root.Children, appendixNode)
	}
	textReferences, err := b.sceneTextReferences(root)
	if err != nil {
		return nil, err
	}
	if err := b.resolveFontFallbacks(textReferences); err != nil {
		return nil, err
	}
	if err := b.shapeTextRuns(textReferences); err != nil {
		return nil, err
	}

	document := d2scene.NewDocument(d2scene.Box{
		X: float64(left), Y: float64(top), Width: float64(width), Height: float64(height),
	}, root)
	document.Assets = b.assets
	document.Links = append([]d2scene.LinkRegion(nil), b.links...)
	document.ViewportFit = d2scene.ViewportMeet
	if b.options.Center != nil && *b.options.Center {
		document.ViewportAlign = d2scene.ViewportAlignXMidYMid
	}
	logicalScale := 1.0
	if b.options.Scale != nil {
		logicalScale = *b.options.Scale
	}
	if appendixLayout != nil {
		// SVG exports with appendix items use the expanded viewbox dimensions
		// for their outer width and height, without applying the requested scale.
		logicalScale = 1
	}
	scaledWidth := logicalScale * float64(width)
	scaledHeight := logicalScale * float64(height)
	if !finite(scaledWidth) || !finite(scaledHeight) || scaledWidth <= 0 || scaledHeight <= 0 {
		return nil, invalidField("options", "scale", logicalScale, "produces non-finite or non-positive logical dimensions")
	}
	document.LogicalWidth = math.Ceil(scaledWidth)
	document.LogicalHeight = math.Ceil(scaledHeight)
	return document, nil
}

type diagramObject struct {
	shape       *d2target.Shape
	connection  *d2target.Connection
	z           int
	level       int
	shapeObject bool
	order       int
}

func (b *builder) buildShape(targetShape d2target.Shape) (*d2scene.Node, error) {
	fillValue, strokeValue := d2themes.ShapeTheme(targetShape)
	fill, err := b.paint(fillValue, fmt.Sprintf("shape %q fill", targetShape.ID))
	if err != nil {
		return nil, err
	}
	stroke, err := b.stroke(strokeValue, targetShape.StrokeWidth, targetShape.StrokeDash, d2scene.CapButt, d2scene.JoinRound, fmt.Sprintf("shape %q stroke", targetShape.ID))
	if err != nil {
		return nil, err
	}

	group := d2scene.NewNode(nil)
	group.ID = targetShape.ID
	group.Classes = append([]string(nil), targetShape.Classes...)
	group.Opacity = targetShape.Opacity
	if targetShape.Opacity == 0 {
		return group, nil
	}
	if targetShape.Animated {
		configureAnimatedShape(group)
	}
	if targetShape.Type == d2target.ShapeCode {
		children, err := b.buildCodeShape(targetShape, stroke)
		if err != nil {
			return nil, err
		}
		b.appendShapeGeometry(group, targetShape, nil)
		icon, err := b.buildShapeIcon(targetShape, false)
		if err != nil {
			return nil, err
		}
		if icon != nil {
			group.Children = append(group.Children, icon)
		}
		group.Children = append(group.Children, children...)
		return b.finishShape(group, targetShape)
	}
	if targetShape.Type == d2target.ShapeClass || targetShape.Type == d2target.ShapeSQLTable {
		var children []*d2scene.Node
		if b.options.Sketch {
			children, err = b.buildSketchStructuredShape(targetShape, fill, stroke)
		} else {
			children, err = b.buildStructuredShape(targetShape, fill, stroke)
		}
		if err != nil {
			return nil, err
		}
		if !b.options.Sketch {
			pattern, err := b.builtinPattern(targetShape.FillPattern)
			if err != nil {
				return nil, fmt.Errorf("scene: shape %q fill pattern: %w", targetShape.ID, err)
			}
			children, err = b.interleavePattern(children, pattern, targetShape.ID, structuredPatternNode(targetShape.ID))
			if err != nil {
				return nil, err
			}
		}
		icon, err := b.buildShapeIcon(targetShape, true)
		if err != nil {
			return nil, err
		}
		if icon != nil {
			children = append(children, icon)
		}
		b.appendShapeGeometry(group, targetShape, children)
		return b.finishShape(group, targetShape)
	}

	var geometryChildren []*d2scene.Node
	if targetShape.Type == d2target.ShapeImage {
		imageNode, err := b.buildShapeImage(targetShape)
		if err != nil {
			return nil, err
		}
		geometryChildren = append(geometryChildren, imageNode)
	} else if targetShape.ThreeDee || targetShape.Multiple || targetShape.DoubleBorder {
		var effectNodes []*d2scene.Node
		if b.options.Sketch {
			effectNodes, err = b.buildSketchShapeEffects(targetShape, fill, stroke)
		} else {
			effectNodes, err = b.buildShapeEffects(targetShape, fill, stroke)
		}
		if err != nil {
			return nil, err
		}
		if !b.options.Sketch {
			pattern, err := b.builtinPattern(targetShape.FillPattern)
			if err != nil {
				return nil, fmt.Errorf("scene: shape %q fill pattern: %w", targetShape.ID, err)
			}
			effectNodes, err = b.interleavePattern(effectNodes, pattern, targetShape.ID, effectPatternNode(targetShape))
			if err != nil {
				return nil, err
			}
		}
		geometryChildren = append(geometryChildren, effectNodes...)
	} else {
		var outlineNodes []*d2scene.Node
		if b.options.Sketch {
			outlineNodes, err = b.buildSketchOrdinaryShape(targetShape, "")
		} else {
			outlineNodes, err = b.buildOrdinaryShapeOutline(targetShape, fill, stroke, "")
		}
		if err != nil {
			return nil, err
		}
		if len(outlineNodes) != 0 && !b.options.Sketch {
			pattern, err := b.builtinPattern(targetShape.FillPattern)
			if err != nil {
				return nil, fmt.Errorf("scene: shape %q fill pattern: %w", targetShape.ID, err)
			}
			outlineNodes, err = b.interleavePattern(outlineNodes, pattern, targetShape.ID, ordinaryPatternNode)
			if err != nil {
				return nil, err
			}
		}
		geometryChildren = append(geometryChildren, outlineNodes...)
	}
	b.appendShapeGeometry(group, targetShape, geometryChildren)
	icon, err := b.buildShapeIcon(targetShape, false)
	if err != nil {
		return nil, err
	}
	if icon != nil {
		group.Children = append(group.Children, icon)
	}

	if targetShape.Label != "" {
		var textNodes []*d2scene.Node
		if targetShape.Language != "" && targetShape.Language != "latex" && targetShape.Language != "markdown" {
			textNodes, err = b.buildCodeShape(targetShape, stroke)
		} else {
			textNodes, err = b.buildShapeText(targetShape)
		}
		if err != nil {
			return nil, err
		}
		group.Children = append(group.Children, textNodes...)
	}
	return b.finishShape(group, targetShape)
}

func (b *builder) buildShapeText(targetShape d2target.Shape) ([]*d2scene.Node, error) {
	if targetShape.Language == "latex" {
		topLeft := latexShapeLabelTopLeft(targetShape)
		node, err := b.buildLatexLabelNode(
			fmt.Sprintf("shape %q", targetShape.ID), targetShape.ID+":label:0",
			targetShape.Label, targetShape.Stroke, topLeft,
		)
		if err != nil {
			return nil, err
		}
		return []*d2scene.Node{node}, nil
	}
	if targetShape.Language == "markdown" {
		return b.buildShapeMarkdown(targetShape)
	}
	_, topLeft := shapeLabelPlacement(targetShape)
	font, err := b.font(targetShape.Text)
	if err != nil {
		return nil, fmt.Errorf("scene: shape %q: %w", targetShape.ID, err)
	}
	fill, err := b.paint(targetShape.GetFontColor(), fmt.Sprintf("shape %q label color", targetShape.ID))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(targetShape.Label, "\n")
	nodes := make([]*d2scene.Node, 0, len(lines)+1)
	if targetShape.LabelFill != "" {
		labelFill, err := b.paint(targetShape.LabelFill, fmt.Sprintf("shape %q label fill", targetShape.ID))
		if err != nil {
			return nil, err
		}
		fillNode := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{X: topLeft.X, Y: topLeft.Y, Width: float64(targetShape.LabelWidth), Height: float64(targetShape.LabelHeight)},
			Fill: labelFill,
		})
		fillNode.ID = targetShape.ID + ":label-fill"
		nodes = append(nodes, fillNode)
	}
	lineAdvance := float64(targetShape.LabelHeight) / float64(len(lines))
	for i, line := range lines {
		baseline := topLeft.Y + float64(targetShape.FontSize)
		if i > 0 {
			baseline += float64(i) * lineAdvance
		}
		run := d2scene.TextRun{
			Text:      line,
			Origin:    d2scene.Point{X: topLeft.X + float64(targetShape.LabelWidth)/2, Y: baseline},
			Anchor:    d2scene.AnchorMiddle,
			Font:      font,
			Fill:      fill,
			Underline: targetShape.Underline,
			Ink:       d2scene.NewBounds(topLeft.X, topLeft.Y+float64(i)*lineAdvance, topLeft.X+float64(targetShape.LabelWidth), topLeft.Y+float64(i+1)*lineAdvance),
		}
		textNode := d2scene.NewNode(run)
		textNode.ID = fmt.Sprintf("%s:label:%d", targetShape.ID, i)
		nodes = append(nodes, textNode)
	}
	return nodes, nil
}

func targetGeometry(targetShape d2target.Shape) shape.Shape {
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[targetShape.Type]
	geometry := shape.NewShape(shapeType, geo.NewBox(
		geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)),
		float64(targetShape.Width), float64(targetShape.Height),
	))
	if targetShape.Type == d2target.ShapeCloud && targetShape.ContentAspectRatio != nil {
		geometry.SetInnerBoxAspectRatio(*targetShape.ContentAspectRatio)
	}
	return geometry
}

func scenePath(commands []svg.PathCommand, fill d2scene.Paint, stroke *d2scene.Stroke) (d2scene.Path, error) {
	path := d2scene.Path{Fill: fill, Stroke: stroke}
	path.Commands = make([]d2scene.PathCommand, 0, len(commands))
	for commandIndex, command := range commands {
		switch command.Kind {
		case svg.PathCommandMove:
			path.Commands = append(path.Commands, d2scene.MoveTo(command.End.X, command.End.Y))
		case svg.PathCommandLine:
			path.Commands = append(path.Commands, d2scene.LineTo(command.End.X, command.End.Y))
		case svg.PathCommandCubic:
			path.Commands = append(path.Commands, d2scene.CubicTo(
				command.Control1.X, command.Control1.Y,
				command.Control2.X, command.Control2.Y,
				command.End.X, command.End.Y,
			))
		case svg.PathCommandClose:
			path.Commands = append(path.Commands, d2scene.ClosePath())
		default:
			return d2scene.Path{}, fmt.Errorf("unknown typed command %d at index %d", command.Kind, commandIndex)
		}
	}
	return path, nil
}

func (b *builder) font(text d2target.Text) (d2scene.Font, error) {
	family := d2fonts.SourceSansPro
	if b.diagram.FontFamily != nil {
		family = *b.diagram.FontFamily
	}
	switch {
	case text.FontFamily == "", strings.EqualFold(text.FontFamily, "default"):
		// DEFAULT is a role, not a concrete bundled font. Keep the diagram's
		// configured primary family when it has one.
	case strings.EqualFold(text.FontFamily, "mono"):
		family = d2fonts.SourceCodePro
		if b.diagram.MonoFontFamily != nil {
			family = *b.diagram.MonoFontFamily
		}
	default:
		if mapped, ok := d2fonts.D2_FONT_TO_FAMILY[strings.ToLower(text.FontFamily)]; ok {
			family = mapped
		}
	}
	style := d2fonts.FONT_STYLE_REGULAR
	if text.Bold {
		style = d2fonts.FONT_STYLE_BOLD
	} else if text.Italic {
		style = d2fonts.FONT_STYLE_ITALIC
	}
	fontSpec := d2fonts.Font{Family: family, Style: style}
	fontBytes, ok := d2fonts.FontFaces.Lookup(fontSpec)
	if !ok {
		return d2scene.Font{}, fmt.Errorf("font %s/%s is not loaded", family, style)
	}
	assetID := d2scene.AssetID("font:" + string(family) + ":" + string(style))
	if _, exists := b.assets[assetID]; !exists {
		b.assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: retainedFontBytes(fontBytes)}
	}
	weight := 400
	if style == d2fonts.FONT_STYLE_BOLD {
		weight = 700
	} else if style == d2fonts.FONT_STYLE_SEMIBOLD {
		weight = 600
	}
	return d2scene.Font{Family: string(family), Style: string(style), Weight: weight, Size: float64(text.FontSize), Asset: assetID}, nil
}

func retainedFontBytes(data []byte) []byte {
	return append([]byte(nil), data...)
}

func unsupported(object, feature string) error {
	return fmt.Errorf("scene: %s uses unsupported %s", object, feature)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clampBorderRadius(radius, width, height float64) float64 {
	return math.Max(0, math.Min(radius, math.Min(width/2, height/2)))
}

func invalidField(object, field string, value any, requirement string) error {
	if value == nil {
		return fmt.Errorf("scene: %s field %q %s", object, field, requirement)
	}
	return fmt.Errorf("scene: %s field %q has invalid value %v: %s", object, field, value, requirement)
}

func validateShapeNumbers(object string, targetShape d2target.Shape) error {
	for _, field := range []struct {
		name  string
		value int
	}{
		{name: "width", value: targetShape.Width},
		{name: "height", value: targetShape.Height},
		{name: "strokeWidth", value: targetShape.StrokeWidth},
		{name: "borderRadius", value: targetShape.BorderRadius},
		{name: "iconBorderRadius", value: targetShape.IconBorderRadius},
	} {
		if field.value < 0 {
			return invalidField(object, field.name, field.value, "must be non-negative")
		}
	}
	if err := validateOpacity(object, targetShape.Opacity); err != nil {
		return err
	}
	if err := validateDash(object, targetShape.StrokeDash, targetShape.StrokeWidth); err != nil {
		return err
	}
	if targetShape.ContentAspectRatio != nil && (!finite(*targetShape.ContentAspectRatio) || *targetShape.ContentAspectRatio <= 0) {
		return invalidField(object, "contentAspectRatio", *targetShape.ContentAspectRatio, "must be finite and greater than zero")
	}
	if err := validateTextNumbers(object, "", targetShape.Text); err != nil {
		return err
	}
	return validateShapeIntegerBounds(object, targetShape)
}

func validateConnectionNumbers(object string, connection d2target.Connection) error {
	if connection.StrokeWidth < 0 {
		return invalidField(object, "strokeWidth", connection.StrokeWidth, "must be non-negative")
	}
	if err := validateOpacity(object, connection.Opacity); err != nil {
		return err
	}
	if err := validateDash(object, connection.StrokeDash, connection.StrokeWidth); err != nil {
		return err
	}
	if !finite(connection.BorderRadius) || connection.BorderRadius < 0 {
		return invalidField(object, "borderRadius", connection.BorderRadius, "must be finite and non-negative")
	}
	if !finite(connection.IconBorderRadius) || connection.IconBorderRadius < 0 {
		return invalidField(object, "iconBorderRadius", connection.IconBorderRadius, "must be finite and non-negative")
	}
	if !finite(connection.LabelPercentage) || connection.LabelPercentage < 0 || connection.LabelPercentage > 1 {
		return invalidField(object, "labelPercentage", connection.LabelPercentage, "must be finite and within [0,1]")
	}
	if err := validateTextNumbers(object, "", connection.Text); err != nil {
		return err
	}
	if connection.SrcLabel != nil {
		if err := validateTextNumbers(object, "srcLabel.", *connection.SrcLabel); err != nil {
			return err
		}
	}
	if connection.DstLabel != nil {
		if err := validateTextNumbers(object, "dstLabel.", *connection.DstLabel); err != nil {
			return err
		}
	}
	return nil
}

func validateOpacity(object string, opacity float64) error {
	if !finite(opacity) || opacity < 0 || opacity > 1 {
		return invalidField(object, "opacity", opacity, "must be finite and within [0,1]")
	}
	return nil
}

func validateDash(object string, dash float64, strokeWidth int) error {
	if !finite(dash) || dash < 0 {
		return invalidField(object, "strokeDash", dash, "must be finite and non-negative")
	}
	if dash > 0 && strokeWidth > 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(strokeWidth), dash)
		if !finite(dashSize) || !finite(gapSize) || dashSize < 0 || gapSize < 0 {
			return invalidField(object, "strokeDash", dash, "must produce finite non-negative dash lengths with strokeWidth")
		}
	}
	return nil
}

func validateTextNumbers(object, prefix string, text d2target.Text) error {
	if text.FontSize < 0 {
		return invalidField(object, prefix+"fontSize", text.FontSize, "must be non-negative")
	}
	if text.LabelWidth < 0 {
		return invalidField(object, prefix+"labelWidth", text.LabelWidth, "must be non-negative")
	}
	if text.LabelHeight < 0 {
		return invalidField(object, prefix+"labelHeight", text.LabelHeight, "must be non-negative")
	}
	return nil
}

func validateShapeIntegerBounds(object string, targetShape d2target.Shape) error {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	halfStroke := int64(targetShape.StrokeWidth/2 + targetShape.StrokeWidth%2)
	for _, axis := range []struct {
		name      string
		position  int
		dimension int
	}{
		{name: "pos.x", position: targetShape.Pos.X, dimension: targetShape.Width},
		{name: "pos.y", position: targetShape.Pos.Y, dimension: targetShape.Height},
	} {
		if _, ok := checkedSub(int64(axis.position), halfStroke, minInt, maxInt); !ok {
			return invalidField(object, axis.name, axis.position, "must fit raster bounds arithmetic with strokeWidth")
		}
		end, ok := checkedAdd(int64(axis.position), int64(axis.dimension), minInt, maxInt)
		if !ok {
			return invalidField(object, axis.name, axis.position, "must fit raster bounds arithmetic with its dimension")
		}
		if _, ok := checkedAdd(end, halfStroke, minInt, maxInt); !ok {
			return invalidField(object, axis.name, axis.position, "must fit raster bounds arithmetic with strokeWidth")
		}
	}
	return nil
}

func validateRouteCoordinate(object, field string, coordinate float64, strokeWidth int) error {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	halfStroke := math.Ceil(float64(strokeWidth) / 2)
	// BoundingBox floors/ceils route coordinates and converts them to int.
	// Strict comparisons avoid the rounded float64 representation of MaxInt.
	if math.Floor(coordinate)-halfStroke <= float64(minInt) || math.Ceil(coordinate)+halfStroke >= float64(maxInt) {
		return invalidField(object, field, coordinate, "must fit raster integer bounds arithmetic")
	}
	return nil
}

func invalidPaddingError(pad int64) error {
	return fmt.Errorf("padding %d produces invalid scene dimensions", pad)
}

func dimensions(tl, br d2target.Point, pad int64) (left, top, width, height int, err error) {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	if pad < minInt || pad > maxInt {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	doublePad, ok := checkedAdd(pad, pad, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	left64, ok := checkedSub(int64(tl.X), pad, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	top64, ok := checkedSub(int64(tl.Y), pad, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	width64, ok := checkedSub(int64(br.X), int64(tl.X), minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	width64, ok = checkedAdd(width64, doublePad, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	height64, ok := checkedSub(int64(br.Y), int64(tl.Y), minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	height64, ok = checkedAdd(height64, doublePad, minInt, maxInt)
	if !ok || width64 < 0 || height64 < 0 {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	return int(left64), int(top64), int(width64), int(height64), nil
}

func checkedAdd(a, value, minInt, maxInt int64) (int64, bool) {
	if value > 0 && a > maxInt-value || value < 0 && a < minInt-value {
		return 0, false
	}
	return a + value, true
}

func checkedSub(a, value, minInt, maxInt int64) (int64, bool) {
	if value > 0 && a < minInt+value || value < 0 && a > maxInt+value {
		return 0, false
	}
	return a - value, true
}

func expandDimensions(left, top, width, height, amount int) (int, int, int, int, bool) {
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	amount64 := int64(amount)
	doubleAmount, ok := checkedAdd(amount64, amount64, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, false
	}
	left64, ok := checkedSub(int64(left), amount64, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, false
	}
	top64, ok := checkedSub(int64(top), amount64, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, false
	}
	width64, ok := checkedAdd(int64(width), doubleAmount, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, false
	}
	height64, ok := checkedAdd(int64(height), doubleAmount, minInt, maxInt)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return int(left64), int(top64), int(width64), int(height64), true
}

func (b *builder) stroke(raw string, width int, dash float64, cap d2scene.LineCap, join d2scene.LineJoin, description string) (*d2scene.Stroke, error) {
	paint, err := b.paint(raw, description)
	if err != nil {
		return nil, err
	}
	if paint == nil || width <= 0 {
		return nil, nil
	}
	stroke := &d2scene.Stroke{Paint: paint, Width: float64(width), Cap: cap, Join: join, MiterLimit: 4}
	if dash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(width), dash)
		stroke.Dashes = []float64{dashSize, gapSize}
	}
	return stroke, nil
}
