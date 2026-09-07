package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"unicode/utf8"

	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

// Page combines the native PNG with annotations in final image pixels. Writers
// can resize the image and annotations together for a PDF page or PPTX slide.
type Page struct {
	PNG           []byte
	Links         []d2scene.LinkRegion
	Width, Height int
}

type nativeLink struct {
	points [4]Vec
	region d2scene.LinkRegion
}

// RenderPage renders one compiled board and returns its image and bounded
// annotation regions for D2's native PDF/PPTX writers.
func RenderPage(ctx context.Context, diagram *d2target.Diagram, opts *Options) (*Page, error) {
	o, err := normalize(opts)
	if err != nil {
		return nil, err
	}
	if o.Format != PNG {
		return nil, fmt.Errorf("isometric page export requires PNG rendering")
	}
	if o.LinkBudget == nil {
		o.LinkBudget = &d2scenebuild.LinkBudget{MaxRegions: 4096, MaxStringBytes: 1 << 20}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := preflightPageMetadata(ctx, diagram, *o.LinkBudget); err != nil {
		return nil, err
	}
	s, err := openCapture(ctx, diagram, o)
	if err != nil {
		return nil, err
	}
	defer s.close()
	data, err := s.frame(0, false)
	if err != nil {
		return nil, err
	}
	links, err := projectPageLinks(s.ctx, s.scene)
	if err != nil {
		return nil, err
	}
	return &Page{PNG: data, Links: links, Width: s.opts.Width, Height: s.opts.Height}, nil
}

func preflightPageMetadata(ctx context.Context, diagram *d2target.Diagram, budget d2scenebuild.LinkBudget) error {
	if diagram == nil {
		return fmt.Errorf("isometric page requires a diagram")
	}
	regions, bytes := 0, 0
	check := func(link, tooltip, pretty string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if pretty != "" && link == "" {
			return fmt.Errorf("isometric page prettyLink requires a non-empty link")
		}
		if link == "" && tooltip == "" {
			return nil
		}
		regions++
		if regions > budget.MaxRegions {
			return fmt.Errorf("isometric page link regions exceed %d", budget.MaxRegions)
		}
		for _, v := range []string{link, tooltip} {
			if len(v) > budget.MaxStringBytes-bytes {
				return fmt.Errorf("isometric page link metadata exceeds %d bytes", budget.MaxStringBytes)
			}
			bytes += len(v)
			if err := validatePageLinkText(ctx, v); err != nil {
				return err
			}
		}
		if link != "" && tooltip != "" {
			if p, err := url.ParseRequestURI(tooltip); err == nil && p.Host != "" {
				return fmt.Errorf("isometric page tooltip must not be a URL when link is also set")
			}
		}
		return nil
	}
	for _, shape := range diagram.Shapes {
		if err := check(shape.Link, shape.Tooltip, shape.PrettyLink); err != nil {
			return err
		}
	}
	for _, edge := range diagram.Connections {
		if err := check(edge.Link, edge.Tooltip, edge.PrettyLink); err != nil {
			return err
		}
		if edge.Link != "" || edge.Tooltip != "" {
			if edge.Label == "" && edge.Icon == nil {
				return fmt.Errorf("isometric page connection %q needs a label or icon for its metadata hit region", edge.ID)
			}
		}
	}
	return ctx.Err()
}

func (b *meshBuilder) addNativeLink(points [4]Vec, region d2scene.LinkRegion) {
	if b.options.links == nil || b.err != nil || region.URL == "" && region.Target == "" && region.Tooltip == "" {
		return
	}
	budget := b.options.links
	if len(b.links) >= budget.MaxRegions {
		b.err = fmt.Errorf("isometric page link regions exceed %d", budget.MaxRegions)
		return
	}
	bytes := len(region.URL) + len(region.Target) + len(region.Tooltip)
	if bytes > budget.MaxStringBytes-b.linkBytes {
		b.err = fmt.Errorf("isometric page link metadata exceeds %d bytes", budget.MaxStringBytes)
		return
	}
	for _, v := range []string{region.URL, region.Target, region.Tooltip} {
		if err := validatePageLinkText(b.ctx, v); err != nil {
			b.err = err
			return
		}
	}
	for _, p := range points {
		if !captionFinite(p.X, p.Y, p.Z) {
			b.err = fmt.Errorf("isometric page link bounds are non-finite")
			return
		}
	}
	b.linkBytes += bytes
	b.links = append(b.links, nativeLink{points: points, region: region})
}

// Match the existing native PDF/PPTX metadata contract. Reject unsupported
// characters before the XML writer could replace them in a destination.
func validatePageLinkText(ctx context.Context, value string) error {
	for offset := 0; offset < len(value); {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, size := utf8.DecodeRuneInString(value[offset:])
		if r == utf8.RuneError && size == 1 {
			return fmt.Errorf("isometric page link metadata must be valid UTF-8")
		}
		if !(r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xd7ff || r >= 0xe000 && r <= 0xfffd || r >= 0x10000 && r <= 0x10ffff) {
			return fmt.Errorf("isometric page link metadata contains a character forbidden by XML 1.0")
		}
		offset += size
	}
	return ctx.Err()
}

func (b *meshBuilder) validateTypedLink(link, tooltip string) bool {
	if b.options.vector {
		return true // SVG preserves arbitrary source tooltip text in a title.
	}
	if link != "" && tooltip != "" {
		if parsed, err := url.ParseRequestURI(tooltip); err == nil && parsed.Host != "" {
			b.err = fmt.Errorf("isometric page tooltip must not be a URL when link is also set")
			return false
		}
	}
	return true
}

func nativeLinkDestination(link, tooltip string) d2scene.LinkRegion {
	r := d2scene.LinkRegion{URL: link, Tooltip: tooltip}
	if key, err := d2parser.ParseKey(link); err == nil && len(key.Path) > 0 && key.Path[0].Unbox().ScalarString() == "root" {
		r.URL, r.Target = "", link
	}
	return r
}

func surfaceCorners(s labelSurface) [4]Vec {
	u, v := nv(math.Cos(s.angle), 0, math.Sin(s.angle)), nv(-math.Sin(s.angle), 0, math.Cos(s.angle))
	point := func(x, y float64) Vec { return nadd(s.center, nadd(nmul(u, x*s.width), nmul(v, y*s.depth))) }
	return [4]Vec{point(-.5, -.5), point(.5, -.5), point(.5, .5), point(-.5, .5)}
}

func (b *meshBuilder) addSurfaceLink(link, tooltip string, s labelSurface) {
	if b.options.links == nil || link == "" && tooltip == "" || s.width <= 0 || s.depth <= 0 {
		return
	}
	if !b.validateTypedLink(link, tooltip) {
		return
	}
	b.addNativeLink(surfaceCorners(s), nativeLinkDestination(link, tooltip))
}

func (b *meshBuilder) addMeshLink(link, tooltip string, triangles []Triangle) {
	if b.options.links == nil || link == "" && tooltip == "" || len(triangles) == 0 {
		return
	}
	if !b.validateTypedLink(link, tooltip) {
		return
	}
	var e projectedExtent
	e.mesh(triangles)
	c := nativeCameraAxes()
	point := func(x, y float64) Vec { return nadd(nmul(c.right, x), nadd(nmul(c.up, y), nmul(c.direction, e.maxZ))) }
	b.addNativeLink([4]Vec{point(e.minX, e.minY), point(e.maxX, e.minY), point(e.maxX, e.maxY), point(e.minX, e.maxY)}, nativeLinkDestination(link, tooltip))
}

// Map source-document link boxes through the same meet/stretch viewport used
// for its texture, and then through the physical surface's orientation.
func (b *meshBuilder) addDocumentLinks(doc *d2scene.Document, s labelSurface) {
	if b.options.links == nil || doc == nil || len(doc.Links) == 0 || b.err != nil {
		return
	}
	if !captionFinite(doc.ViewBox.Width, doc.ViewBox.Height, s.width, s.depth) || doc.ViewBox.Width <= 0 || doc.ViewBox.Height <= 0 || s.width <= 0 || s.depth <= 0 {
		b.err = fmt.Errorf("isometric page document link viewport is invalid")
		return
	}
	scaleX, scaleY := s.width/doc.ViewBox.Width, s.depth/doc.ViewBox.Height
	ox, oy := 0., 0.
	if doc.ViewportFit == d2scene.ViewportMeet {
		// Surface aspect equals the raster viewport aspect except for rounded
		// texture dimensions; use logical pixels to retain its exact letterbox.
		pw, ph := doc.LogicalWidth, doc.LogicalHeight
		if pw <= 0 || ph <= 0 {
			pw, ph = s.width, s.depth
		}
		uniform := min(pw/doc.ViewBox.Width, ph/doc.ViewBox.Height)
		scaleX, scaleY = uniform*s.width/pw, uniform*s.depth/ph
		if doc.ViewportAlign == d2scene.ViewportAlignXMidYMid {
			ox, oy = (s.width-doc.ViewBox.Width*scaleX)/2, (s.depth-doc.ViewBox.Height*scaleY)/2
		}
	}
	u, v := nv(math.Cos(s.angle), 0, math.Sin(s.angle)), nv(-math.Sin(s.angle), 0, math.Cos(s.angle))
	point := func(x, y float64) Vec { return nadd(s.center, nadd(nmul(u, x-s.width/2), nmul(v, y-s.depth/2))) }
	for _, r := range doc.Links {
		x0, y0 := (r.Box.X-doc.ViewBox.X)*scaleX+ox, (r.Box.Y-doc.ViewBox.Y)*scaleY+oy
		x1, y1 := x0+r.Box.Width*scaleX, y0+r.Box.Height*scaleY
		x0, y0, x1, y1 = max(0., x0), max(0., y0), min(s.width, x1), min(s.depth, y1)
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		b.addNativeLink([4]Vec{point(x0, y0), point(x1, y0), point(x1, y1), point(x0, y1)}, r)
		if b.err != nil {
			return
		}
	}
}

func projectPageLinks(ctx context.Context, s *nativeScene) ([]d2scene.LinkRegion, error) {
	links := make([]d2scene.LinkRegion, 0, len(s.links))
	c := cameraAtResolution(s.raster.camera, s.width, s.height)
	for _, link := range s.links {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, v := range link.points {
			p := c.project(v)
			x0, y0, x1, y1 = min(x0, p.x), min(y0, p.y), max(x1, p.x), max(y1, p.y)
		}
		x0, y0, x1, y1 = max(0., x0), max(0., y0), min(float64(s.width), x1), min(float64(s.height), y1)
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		r := link.region
		r.Box = d2scene.Box{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
		links = append(links, r)
	}
	// Broad container/shape regions precede smaller inline links so document
	// viewers can activate the most specific overlapping annotation.
	sort.SliceStable(links, func(i, j int) bool {
		return links[i].Box.Width*links[i].Box.Height > links[j].Box.Width*links[j].Box.Height
	})
	return links, nil
}
