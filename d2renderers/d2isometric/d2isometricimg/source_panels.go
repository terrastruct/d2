package d2isometricimg

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
)

// A printed panel retains native legend and root-frame decoration. Diagram
// components, including sequence actors and spans, use individual geometry.
type nativePanel struct {
	document      *d2scene.Document
	width, height int
	first, last   int
	animated      bool
}

func nativeSourceTarget(s *d2isometric.Scene) *d2target.Diagram {
	d := d2target.NewDiagram()
	d.Root, d.Legend = s.Root, s.Legend
	d.FontFamily, d.MonoFontFamily = s.FontFamily, s.MonoFontFamily
	d.Config = &d2target.Config{ThemeID: &s.ThemeID, ThemeOverrides: s.ThemeOverrides}
	for _, n := range s.Nodes {
		d.Shapes = append(d.Shapes, n.Metadata.Original)
	}
	for _, e := range s.Edges {
		d.Connections = append(d.Connections, e.Metadata.Original)
	}
	return d
}

func documentSurface(doc *d2scene.Document, scale, y float64) labelSurface {
	v := doc.ViewBox
	return labelSurface{center: nv((v.X+v.Width/2)*scale, y, (v.Y+v.Height/2)*scale), width: v.Width * scale, depth: v.Height * scale}
}

func sceneHasAnimation(n *d2scene.Node) bool {
	if n == nil {
		return false
	}
	if len(n.Animations) > 0 {
		return true
	}
	for _, c := range n.Children {
		if sceneHasAnimation(c) {
			return true
		}
	}
	return n.Mask != nil && sceneHasAnimation(n.Mask.Root)
}

func (b *meshBuilder) sourcePanel(doc *d2scene.Document, surface labelSurface, plannedPanels int) error {
	if plannedPanels < 1 || plannedPanels > 2 {
		return fmt.Errorf("isometric source panel count must be at most a legend and root frame")
	}
	if !captionFinite(doc.ViewBox.Width, doc.ViewBox.Height) || doc.ViewBox.Width <= 0 || doc.ViewBox.Height <= 0 {
		return fmt.Errorf("isometric source panel has invalid dimensions")
	}
	// Unconfigured callers retain source-relative sampling. Output-aware
	// renders reserve an equal share for every planned panel, so a large first
	// panel cannot exhaust the aggregate budget before its root frame appears.
	// Dimensions only control sampling; content is never relaid out.
	ratio := math.Min(2, 2048/math.Max(doc.ViewBox.Width, doc.ViewBox.Height))
	w, h := max(1, int(math.Ceil(doc.ViewBox.Width*ratio))), max(1, int(math.Ceil(doc.ViewBox.Height*ratio)))
	if b.outputDensity > 0 {
		w, h = surfaceTextureDimensionsAtDensity(surface.width, surface.depth, 4096, (8<<20)/plannedPanels, b.outputDensity)
	}
	pixels := int64(w) * int64(h)
	for _, p := range b.panels {
		pixels += int64(p.width) * int64(p.height)
	}
	if pixels > 8<<20 {
		return fmt.Errorf("isometric source panels exceed 8M texture pixels")
	}
	tex, err := rasterNativeSurfaceDocument(b.ctx, doc, w, h)
	if err != nil {
		return err
	}
	first := len(b.triangles)
	b.surfaceTexture(tex, surface, 1)
	linkDocument := *doc
	linkDocument.LogicalWidth, linkDocument.LogicalHeight = float64(w), float64(h)
	linkDocument.ViewportFit, linkDocument.ViewportAlign = d2scene.ViewportMeet, d2scene.ViewportAlignXMidYMid
	b.addDocumentLinks(&linkDocument, surface)
	for i := first; i < len(b.triangles); i++ {
		b.triangles[i].NoDepthWrite = false
	}
	b.panels = append(b.panels, nativePanel{document: doc, width: w, height: h, first: first, last: len(b.triangles), animated: sceneHasAnimation(doc.Root)})
	return b.err
}

func nativeMeshBounds(triangles []Triangle) d2scene.Box {
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, t := range triangles {
		for _, v := range t.V {
			x0, y0 = min(x0, v.Position.X), min(y0, v.Position.Z)
			x1, y1 = max(x1, v.Position.X), max(y1, v.Position.Z)
		}
	}
	if len(triangles) == 0 {
		return d2scene.Box{Width: 1, Height: 1}
	}
	return d2scene.Box{X: x0, Y: y0, Width: max(.01, x1-x0), Height: max(.01, y1-y0)}
}

