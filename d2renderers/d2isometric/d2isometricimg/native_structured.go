package d2isometricimg

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func nativeStructuredNode(n d2isometric.Node) bool {
	return n.Type == d2target.ShapeClass || n.Type == d2target.ShapeSQLTable
}

// Row coordinates are the compiler's original document coordinates. Gutters
// consume existing vertical padding, never the allocated row centers or ports.
type nativeStructuredRow struct {
	z, depth    float64
	back, front float64
	section     string
}

func nativeStructuredRows(s d2target.Shape) []nativeStructuredRow {
	count := len(s.Columns)
	headerRows := 1
	if s.Type == d2target.ShapeClass {
		count, headerRows = len(s.Fields)+len(s.Methods), 2
	}
	rh := float64(s.Height) / float64(count+headerRows)
	hh := rh * float64(headerRows)
	if s.Type == d2target.ShapeClass {
		hh = max(hh, float64(s.LabelHeight)+10)
	}
	rows := []nativeStructuredRow{{depth: hh, section: "header"}}
	for i := 0; i < count; i++ {
		section := "field"
		if s.Type == d2target.ShapeClass && i >= len(s.Fields) {
			section = "method"
		}
		z := hh + float64(i)*rh
		// A multiline class header can use more than its nominal two rows.
		// The ordinary renderer clips the last cells at the source viewport;
		// keep that same boundary instead of extending the physical component.
		rows = append(rows, nativeStructuredRow{z: z, depth: min(rh, max(0, float64(s.Height)-z)), section: section})
	}
	for i := range rows {
		r := &rows[i]
		gap := 3.8
		if i == 0 {
			gap = 1.2
		}
		// Very small or explicitly tight rows keep all available text space.
		padding := max(0, (r.depth-float64(s.FontSize))/2-1)
		r.back, r.front = min(gap, padding*.5), min(gap, padding*.5)
		if i > 1 && r.section != rows[i-1].section {
			r.back = min(7.5, padding*.8)
		}
		if i > 0 && i+1 < len(rows) && r.section != rows[i+1].section {
			r.front = min(7.5, padding*.8)
		}
	}
	return rows
}

func nativeStructuredTextRow(id string, s d2target.Shape) int {
	if strings.Contains(id, "header-label") {
		return 0
	}
	parts := strings.Split(id, ":")
	if len(parts) < 4 {
		return -1
	}
	i, err := strconv.Atoi(parts[2])
	if err != nil {
		return -1
	}
	if parts[1] == "class-method" {
		i += len(s.Fields)
	}
	return i + 1
}

type nativeStructuredCell struct {
	node *d2scene.Node
	row  int
}

