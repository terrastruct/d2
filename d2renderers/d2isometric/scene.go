// Package d2isometric builds 3D scenes from compiled D2 diagrams for native image
// rendering. It composes new geometry without modifying the supplied diagram.
package d2isometric

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/color"
)

const (
	MaxNodes        = 2000
	MaxEdges        = 5000
	MaxIDBytes      = 8192
	MaxIDDepth      = 128
	maxEntries      = 100000
	maxBytes        = 8 << 20
	coordinateLimit = 1e9
)

type RenderOpts struct {
	ThemeID        *int64
	ThemeOverrides *d2target.ThemeOverrides
}

type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
type Board struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId,omitempty"`
	Label    string `json:"label"`
	ParentID string `json:"parentId,omitempty"`
	Kind     string `json:"kind"`
	// Level records authored container depth; presentation lowers supporting plates.
	Level       int      `json:"level"`
	Position    Vec3     `json:"position"`
	Size        Vec3     `json:"size"`
	NodeIDs     []string `json:"nodeIds"`
	HeaderDepth float64  `json:"headerDepth"`
}
type NodeMetadata struct {
	Original d2target.Shape `json:"original"`
}
type Node struct {
	ID             string       `json:"id"`
	Label          string       `json:"label"`
	Type           string       `json:"type"`
	BoardID        string       `json:"boardId"`
	ParentID       string       `json:"parentId,omitempty"`
	Container      bool         `json:"container"`
	SequenceRole   string       `json:"sequenceRole,omitempty"`
	Position       Vec3         `json:"position"`
	Size           Vec3         `json:"size"`
	Fill           string       `json:"fill"`
	FillExplicit   bool         `json:"fillExplicit"`
	Stroke         string       `json:"stroke"`
	StrokeExplicit bool         `json:"strokeExplicit"`
	FontColor      string       `json:"fontColor"`
	FontExplicit   bool         `json:"fontExplicit"`
	Opacity        float64      `json:"opacity"`
	StrokeWidth    int          `json:"strokeWidth"`
	StrokeDash     float64      `json:"strokeDash"`
	Tooltip        string       `json:"tooltip,omitempty"`
	Link           string       `json:"link,omitempty"`
	Icon           string       `json:"icon,omitempty"`
	Metadata       NodeMetadata `json:"metadata"`
}
type EdgeMetadata struct {
	Original d2target.Connection `json:"original"`
}
type Edge struct {
	ID             string             `json:"id"`
	Source         string             `json:"source"`
	Target         string             `json:"target"`
	Label          string             `json:"label"`
	SourceLabel    *d2target.Text     `json:"sourceLabel,omitempty"`
	TargetLabel    *d2target.Text     `json:"targetLabel,omitempty"`
	SourceArrow    d2target.Arrowhead `json:"sourceArrow"`
	TargetArrow    d2target.Arrowhead `json:"targetArrow"`
	Stroke         string             `json:"stroke"`
	StrokeExplicit bool               `json:"strokeExplicit"`
	FontColor      string             `json:"fontColor"`
	FontExplicit   bool               `json:"fontExplicit"`
	StrokeWidth    int                `json:"strokeWidth"`
	StrokeDash     float64            `json:"strokeDash"`
	Opacity        float64            `json:"opacity"`
	Link           string             `json:"link,omitempty"`
	Tooltip        string             `json:"tooltip,omitempty"`
	Icon           string             `json:"icon,omitempty"`
	Points         []Vec3             `json:"points"`
	SequenceRole   string             `json:"sequenceRole,omitempty"`
	Metadata       EdgeMetadata       `json:"metadata"`
}
type Scene struct {
	FontFamily     *d2fonts.FontFamily      `json:"fontFamily,omitempty"`
	MonoFontFamily *d2fonts.FontFamily      `json:"monoFontFamily,omitempty"`
	ThemeID        int64                    `json:"themeId"`
	ThemeOverrides *d2target.ThemeOverrides `json:"themeOverrides,omitempty"`
	HasSequence    bool                     `json:"hasSequence,omitempty"`
	PixelScale     float64                  `json:"pixelScale"`
	Description    string                   `json:"description,omitempty"`
	Background     string                   `json:"background"`
	Boards         []Board                  `json:"boards"`
	Nodes          []Node                   `json:"nodes"`
	Edges          []Edge                   `json:"edges"`
	Warnings       []string                 `json:"warnings,omitempty"`
	Root           d2target.Shape           `json:"root"`
	Legend         *d2target.Legend         `json:"legend,omitempty"`
}

