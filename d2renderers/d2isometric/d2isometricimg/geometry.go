package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/mazznoer/csscolorparser"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
)

// The native renderer turns the compiled scene into meshes with layout-sized
// footprints and physical extrusion.
type nativeScene struct {
	raster        *Raster
	camera        rasterCamera // logical output coordinates, also used by SVG
	packets       []packetRoute
	triangles     []Triangle
	panels        []nativePanel
	width, height int
	background    color.NRGBA
	animatedNodes []nativeAnimatedNode
	pixelScale    float64
	links         []nativeLink
}
type meshBuilder struct {
	ctx               context.Context
	triangles         []Triangle
	err               error
	text              *textPainter
	rich              *richLabelPainter
	icons             *surfaceIconPainter
	scale             float64
	panels            []nativePanel
	animatedNodes     []nativeAnimatedNode
	facePixels        int64
	faceMaxPixels     int64
	arrowBackground   string
	arrowOwners       map[string]*nativeArrowOwner
	arrowMarkers      map[nativeArrowMarkerKey]nativeArrowMarker
	arrowWork         int64
	arrowCachePoints  int
	routeCasingFloor  float64
	hierarchySupports map[string]float64
	nodeSupportDrop   float64
	fonts             *d2scenebuild.FontFallbackOptions
	outputDensity     float64
	options           nativeSceneOptions
	links             []nativeLink
	linkBytes         int
}
type labelSurface struct {
	center              Vec
	width, depth, angle float64
	align               string
}

const maxNativeTriangles = 1_000_000