func nativeBoundsTarget(s *d2isometric.Scene, bounds d2scene.Box, scale float64) *d2target.Diagram {
	d := nativeSourceTarget(s)
	d.Shapes, d.Connections, d.Legend = nil, nil, nil
	shape := d2target.BaseShape()
	shape.ID, shape.Type, shape.Fill, shape.Stroke, shape.Opacity = "native-bounds", d2target.ShapeRectangle, "transparent", "transparent", 0
	shape.Pos = d2target.Point{X: int(math.Floor(bounds.X / scale)), Y: int(math.Floor(bounds.Y / scale))}
	shape.Width, shape.Height = max(1, int(math.Ceil(bounds.Width/scale))), max(1, int(math.Ceil(bounds.Height/scale)))
	d.Shapes = []d2target.Shape{*shape}
	return d
}

func (b *meshBuilder) rootDecorations(s *d2isometric.Scene, assets *d2scenebuild.AssetOptions) error {
	// A legend and an enclosing root border/pattern each need their own panel.
	panelCount := 0
	if s.Legend != nil {
		panelCount++
	}
	if s.Root.StrokeWidth > 0 || s.Root.FillPattern != "" || s.Root.DoubleBorder {
		panelCount++
	}
	if s.Legend != nil {
		bounds := nativeMeshBounds(b.triangles)
		d := nativeBoundsTarget(s, bounds, b.scale)
		d.Legend = s.Legend
		doc, err := nativeSurfaceDocument(b.ctx, d, assets, b.fonts)
		if err != nil {
			return fmt.Errorf("isometric legend: %w", err)
		}
		for _, n := range doc.Root.Children {
			if n.ID != "legend" {
				continue
			}
			for _, c := range n.Children {
				if c.ID != "legend:panel" {
					continue
				}
				r, ok := c.Primitive.(d2scene.Rect)
				if !ok {
					return fmt.Errorf("isometric legend panel is not a rectangle")
				}
				doc.Root = n
				doc.ViewBox = d2scene.Box{X: r.Box.X - 12, Y: r.Box.Y - 12, Width: r.Box.Width + 24, Height: r.Box.Height + 24}
				doc.LogicalWidth, doc.LogicalHeight = doc.ViewBox.Width, doc.ViewBox.Height
				if err := b.sourcePanel(doc, documentSurface(doc, b.scale, .065), panelCount); err != nil {
					return err
				}
				break
			}
			break
		}
	}
	root := s.Root
	if root.Label != "" || root.Icon != nil || s.Description != "" {
		bounds := nativeMeshBounds(b.triangles)
		theme := d2themescatalog.Find(s.ThemeID)
		theme.ApplyOverrides(s.ThemeOverrides)
		root.Color, root.LabelFill = d2themes.ResolveThemeColor(theme, root.Color), d2themes.ResolveThemeColor(theme, root.LabelFill)
		if root.FontSize <= 0 {
			root.FontSize = 28
		}
		font := float64(root.FontSize) * b.scale
		w := max(bounds.Width, float64(root.LabelWidth)*b.scale)
		if root.Icon != nil {
			w = max(w, font*4)
		}
		h := max(font*1.5, float64(root.LabelHeight)*b.scale)
		// A target built directly (without compilation) may not have metrics.
		if root.LabelHeight <= 0 {
			lines := 0
			for _, line := range strings.Split(root.Label, "\n") {
				lines += max(1, int(math.Ceil(float64(len([]rune(line)))*font*.56/w)))
			}
			h = max(h, float64(lines)*font*1.4)
		}
		bottom := bounds.Y - .35
		ink := readableSurfaceInk(s.Background, 1)
		if !nativeToken(root.Color) {
			ink = root.Color
		}
		if s.Description != "" {
			style := root.Text
			style.Label, style.Language, style.FontSize, style.Bold = s.Description, "", 16, false
			style.LabelWidth, style.LabelHeight = 0, 0
			dh := max(.25, math.Ceil(float64(len([]rune(style.Label)))*.085/w)*.25)
			b.label(style.Label, labelSurface{center: nv(bounds.X+w/2, .066, bottom-dh/2), width: w, depth: dh, align: "left"}, style, ink, 1, "board")
			bottom -= dh + .18
		}
		if root.Label != "" || root.Icon != nil {
			surface := labelSurface{center: nv(bounds.X+w/2, .066, bottom-h/2), width: w, depth: h, align: "left"}
			surface = b.shapeIcon(root, surface, 1, "board")
			b.shapeLabel(root, surface, ink, 1, "board")
		}
	}
	// The ordinary background already uses Root.Fill. A framed/patterned root
	// is drawn as one source-defined ground surface enclosing all its content.
	if root.StrokeWidth > 0 || root.FillPattern != "" || root.DoubleBorder {
		bounds := nativeMeshBounds(b.triangles)
		bounds.X -= .25
		bounds.Y -= .25
		bounds.Width += .5
		bounds.Height += .5
		d := nativeBoundsTarget(s, bounds, b.scale)
		doc, err := nativeSurfaceDocument(b.ctx, d, assets, b.fonts)
		if err != nil {
			return fmt.Errorf("isometric root frame: %w", err)
		}
		children := doc.Root.Children[:0]
		for _, n := range doc.Root.Children {
			if strings.HasPrefix(n.ID, "root:") {
				children = append(children, n)
			}
		}
		doc.Root.Children = children
		if err := b.sourcePanel(doc, documentSurface(doc, b.scale, -.045), panelCount); err != nil {
			return err
		}
	}
	return b.err
}