// Keep the existing source, row, font and texture admission limits. The scene
// supplies native TextRuns, including exact baselines and resolved font assets;
// only those individual cells are rasterized onto the physical row solids.
func (b *meshBuilder) structuredDocument(n d2isometric.Node) (*d2scene.Document, []nativeStructuredCell, error) {
	p := b.rich
	if p == nil {
		return nil, nil, fmt.Errorf("native structured shape requires a rich label painter")
	}
	if p.used >= p.count {
		return nil, nil, fmt.Errorf("isometric rich label allocation exceeds its declared count")
	}
	bytes, rows, err := richSourceSize(n.Metadata.Original)
	if err != nil {
		return nil, nil, err
	}
	if bytes > maxRichSourceBytes-p.sourceBytes || rows > maxRichTotalRows-p.rows {
		return nil, nil, fmt.Errorf("isometric rich labels exceed aggregate source or row budget")
	}
	p.sourceBytes += bytes
	p.rows += rows
	p.used++
	surface := labelSurface{width: n.Size.X, depth: n.Size.Z}
	style := b.printStyle(surface, n.Metadata.Original.Text, n.FontColor, n.Opacity, "node")
	doc, err := richLabelDocumentWithResources(b.ctx, n.Metadata.Original, style, p.fallbackFonts, p.themeID, p.themeOverrides, p.primary, p.mono)
	if err != nil {
		return nil, nil, err
	}
	var cells []nativeStructuredCell
	var walk func(*d2scene.Node)
	walk = func(node *d2scene.Node) {
		if node == nil {
			return
		}
		if run, ok := node.Primitive.(d2scene.TextRun); ok && run.Text != "" {
			cells = append(cells, nativeStructuredCell{node: node, row: nativeStructuredTextRow(node.ID, n.Metadata.Original)})
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(doc.Root)
	return doc, cells, nil
}

func (b *meshBuilder) structuredNode(n d2isometric.Node) {
	if b.err != nil || n.Opacity <= 0 {
		return
	}
	if n.Opacity < 1 {
		// Opacity belongs to the source object. Resolve visibility between
		// its plinth, rails and printed cells before applying that fade once.
		group := &nativeOpacityGroup{Opacity: n.Opacity}
		first := len(b.triangles)
		n.Opacity = 1
		defer func() {
			for i := first; i < len(b.triangles); i++ {
				b.triangles[i].OpacityGroup = group
			}
		}()
	}
	doc, cells, err := b.structuredDocument(n)
	if err != nil {
		b.err = fmt.Errorf("native structured shape %q: %w", n.ID, err)
		return
	}
	s := n.Metadata.Original
	rows := nativeStructuredRows(s)
	faceBudget := b.faceMaxPixels
	if faceBudget > 0 {
		copies := 1
		if s.Multiple {
			copies++
		}
		b.faceMaxPixels = max(1, faceBudget/int64(copies*(len(rows)+1)))
		defer func() { b.faceMaxPixels = faceBudget }()
	}
	sx, sz := n.Size.X/float64(s.Width), n.Size.Z/float64(s.Height)
	floor := n.Position.Y - n.Size.Y/2
	height := nativeCanonicalHeight(n, b.scale)
	// The shared flat route plane is .01 above the compiled node floor.
	// Begin each row wall there so row ports meet that row's physical wall,
	// rather than ending on a continuous backing below all of the rows.
	base, header, body := floor+.01, floor+height, floor+height*.32/.46
	for copyIndex := 1; copyIndex >= 0; copyIndex-- {
		if copyIndex == 1 && !s.Multiple {
			continue
		}
		copyNode := n
		copyNode.Position.X += float64(copyIndex) * d2target.MULTIPLE_OFFSET * sx
		copyNode.Position.Z -= float64(copyIndex) * d2target.MULTIPLE_OFFSET * sz
		dy := -float64(copyIndex) * min(.045, height*.25)
		b.structuredRail(copyNode, 0, float64(s.Height), floor+dy+b.nodeSupportDrop, base+dy, n.Fill)
		for i, row := range rows {
			top, fill := body, n.Stroke
			if i == 0 {
				top, fill = header, n.Fill
			}
			b.structuredRail(copyNode, row.z+row.back, row.depth-row.back-row.front, base+dy, top+dy, fill)
		}
	}
	for _, cell := range cells {
		if cell.row < 0 || cell.row >= len(rows) {
			b.err = fmt.Errorf("native structured shape %q has an unassigned text cell %q", n.ID, cell.node.ID)
			return
		}
		top := body
		if cell.row == 0 {
			top = header
		}
		b.structuredCell(doc, cell.node, rows[cell.row], n, top, len(cells))
	}
	if s.Icon != nil {
		face := nativeStructuredIconSurface(n, header+.003)
		tex, err := b.icons.texture(s.Icon, face.width, face.depth, 0)
		if err != nil {
			b.err = fmt.Errorf("isometric icon for %q: %w", n.ID, err)
			return
		}
		b.surfaceTexture(tex, face, n.Opacity)
	}
}

func nativeStructuredIconSurface(n d2isometric.Node, y float64) labelSurface {
	s := n.Metadata.Original
	face := labelSurface{center: nv(n.Position.X, y, n.Position.Z), width: n.Size.X, depth: n.Size.Z}
	position := label.FromString(s.IconPosition)
	if !position.IsOutside() {
		// The full source face is needed to locate the existing header;
		// passing only its strip would subdivide that strip a second time.
		icon, _ := surfaceIconLayout(face, s, n.Size.X/float64(s.Width), "node")
		return icon
	}
	// The compiler reserves outside icon space independently of the header.
	// Keep its native placement: narrow SQL headers have no spare inside lane
	// in which to move an outside icon without covering their source label.
	box := geo.NewBox(geo.NewPoint(0, 0), float64(s.Width), float64(s.Height))
	size := float64(d2target.GetIconSize(box, s.IconPosition))
	p := position.GetPointOnBox(box, label.PADDING, size, size)
	sx, sz := n.Size.X/float64(s.Width), n.Size.Z/float64(s.Height)
	return labelSurface{center: nv(n.Position.X-n.Size.X/2+(p.X+size/2)*sx, y, n.Position.Z-n.Size.Z/2+(p.Y+size/2)*sz), width: size * sx, depth: size * sz}
}

func (b *meshBuilder) structuredRail(n d2isometric.Node, z, depth, bottom, top float64, fill string) {
	if depth <= 0 || top <= bottom || b.err != nil {
		return
	}
	s := n.Metadata.Original
	sx, sz := n.Size.X/float64(s.Width), n.Size.Z/float64(s.Height)
	// Use D2's rounded rectangle contour, retaining the source radius and
	// modifiers without replacing a rounded outline with a beveled cuboid.
	face := nativeFaceSource(n, fill)
	face.Type, face.Text = d2target.ShapeRectangle, d2target.Text{}
	face.Fill, face.Stroke = fill, n.Fill
	face.Width, face.Height = s.Width, max(1, int(math.Round(depth)))
	face.LabelPosition = ""
	face.BorderRadius = min(s.BorderRadius, face.Height/2)
	profiles, err := nativeShapeProfiles(face)
	if err != nil {
		b.err = err
		return
	}
	first := len(b.triangles)
	mat := nativeMaterial(fill, .72, 0, n.Opacity)
	decorated := n.Opacity < 1 || nativePaint(n.Fill, "#263c4e").A < 255 || n.StrokeDash != 0 || s.DoubleBorder || s.FillPattern != ""
	cap := mat
	if decorated {
		cap = nil
	}
	for _, profile := range profiles {
		world := make([]Vec, len(profile))
		for i, p := range profile {
			world[i] = nv(n.Position.X-n.Size.X/2+p.X*sx, top, n.Position.Z-n.Size.Z/2+(z+p.Z*depth/float64(face.Height))*sz)
		}
		b.extrudedProfile(world, bottom, cap, mat)
	}
	if decorated {
		tex, ink, area := b.nativeFaceLayers(face, n.Opacity)
		area.center = nv(n.Position.X-n.Size.X/2+area.center.X*sx, top+.0005, n.Position.Z-n.Size.Z/2+(z+area.center.Z*depth/float64(face.Height))*sz)
		area.width, area.depth = area.width*sx, area.depth*depth/float64(face.Height)*sz
		start := len(b.triangles)
		b.surfaceTexture(tex, area, n.Opacity)
		for i := start; i < len(b.triangles); i++ {
			b.triangles[i].Material.Unlit = false
			b.triangles[i].NoDepthWrite = false
			b.triangles[i].CastShadow = true
		}
		b.surfaceTexture(ink, area, n.Opacity)
	}
	ink := n
	ink.Type, ink.Fill, ink.Stroke, ink.StrokeExplicit = d2target.ShapeRectangle, fill, n.Fill, true
	ink.Metadata.Original = face
	b.classicInkEdges(ink, b.triangles[first:])
}

func (b *meshBuilder) structuredCell(doc *d2scene.Document, cell *d2scene.Node, row nativeStructuredRow, n d2isometric.Node, top float64, count int) {
	if b.err != nil {
		return
	}
	s := n.Metadata.Original
	sx, sz := n.Size.X/float64(s.Width), n.Size.Z/float64(s.Height)
	box := d2scene.Box{Y: row.z, Width: float64(s.Width), Height: row.depth}
	if box.Height <= 0 {
		return
	}
	budget := max(1, min(4*1024*1024, maxRichPixels/max(1, b.rich.count))/max(1, count))
	w, h := surfaceTextureDimensionsAtDensity(n.Size.X, row.depth*sz, 4096, budget, b.outputDensity)
	if w*h > maxRichPixels-b.rich.pixels {
		b.err = fmt.Errorf("isometric rich labels exceed texture pixel budget")
		return
	}
	b.rich.pixels += w * h
	// A cell owns just its text primitive. No header, border, row fill or
	// other cell is included in this transparent decal.
	part := *doc
	part.Root = cell
	part.ViewBox, part.LogicalWidth, part.LogicalHeight = box, float64(w), float64(h)
	part.ViewportFit = d2scene.ViewportStretch
	frame, err := d2raster.Render(b.ctx, &part, richRasterOptions())
	if err != nil {
		b.err = err
		return
	}
	tex := image.NewRGBA(frame.Bounds())
	draw.Draw(tex, tex.Bounds(), frame, frame.Bounds().Min, draw.Src)
	if err := retainNativeVectorSurface(b.ctx, tex, &part); err != nil {
		b.err = err
		return
	}
	area := labelSurface{center: nv(n.Position.X, top+.003, n.Position.Z-n.Size.Z/2+(row.z+row.depth/2)*sz), width: box.Width * sx, depth: box.Height * sz}
	b.surfaceTexture(tex, area, n.Opacity)
}