// BuildScene maps a single D2 board. IDs, text, endpoint direction, and original
// shape/edge metadata are retained, including connections to container boards.
// The returned scene owns its data and is independent of the input diagram.
func BuildScene(diagram *d2target.Diagram, opts *RenderOpts) (*Scene, error) {
	if diagram == nil {
		return nil, fmt.Errorf("isometric: nil diagram")
	}
	if len(diagram.Shapes) > MaxNodes || len(diagram.Connections) > MaxEdges {
		return nil, fmt.Errorf("isometric: scene exceeds %d nodes or %d edges", MaxNodes, MaxEdges)
	}
	if opts == nil {
		opts = &RenderOpts{}
	}
	// Admission precedes encoding, key parsing, graph traversal and allocations.
	admission := budget{}
	for _, v := range []any{diagram.Name, diagram.Description, diagram.Root, diagram.Legend, diagram.Shapes, diagram.Connections, opts} {
		if err := admission.check(reflect.ValueOf(v)); err != nil {
			return nil, err
		}
	}
	for _, s := range diagram.Shapes {
		if s.Width < 0 || s.Height < 0 || s.LabelWidth < 0 || s.LabelHeight < 0 || s.StrokeWidth < 0 || s.Opacity < 0 || s.Opacity > 1 {
			return nil, fmt.Errorf("isometric: node %q has invalid dimensions or opacity", s.ID)
		}
	}
	for _, c := range diagram.Connections {
		if c.StrokeWidth < 0 || c.Opacity < 0 || c.Opacity > 1 {
			return nil, fmt.Errorf("isometric: edge %q has invalid stroke or opacity", c.ID)
		}
		for _, p := range c.Route {
			if p == nil {
				return nil, fmt.Errorf("isometric: edge %q has nil route point", c.ID)
			}
		}
	}
	copyData, err := json.Marshal(struct {
		Shapes      []d2target.Shape
		Connections []d2target.Connection
		Root        d2target.Shape
		Legend      *d2target.Legend
	}{diagram.Shapes, diagram.Connections, diagram.Root, diagram.Legend})
	if err != nil {
		return nil, fmt.Errorf("isometric: invalid diagram: %w", err)
	}
	if len(copyData) > maxBytes*2 {
		return nil, fmt.Errorf("isometric: encoded scene metadata exceeds budget")
	}
	var owned struct {
		Shapes      []d2target.Shape
		Connections []d2target.Connection
		Root        d2target.Shape
		Legend      *d2target.Legend
	}
	if err = json.Unmarshal(copyData, &owned); err != nil {
		return nil, err
	}
	// url.Userinfo has private fields that JSON cannot round-trip. Copy URL
	// values explicitly so icon identity and ownership also survive userinfo.
	for i := range owned.Shapes {
		owned.Shapes[i].Icon = cloneURL(diagram.Shapes[i].Icon)
	}
	for i := range owned.Connections {
		owned.Connections[i].Icon = cloneURL(diagram.Connections[i].Icon)
	}
	owned.Root.Icon = cloneURL(diagram.Root.Icon)
	if owned.Legend != nil {
		for i := range owned.Legend.Shapes {
			owned.Legend.Shapes[i].Icon = cloneURL(diagram.Legend.Shapes[i].Icon)
		}
		for i := range owned.Legend.Connections {
			owned.Legend.Connections[i].Icon = cloneURL(diagram.Legend.Connections[i].Icon)
		}
	}
	themeID := int64(0)
	var overrides *d2target.ThemeOverrides
	if diagram.Config != nil {
		if diagram.Config.ThemeID != nil {
			themeID = *diagram.Config.ThemeID
		}
		overrides = diagram.Config.ThemeOverrides
	}
	if opts.ThemeID != nil {
		themeID = *opts.ThemeID
	}
	if opts.ThemeOverrides != nil {
		overrides = opts.ThemeOverrides
	}
	if err := admission.check(reflect.ValueOf(overrides)); err != nil {
		return nil, err
	}
	theme := d2themescatalog.Find(themeID)
	if theme.Name == "" {
		return nil, fmt.Errorf("isometric: unknown theme %d", themeID)
	}
	theme.ApplyOverrides(overrides)
	paint := func(value string) string { return d2themes.ResolveThemeColor(theme, value) }
	scene := &Scene{PixelScale: SceneScale, Description: diagram.Description, Background: paint(diagram.Root.Fill), Boards: []Board{}, Nodes: []Node{}, Edges: []Edge{}}
	scene.Root = owned.Root
	scene.Legend = owned.Legend
	scene.ThemeID = themeID
	if overrides != nil {
		data, err := json.Marshal(overrides)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &scene.ThemeOverrides); err != nil {
			return nil, err
		}
	}
	if diagram.FontFamily != nil {
		family := *diagram.FontFamily
		scene.FontFamily = &family
	}
	if diagram.MonoFontFamily != nil {
		family := *diagram.MonoFontFamily
		scene.MonoFontFamily = &family
	}
	if scene.Background == "" {
		scene.Background = "#f5f7fa"
	}
	indices := map[string]int{}
	keys := map[string]int{}
	paths := make([][]string, len(owned.Shapes))
	for i, s := range owned.Shapes {
		if len(s.ID) > MaxIDBytes {
			return nil, fmt.Errorf("isometric: node ID exceeds %d bytes", MaxIDBytes)
		}
		if s.ID == "" {
			return nil, fmt.Errorf("isometric: node %d has empty ID", i)
		}
		if _, ok := indices[s.ID]; ok {
			return nil, fmt.Errorf("isometric: duplicate node ID %q", s.ID)
		}
		path, e := d2parser.ParseKey(s.ID)
		if e != nil {
			return nil, fmt.Errorf("isometric: node ID: %w", e)
		}
		if len(path.Path) > MaxIDDepth {
			return nil, fmt.Errorf("isometric: node ID nesting exceeds %d components", MaxIDDepth)
		}
		for _, part := range path.Path {
			paths[i] = append(paths[i], part.Unbox().ScalarString())
		}
		key := pathKey(paths[i])
		if _, ok := keys[key]; ok {
			return nil, fmt.Errorf("isometric: equivalent node ID %q", s.ID)
		}
		indices[s.ID] = i
		keys[key] = i
		n := Node{ID: s.ID, Label: s.Label, Type: s.Type, Fill: paint(s.Fill), Stroke: paint(s.Stroke), FontColor: paint(s.GetFontColor()), Opacity: s.Opacity, StrokeWidth: s.StrokeWidth, StrokeDash: s.StrokeDash, Tooltip: s.Tooltip, Link: s.Link, Metadata: NodeMetadata{Original: s}}
		n.FillExplicit = overrides != nil || (s.Fill != "" && !color.IsThemeColor(s.Fill))
		n.StrokeExplicit = overrides != nil || (s.Stroke != "" && !color.IsThemeColor(s.Stroke))
		n.FontExplicit = overrides != nil || (s.Color != "" && !color.IsThemeColor(s.Color))
		if s.Icon != nil {
			n.Icon = s.Icon.String()
		}
		scene.Nodes = append(scene.Nodes, n)
	}
	parents := make([]int, len(scene.Nodes))
	for i := range parents {
		parents[i] = -1
	}
	for i, path := range paths {
		for depth := len(path) - 1; depth > 0; depth-- {
			if parent, ok := keys[pathKey(path[:depth])]; ok {
				parents[i] = parent
				scene.Nodes[i].ParentID = scene.Nodes[parent].ID
				scene.Nodes[parent].Container = true
				break
			}
		}
	}
	edgeIDs := map[string]bool{}
	for _, c := range owned.Connections {
		if c.ID == "" || edgeIDs[c.ID] {
			return nil, fmt.Errorf("isometric: empty or duplicate edge ID %q", c.ID)
		}
		edgeIDs[c.ID] = true
		e := Edge{ID: c.ID, Source: c.Src, Target: c.Dst, Label: c.Label, SourceLabel: c.SrcLabel, TargetLabel: c.DstLabel, SourceArrow: c.SrcArrow, TargetArrow: c.DstArrow, Stroke: paint(c.Stroke), FontColor: paint(c.Color), StrokeWidth: c.StrokeWidth, StrokeDash: c.StrokeDash, Opacity: c.Opacity, Link: c.Link, Tooltip: c.Tooltip, Metadata: EdgeMetadata{Original: c}}
		e.StrokeExplicit = overrides != nil || (c.Stroke != "" && !color.IsThemeColor(c.Stroke))
		e.FontExplicit = overrides != nil || (c.Color != "" && !color.IsThemeColor(c.Color))
		if c.Icon != nil {
			e.Icon = c.Icon.String()
		}
		scene.Edges = append(scene.Edges, e)
		_, src := indices[c.Src]
		_, dst := indices[c.Dst]
		if (!src || !dst) && len(c.Route) < 2 {
			return nil, fmt.Errorf("isometric: edge %q has an unknown endpoint and no usable original route", c.ID)
		}
	}
	classifySequence(scene, indices, parents)
	if err := composeHierarchy(scene, indices, parents); err != nil {
		return nil, err
	}
	if len(diagram.Layers)+len(diagram.Scenarios)+len(diagram.Steps) > 0 {
		scene.Warnings = append(scene.Warnings, "This scene contains the selected D2 board. The export pipeline renders other layers, scenarios and steps separately.")
	}
	return scene, nil
}