func finishNativeScene(b *meshBuilder, s *d2isometric.Scene, width, height int, packets []packetRoute) (*nativeScene, error) {
	if b.err != nil {
		return nil, b.err
	}
	background := color.NRGBA{245, 247, 251, 255}
	if !nativeToken(s.Root.Fill) {
		c := nativePaint(s.Background, "#f5f7fb")
		a := float64(c.A) / 255
		background = color.NRGBA{uint8(float64(c.R)*a + float64(background.R)*(1-a)), uint8(float64(c.G)*a + float64(background.G)*(1-a)), uint8(float64(c.B)*a + float64(background.B)*(1-a)), 255}
	}
	var extents projectedExtent
	extents.animatedMesh(b.triangles, b.animatedNodes, b.scale)
	extents.trafficRoutes(packets)
	camera := extents.camera(width, height, b.options.fitContent)
	if b.options.camera != nil {
		camera = *b.options.camera
	}
	width, height = camera.width, camera.height
	result := &nativeScene{packets: packets, triangles: b.triangles, panels: b.panels, width: width, height: height, background: background, animatedNodes: b.animatedNodes, pixelScale: b.scale, links: b.links}
	result.camera = camera
	if b.options.deferRaster {
		return result, nil
	}
	camera = cameraAtResolution(camera, width*rasterAA, height*rasterAA)
	raster, err := newRaster(b.ctx, width, height, b.triangles, background, &camera, nil)
	if err != nil {
		return nil, err
	}
	result.raster = raster
	return result, nil
}

func (s *nativeScene) frameRaster(ctx context.Context, seconds float64, animated bool) (*Raster, error) {
	if !animated {
		return s.raster, nil
	}
	var triangles []Triangle
	if len(s.animatedNodes) > 0 {
		triangles = append([]Triangle(nil), s.triangles...)
		phase := seconds - math.Floor(seconds)
		pulse := 1 - math.Abs(2*phase-1)
		for _, node := range s.animatedNodes {
			for i := node.first; i < node.last; i++ {
				for j := range triangles[i].V {
					triangles[i].V[j].Position.Z -= 4 * s.pixelScale * pulse
				}
				if !triangles[i].CastShadow && !triangles[i].Material.Unlit && (triangles[i].Material.Texture == nil || !triangles[i].NoDepthWrite) {
					triangles[i].CastShadow = pulse > 0
					triangles[i].ShadowOpacity = &pulse
				}
			}
		}
	}
	for _, p := range s.panels {
		if !p.animated {
			continue
		}
		if triangles == nil {
			triangles = append([]Triangle(nil), s.triangles...)
		}
		tex, err := rasterNativeSurfaceDocument(ctx, p.document, p.width, p.height, seconds)
		if err != nil {
			return nil, err
		}
		for i := p.first; i < p.last; i++ {
			m := *triangles[i].Material
			m.Texture, m.Vector = tex, nativeVectorForTexture(ctx, tex)
			triangles[i].Material = &m
		}
	}
	if triangles == nil {
		return s.raster, nil
	}
	// Keep the first-frame camera fixed; an animated boundary must not make
	// every other component shift or change scale from frame to frame.
	shadowCamera := s.raster.shadow.camera
	if shadowCamera.width == 0 {
		shadowCamera = rasterFit(s.triangles, rasterShadowDirection(), 2048, 2048, 1.08)
	}
	groundCamera := s.raster.groundShadow.camera
	if groundCamera.width == 0 {
		ground := rasterShadowGround(s.triangles)
		groundCamera = rasterFit(rasterGroundTriangles(s.triangles, ground, nativeViewDirection()), rasterShadowDirection(), 2048, 2048, 1.08)
	}
	return newRaster(ctx, s.width, s.height, triangles, s.background, &s.raster.camera, &shadowCamera, &groundCamera)
}