func nv(x, y, z float64) Vec    { return Vec{X: x, Y: y, Z: z} }
func nadd(a, b Vec) Vec         { return nv(a.X+b.X, a.Y+b.Y, a.Z+b.Z) }
func nsub(a, b Vec) Vec         { return nv(a.X-b.X, a.Y-b.Y, a.Z-b.Z) }
func nmul(a Vec, s float64) Vec { return nv(a.X*s, a.Y*s, a.Z*s) }
func ndot(a, b Vec) float64     { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func ncross(a, b Vec) Vec       { return nv(a.Y*b.Z-a.Z*b.Y, a.Z*b.X-a.X*b.Z, a.X*b.Y-a.Y*b.X) }
func nlen(a Vec) float64        { return math.Sqrt(ndot(a, a)) }
func nunit(a Vec) Vec {
	if l := nlen(a); l > 1e-12 {
		return nmul(a, 1/l)
	}
	return nv(0, 1, 0)
}
func nlerp(a, b Vec, t float64) Vec { return nadd(a, nmul(nsub(b, a), t)) }
func nativeToken(s string) bool {
	if s == "" {
		return true
	}
	for _, p := range []string{"N", "B", "AA", "AB"} {
		if strings.HasPrefix(s, p) {
			v := strings.TrimPrefix(s, p)
			if v != "" && strings.Trim(v, "0123456789") == "" {
				return true
			}
		}
	}
	return false
}
func nativePaint(s, fallback string) color.NRGBA {
	if nativeToken(s) {
		s = fallback
	}
	c, e := csscolorparser.Parse(s)
	if e != nil {
		c, _ = csscolorparser.Parse(fallback)
	}
	r, g, b, a := c.RGBA255()
	return color.NRGBA{R: r, G: g, B: b, A: a}
}
func nativeMaterial(s string, rough, metal, opacity float64) *Material {
	c := nativePaint(s, "#edf1f7")
	c.A = uint8(math.Round(float64(c.A) * max(0, min(1, opacity))))
	return &Material{Color: c, Roughness: rough, Metalness: metal}
}
func tintDark(c color.NRGBA, f float64) color.NRGBA {
	c.R = uint8(float64(c.R) * f)
	c.G = uint8(float64(c.G) * f)
	c.B = uint8(float64(c.B) * f)
	return c
}

func (b *meshBuilder) triangle(a, c, d Vertex, m *Material, shadow bool) {
	if b.err != nil || m == nil || m.Color.A == 0 {
		return
	}
	if len(b.triangles) >= maxNativeTriangles {
		b.err = fmt.Errorf("isometric geometry exceeds %d triangles", maxNativeTriangles)
		return
	}
	if len(b.triangles)%1024 == 0 {
		if e := b.ctx.Err(); e != nil {
			b.err = e
			return
		}
	}
	b.triangles = append(b.triangles, Triangle{V: [3]Vertex{a, c, d}, Material: m, CastShadow: shadow})
}
func (b *meshBuilder) flat(a, c, d Vec, m *Material, shadow bool) {
	n := nunit(ncross(nsub(c, a), nsub(d, a)))
	b.triangle(Vertex{Position: a, Normal: n}, Vertex{Position: c, Normal: n}, Vertex{Position: d, Normal: n}, m, shadow)
}

// Project each face of a cube onto a rounded cuboid. Shared boundary positions
// and analytic normals keep bevel seams continuous without changing dimensions.
func (b *meshBuilder) box(center, size Vec, m *Material, r float64) {
	if b.err != nil || size.X <= 0 || size.Y <= 0 || size.Z <= 0 {
		return
	}
	half := nmul(size, .5)
	r = min(r, min(size.X, min(size.Y, size.Z))*.24)
	ext := []float64{half.X, half.Y, half.Z}
	core := nv(max(0, half.X-r), max(0, half.Y-r), max(0, half.Z-r))
	for axis := 0; axis < 3; axis++ {
		u := (axis + 1) % 3
		v := (axis + 2) % 3
		coords := func(h float64) []float64 {
			if r < 1e-6 {
				return []float64{-h, h}
			}
			return []float64{-h, -h + r, h - r, h}
		}
		us, vs := coords(ext[u]), coords(ext[v])
		for _, sign := range []float64{-1, 1} {
			vertex := func(x, y float64) Vertex {
				a := [3]float64{}
				a[axis] = ext[axis] * sign
				a[u] = x
				a[v] = y
				p := nv(a[0], a[1], a[2])
				q := nv(max(-core.X, min(core.X, p.X)), max(-core.Y, min(core.Y, p.Y)), max(-core.Z, min(core.Z, p.Z)))
				n := nunit(nsub(p, q))
				if r < 1e-6 {
					a = [3]float64{}
					a[axis] = sign
					n = nv(a[0], a[1], a[2])
				} else {
					p = nadd(q, nmul(n, r))
				}
				return Vertex{Position: nadd(center, p), Normal: n}
			}
			for i := 1; i < len(us); i++ {
				for j := 1; j < len(vs); j++ {
					a, c, d, e := vertex(us[i-1], vs[j-1]), vertex(us[i], vs[j-1]), vertex(us[i], vs[j]), vertex(us[i-1], vs[j])
					if sign > 0 {
						b.triangle(a, c, d, m, true)
						b.triangle(a, d, e, m, true)
					} else {
						b.triangle(a, d, c, m, true)
						b.triangle(a, e, d, m, true)
					}
				}
			}
		}
	}
}

func (b *meshBuilder) sphere(center, radii Vec, m *Material, segments, rings int) {
	for j := 0; j < rings; j++ {
		for i := 0; i < segments; i++ {
			vertex := func(i, j int) Vertex {
				phi := 2 * math.Pi * float64(i) / float64(segments)
				theta := math.Pi * float64(j) / float64(rings)
				p := nv(math.Sin(theta)*math.Cos(phi), math.Cos(theta), math.Sin(theta)*math.Sin(phi))
				n := nunit(nv(p.X/max(radii.X, 1e-8), p.Y/max(radii.Y, 1e-8), p.Z/max(radii.Z, 1e-8)))
				return Vertex{Position: nadd(center, nv(p.X*radii.X, p.Y*radii.Y, p.Z*radii.Z)), Normal: n}
			}
			a, c, d, e := vertex(i, j), vertex(i+1, j), vertex(i+1, j+1), vertex(i, j+1)
			b.triangle(a, c, d, m, true)
			b.triangle(a, d, e, m, true)
		}
	}
}

// Elliptic cylinders and frusta, optionally laid along X for queue barrels.
func (b *meshBuilder) cylinder(center Vec, rx, rz, height, topRatio float64, body, cap *Material, segments int, horizontal bool) {
	transform := func(p Vec) Vec {
		if horizontal {
			return nadd(center, nv(p.Y, -p.X, p.Z))
		}
		return nadd(center, p)
	}
	normal := func(p Vec) Vec {
		if horizontal {
			return nv(p.Y, -p.X, p.Z)
		}
		return p
	}
	for i := 0; i < segments; i++ {
		vertex := func(i int, top bool) Vertex {
			a := 2 * math.Pi * float64(i) / float64(segments)
			ratio, y := 1., -height/2
			if top {
				ratio, y = topRatio, height/2
			}
			p := nv(rx*ratio*math.Cos(a), y, rz*ratio*math.Sin(a))
			n := nunit(nv(math.Cos(a)/max(rx, 1e-8), (1-topRatio)/max(height, 1e-8), math.Sin(a)/max(rz, 1e-8)))
			return Vertex{Position: transform(p), Normal: normal(n)}
		}
		a, c, d, e := vertex(i, false), vertex(i+1, false), vertex(i+1, true), vertex(i, true)
		b.triangle(a, e, d, body, true)
		b.triangle(a, d, c, body, true)
		for _, top := range []bool{false, true} {
			p, q := vertex(i, top), vertex(i+1, top)
			y, n := -height/2, nv(0, -1, 0)
			if top {
				y, n = height/2, nv(0, 1, 0)
			}
			p.Normal = normal(n)
			q.Normal = p.Normal
			mid := Vertex{Position: transform(nv(0, y, 0)), Normal: p.Normal}
			if top {
				b.triangle(mid, q, p, cap, true)
			} else {
				b.triangle(mid, p, q, cap, true)
			}
		}
	}
}

func (b *meshBuilder) label(text string, s labelSurface, style d2target.Text, ink string, opacity float64, kind string) {
	if b.err != nil || text == "" || s.width <= 0 || s.depth <= 0 || opacity <= 0 {
		return
	}
	style.Label = text
	printStyle := b.printStyle(s, style, ink, opacity, kind)
	var tex *image.RGBA
	var err error
	rich := isRichLabel(d2target.Shape{Text: style})
	if rich {
		tex, err = b.rich.texture(d2target.Shape{Text: style}, printStyle)
	} else {
		tex, _, err = b.text.texture(text, printStyle)
	}
	if err != nil {
		b.err = err
		return
	}
	b.surfaceTexture(tex, s, 1)
	if rich {
		b.addDocumentLinks(b.rich.lastDocument, s)
	}
}

func (b *meshBuilder) printStyle(s labelSurface, style d2target.Text, ink string, opacity float64, kind string) labelTextStyle {
	fontSize := float64(style.FontSize)
	if fontSize <= 0 {
		fontSize = 23.5
		if kind == "board" {
			fontSize = 24.5
		}
		if kind == "edge" {
			fontSize = 19
		}
	}
	var bg *color.NRGBA
	if style.LabelFill != "" && !nativeToken(style.LabelFill) {
		c := nativePaint(style.LabelFill, "transparent")
		bg = &c
		if c.A > 0 && nativeToken(style.Color) {
			ink = readableSurfaceInk(style.LabelFill, opacity)
		}
	}
	return labelTextStyle{Width: s.width, Depth: s.depth, FontSize: fontSize, PixelScale: b.scale, FontFamily: style.FontFamily, Bold: style.Bold, Italic: style.Italic, Underline: style.Underline, Color: nativePaint(ink, "#253650"), Background: bg, Opacity: opacity, Align: s.align, MaxLines: 64}
}

func (b *meshBuilder) shapeLabel(original d2target.Shape, s labelSurface, ink string, opacity float64, kind string) {
	if b.err != nil || s.width <= 0 || s.depth <= 0 || opacity <= 0 {
		return
	}
	if !isRichLabel(original) {
		b.label(original.Label, s, original.Text, ink, opacity, kind)
		return
	}
	tex, err := b.rich.texture(original, b.printStyle(s, original.Text, ink, opacity, kind))
	if err != nil {
		b.err = err
		return
	}
	b.surfaceTexture(tex, s, 1)
	b.addDocumentLinks(b.rich.lastDocument, s)
}

func (b *meshBuilder) shapeIcon(original d2target.Shape, s labelSurface, opacity float64, kind string, radius ...float64) labelSurface {
	if b.err != nil || original.Icon == nil || opacity <= 0 {
		return s
	}
	icon, text := surfaceIconLayout(s, original, b.scale, kind)
	if icon.width <= 0 || icon.depth <= 0 {
		return s
	}
	cornerRadius := float64(original.IconBorderRadius)
	if len(radius) > 0 {
		cornerRadius = radius[0]
	}
	tex, err := b.icons.texture(original.Icon, icon.width, icon.depth, cornerRadius*b.scale)
	if err != nil {
		b.err = fmt.Errorf("isometric icon for %q: %w", original.ID, err)
		return s
	}
	b.surfaceTexture(tex, icon, opacity)
	return text
}

// Default ink follows the physical face, while an authored font color remains
// authoritative. This also keeps ordinary labels readable on dark components.
func readableSurfaceInk(fill string, opacity float64) string {
	c := nativePaint(fill, "#edf1f7")
	a := float64(c.A) / 255 * opacity
	luminance := 0.
	for i, value := range []uint8{c.R, c.G, c.B} {
		v := (float64(value)*a + 247*(1-a)) / 255
		if v <= .04045 {
			v /= 12.92
		} else {
			v = math.Pow((v+.055)/1.055, 2.4)
		}
		luminance += v * []float64{.2126, .7152, .0722}[i]
	}
	if luminance < .24 {
		return "#f5f8fc"
	}
	return "#253650"
}

// Surface artwork shares the same physical plane and camera projection as its
// component. Alpha is composited without obscuring the material underneath.
func (b *meshBuilder) surfaceTexture(tex *image.RGBA, s labelSurface, opacity float64) {
	if b.err != nil || tex == nil || s.width <= 0 || s.depth <= 0 || opacity <= 0 {
		return
	}
	m := &Material{Color: color.NRGBA{255, 255, 255, uint8(math.Round(255 * min(1, opacity)))}, Texture: tex, Unlit: true}
	m.Vector = nativeVectorForTexture(b.ctx, tex)
	u := nv(math.Cos(s.angle), 0, math.Sin(s.angle))
	v := nv(-math.Sin(s.angle), 0, math.Cos(s.angle))
	vert := func(x, z float64) Vertex {
		return Vertex{Position: nadd(s.center, nadd(nmul(u, (x-.5)*s.width), nmul(v, (z-.5)*s.depth))), Normal: nv(0, 1, 0), U: x, V: z}
	}
	start := len(b.triangles)
	b.triangle(vert(0, 0), vert(0, 1), vert(1, 1), m, false)
	b.triangle(vert(0, 0), vert(1, 1), vert(1, 0), m, false)
	for i := start; i < len(b.triangles); i++ {
		b.triangles[i].NoDepthWrite = true
		b.triangles[i].DepthBias = .0005
	}
}

func newNativeScene(ctx context.Context, s *d2isometric.Scene, width, height int, assets ...*d2scenebuild.AssetOptions) (*nativeScene, error) {
	var assetOptions *d2scenebuild.AssetOptions
	if len(assets) > 0 {
		assetOptions = assets[0]
	}
	return newNativeSceneWithFonts(ctx, s, width, height, assetOptions, nil)
}

func newNativeSceneWithFonts(ctx context.Context, s *d2isometric.Scene, width, height int, assetOptions *d2scenebuild.AssetOptions, fonts *d2scenebuild.FontFallbackOptions) (*nativeScene, error) {
	return newNativeSceneWithOptions(ctx, s, width, height, assetOptions, fonts, nativeSceneOptions{})
}

func newNativeSceneWithOptions(ctx context.Context, s *d2isometric.Scene, width, height int, assetOptions *d2scenebuild.AssetOptions, fonts *d2scenebuild.FontFallbackOptions, options nativeSceneOptions) (*nativeScene, error) {
	labelCount := len(s.Boards) + len(s.Nodes) + len(s.Edges)*3 + 2
	painter, err := newTextPainter(ctx, labelCount)
	if err != nil {
		return nil, err
	}
	painter.configureFontFamilies(s.FontFamily, s.MonoFontFamily)
	painter.configureFallbackFonts(fonts)
	painter.configureOutputDensity(options.outputDensity)
	richCount, iconCount := 0, 0
	if isRichLabel(s.Root) {
		richCount++
	}
	if s.Root.Icon != nil {
		iconCount++
	}
	for _, node := range s.Nodes {
		if isRichLabel(node.Metadata.Original) {
			richCount++
		}
		if node.Icon != "" {
			iconCount++
		}
	}
	for _, edge := range s.Edges {
		for _, text := range []*d2target.Text{&edge.Metadata.Original.Text, edge.SourceLabel, edge.TargetLabel} {
			if text != nil && isRichLabel(d2target.Shape{Text: *text}) {
				richCount++
			}
		}
		if edge.Icon != "" {
			iconCount++
		}
	}
	rich, err := newRichLabelPainter(ctx, richCount)
	if err != nil {
		return nil, err
	}
	rich.configureFontFamilies(s.FontFamily, s.MonoFontFamily)
	rich.configureTheme(s.ThemeID, s.ThemeOverrides)
	rich.configureFallbackFonts(fonts)
	rich.configureOutputDensity(options.outputDensity)
	icons, err := newSurfaceIconPainter(ctx, iconCount, assetOptions)
	if err != nil {
		return nil, err
	}
	icons.configureOutputDensity(options.outputDensity)
	b := &meshBuilder{ctx: ctx, text: painter, rich: rich, icons: icons, scale: s.PixelScale, fonts: fonts, options: options, outputDensity: options.outputDensity}
	b.faceMaxPixels = max(64, 24*1024*1024/int64(max(1, len(s.Nodes)+len(s.Boards))))
	theme := d2themescatalog.Find(s.ThemeID)
	theme.ApplyOverrides(s.ThemeOverrides)
	b.arrowBackground = d2themes.ResolveThemeColor(theme, d2target.BG_COLOR)
	captions := newRouteCaptionPlacer()
	if b.scale <= 0 {
		b.scale = .01
	}
	colors := []string{"#7898ba", "#9287b7", "#7da89c", "#b69b80"}
	levels := []int{}
	seen := map[int]bool{}
	for _, board := range s.Boards {
		if !seen[board.Level] {
			levels = append(levels, board.Level)
			seen[board.Level] = true
		}
	}
	sort.Ints(levels)
	nodes := map[string]*d2isometric.Node{}
	for i := range s.Nodes {
		nodes[s.Nodes[i].ID] = &s.Nodes[i]
	}
	tints := map[string]string{}
	boards := hierarchyPresentationBoards(s.Boards, nodes)
	hierarchyBackground := "#f5f7fb"
	b.routeCasingFloor = hierarchyCasingFloor(boards, nodes)
	b.hierarchySupports = hierarchySupportOffsets(boards)
	headerNodes := hierarchyRenderNodes(s.Nodes, b.hierarchySupports)
	if !nativeToken(s.Root.Fill) {
		hierarchyBackground = s.Background
	}
	boardIndex := map[string]d2isometric.Board{}
	var shadowSpans []hierarchyShadowSpan
	for _, board := range boards {
		boardIndex[board.ID] = board
		tint := colors[sort.SearchInts(levels, board.Level)%len(colors)]
		tints[board.ID] = hierarchyBoardTint(nodes[board.SourceID], tint)
	}
	for _, board := range boards {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		owner := nodes[board.SourceID]
		opacity := 1.
		tint := colors[sort.SearchInts(levels, board.Level)%len(colors)]
		if owner != nil {
			opacity = owner.Opacity
		}
		tint = hierarchyBoardTint(owner, tint)
		tints[board.ID] = tint
		if opacity <= 0 {
			continue
		}
		w, d := max(.01, board.Size.X), max(.01, board.Size.Z)
		firstBoardTriangle, firstBoardAnimation := len(b.triangles), len(b.animatedNodes)
		b.hierarchyBoard(board, owner, tint, opacity)

		if board.Label != "" || owner != nil && owner.Icon != "" {
			style := d2target.Text{}
			ink := hierarchyHeaderInk(board, boardIndex, nodes, tints, hierarchyBackground)
			if owner != nil {
				style = owner.Metadata.Original.Text
				if owner.FontExplicit {
					ink = owner.FontColor
				}
			}
			hd := board.HeaderDepth
			if hd <= 0 {
				hd = .62
			}
			lw, lh := float64(style.LabelWidth), float64(style.LabelHeight)
			if lw <= 0 {
				lw = float64(len([]rune(board.Label))) * 14
			}
			if lh <= 0 {
				lh = 30
			}
			pw, pd := min(w, lw*b.scale), min(d, lh*b.scale)
			if owner != nil && owner.Icon != "" {
				pw = min(max(0, w-.7), pw+pd+.12)
			}
			surface := labelSurface{center: nadd(board.Position, nv(-w/2+.35+pw/2, .032, -d/2+hd/2)), width: pw, depth: pd, align: "left"}
			if owner != nil {
				surface.center.Y = hierarchySurfaceY(board) + .00006
				surface = hierarchyBoardHeaderSurface(surface, board, *owner, headerNodes, b.scale, boards)
			}
			captions.Avoid(surface.center, pw, pd)
			if owner != nil {
				surface = b.shapeIcon(owner.Metadata.Original, surface, opacity, "board")
			}
			if owner != nil {
				original := owner.Metadata.Original
				if owner.FontExplicit && nativeToken(original.LabelFill) && hierarchyHeaderInk(board, boardIndex, nodes, tints, hierarchyBackground) == readableSurfaceInk(ink, 1) {
					// Preserve authored light ink when a dark source panel becomes
					// a pale organizational wash. A small printed backing keeps
					// the title readable without restoring a full opaque plate.
					original.LabelFill = tint
					if nativePaint(tint, "transparent").A == 0 || readableSurfaceInk(tint, 1) == readableSurfaceInk(ink, 1) {
						original.LabelFill = readableSurfaceInk(ink, 1)
					}
				}
				b.shapeLabel(original, surface, ink, opacity, "board")
			} else {
				b.label(board.Label, surface, style, ink, opacity, "board")
			}
		}
		if owner != nil {
			nativePhysicalShadows(b.triangles[firstBoardTriangle:], owner.Metadata.Original.Shadow)
			b.addMeshLink(owner.Metadata.Original.Link, owner.Metadata.Original.Tooltip, b.triangles[firstBoardTriangle:])
			if owner.Metadata.Original.Animated && len(b.triangles) > firstBoardTriangle {
				if len(b.animatedNodes) == firstBoardAnimation {
					b.animatedNodes = append(b.animatedNodes, nativeAnimatedNode{first: firstBoardTriangle, last: len(b.triangles)})
				} else {
					b.animatedNodes[firstBoardAnimation].last = len(b.triangles)
				}
			}
		}
		shadowSpans = append(shadowSpans, hierarchyShadowSpan{firstBoardTriangle, len(b.triangles), board.ID})
		if owner != nil {
			boardCopy := board
			b.rememberArrowOwner(*owner, &boardCopy, firstBoardTriangle)
		}
	}
	for _, node := range s.Nodes {
		if !node.Container && node.Opacity > 0 {
			if hierarchySpacer(node) {
				continue
			}
			first := len(b.triangles)
			if node.SequenceRole != "" {
				b.sequenceNode(node, tints[node.BoardID])
			} else {
				b.hierarchyNode(node, tints[node.BoardID])
			}
			captions.AvoidMesh(b.triangles[first:])
			b.addMeshLink(node.Metadata.Original.Link, node.Metadata.Original.Tooltip, b.triangles[first:])
			shadowSpans = append(shadowSpans, hierarchyShadowSpan{first, len(b.triangles), node.BoardID})
			b.rememberArrowOwner(node, nil, first)
		}
	}
	packets := b.edges(s.Edges, captions, s)
	if b.err != nil {
		return nil, b.err
	}
	if err := b.rootDecorations(s, assetOptions); err != nil {
		return nil, err
	}
	hierarchyShadowReceivers(b.triangles, boards, shadowSpans)
	return finishNativeScene(b, s, width, height, packets)
}

func (b *meshBuilder) node(n d2isometric.Node, tint string, relief ...float64) {
	n = nativeClassicNode(n)
	first := len(b.triangles)
	firstLink := len(b.links)
	if nativeMarkdownCard(n) {
		b.markdownCard(n, tint)
	} else if nativeStructuredNode(n) {
		b.structuredNode(n)
	} else if nativeSolidNode(n) {
		b.solidNode(n)
	} else {
		b.canonicalNode(n, tint)
	}
	factor := 1.
	if len(relief) > 0 {
		factor = relief[0]
	}
	var before Vec
	if len(b.triangles) > first {
		before = b.triangles[len(b.triangles)-1].V[0].Position
	}
	if n.SequenceRole == "" && !nativeStructuredNode(n) && !nativeMarkdownCard(n) {
		b.clearOutsideCaption(n, first, factor)
	}
	if len(b.triangles) > first {
		delta := nsub(b.triangles[len(b.triangles)-1].V[0].Position, before)
		for i := firstLink; i < len(b.links); i++ {
			for j := range b.links[i].points {
				b.links[i].points[j] = nadd(b.links[i].points[j], delta)
			}
		}
	}
	if len(relief) == 0 && !nativeStructuredNode(n) && !nativeMarkdownCard(n) {
		b.classicInkEdges(n, b.triangles[first:])
	}
	nativePhysicalShadows(b.triangles[first:], n.Metadata.Original.Shadow)
	if n.Metadata.Original.Animated && len(b.triangles) > first {
		b.animatedNodes = append(b.animatedNodes, nativeAnimatedNode{first: first, last: len(b.triangles)})
	}
	b.markPaintOwner(first)
}

// Physical contact shadows establish depth independently of D2's optional
// graphic drop-shadow style. An authored shadow strengthens that treatment;
// ordinary 3D components still cast a restrained shadow onto their surface.
func nativePhysicalShadows(triangles []Triangle, authored bool) {
	strength := .72
	if authored {
		strength = 1
	}
	for i := range triangles {
		if triangles[i].CastShadow {
			triangles[i].ShadowOpacity = &strength
		}
	}
}

func (s *nativeScene) Frame(ctx context.Context, seconds float64, traffic bool, packetSeconds ...float64) (*image.RGBA, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return nil, fmt.Errorf("isometric frame time must be finite and nonnegative")
	}
	var dynamic []Triangle
	if traffic {
		b := &meshBuilder{ctx: ctx}
		packetTime := seconds
		if len(packetSeconds) > 0 {
			packetTime = packetSeconds[0]
		}
		if math.IsNaN(packetTime) || math.IsInf(packetTime, 0) || packetTime < 0 {
			return nil, fmt.Errorf("isometric packet time must be finite and nonnegative")
		}
		phase := packetTime / CycleSeconds
		phase -= math.Floor(phase)
		if phase < 1e-12 || 1-phase < 1e-12 {
			phase = 0
		}
		for _, p := range s.packets {
			for i := 0; i < 2; i++ {
				t := math.Mod(phase+float64(i)/2, 1)
				if p.reverse && (!p.forward || i%2 != 0) {
					t = 1 - t
				}
				point := pathPoint(p.points, p.lengths, t)
				b.sphere(point, nv(nativeTrafficRadius, nativeTrafficRadius, nativeTrafficRadius), p.material, 10, 8)
			}
		}
		if b.err != nil {
			return nil, b.err
		}
		dynamic = b.triangles
	}
	raster, err := s.frameRaster(ctx, seconds, traffic)
	if err != nil {
		return nil, err
	}
	return raster.Frame(ctx, dynamic)
}