func pathKey(path []string) string { data, _ := json.Marshal(path); return string(data) }

func cloneURL(original *url.URL) *url.URL {
	if original == nil {
		return nil
	}
	copy := *original
	if original.User != nil {
		user := *original.User
		copy.User = &user
	}
	return &copy
}

type budget struct{ bytes, entries int }

func (b *budget) check(v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return b.check(v.Elem())
		}
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return fmt.Errorf("isometric: invalid UTF-8 text")
		}
		b.bytes += len(v.String())
		if b.bytes > maxBytes {
			return fmt.Errorf("isometric: text exceeds %d bytes", maxBytes)
		}
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) || math.Abs(v.Float()) > coordinateLimit {
			return fmt.Errorf("isometric: nonfinite or excessive numeric value")
		}
	case reflect.Int, reflect.Int64, reflect.Int32:
		if v.Int() < -int64(coordinateLimit) || v.Int() > int64(coordinateLimit) {
			return fmt.Errorf("isometric: excessive numeric value")
		}
	case reflect.Slice, reflect.Array:
		b.entries += v.Len()
		if b.entries > maxEntries {
			return fmt.Errorf("isometric: collection entries exceed %d", maxEntries)
		}
		for i := 0; i < v.Len(); i++ {
			if err := b.check(v.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := b.check(v.Field(i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func sourceOrder(nodes []Node, ids []int) {
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := nodes[ids[i]].Metadata.Original, nodes[ids[j]].Metadata.Original
		if a.Pos.Y != b.Pos.Y {
			return a.Pos.Y < b.Pos.Y
		}
		if a.Pos.X != b.Pos.X {
			return a.Pos.X < b.Pos.X
		}
		return strings.Compare(a.ID, b.ID) < 0
	})
}
