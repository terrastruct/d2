package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// D2's repeating dash periods may be non-integral seconds. A bounded GIF cannot
// use their least common multiple, so its private documents fit each period to
// the nearest whole number of cycles in the chosen loop. Exact-time captures
// and all source documents keep their authored durations.
func nativeGIFLoopScene(ctx context.Context, original *nativeScene, loop time.Duration) (*nativeScene, error) {
	if ctx == nil || original == nil || loop <= 0 || loop > 10*time.Minute {
		return nil, fmt.Errorf("invalid native GIF loop")
	}
	out := *original
	out.panels = append([]nativePanel(nil), original.panels...)
	copied := make(map[*d2scene.Node]*d2scene.Node)
	active := make(map[*d2scene.Node]bool)
	tracks := 0
	var clone func(*d2scene.Node, int) (*d2scene.Node, error)
	clone = func(node *d2scene.Node, depth int) (*d2scene.Node, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if node == nil || depth > 256 || active[node] {
			return nil, fmt.Errorf("invalid native GIF animation tree")
		}
		if existing := copied[node]; existing != nil {
			return existing, nil
		}
		if len(copied) >= 200000 || len(node.Children) > 200000 || len(node.Animations) > 200000-tracks {
			return nil, fmt.Errorf("native GIF animation tree exceeds budget")
		}
		tracks += len(node.Animations)
		n := *node
		n.Animations = append([]d2scene.Track(nil), node.Animations...)
		for i := range n.Animations {
			t := &n.Animations[i]
			if t.Repeat && t.Duration > 0 {
				cycles := max(1, int64(math.Round(float64(loop)/float64(t.Duration))))
				t.Duration = max(time.Nanosecond, loop/time.Duration(cycles))
			}
		}
		n.Children = make([]*d2scene.Node, len(node.Children))
		copied[node], active[node] = &n, true
		if node.Mask != nil {
			mask := *node.Mask
			var err error
			mask.Root, err = clone(mask.Root, depth+1)
			if err != nil {
				return nil, err
			}
			n.Mask = &mask
		}
		for i, child := range node.Children {
			var err error
			n.Children[i], err = clone(child, depth+1)
			if err != nil {
				return nil, err
			}
		}
		delete(active, node)
		return &n, nil
	}
	for i := range out.panels {
		panel := &out.panels[i]
		if !panel.animated {
			continue
		}
		if panel.document == nil {
			return nil, fmt.Errorf("native GIF animated panel is missing its document")
		}
		doc := *panel.document
		root, err := clone(doc.Root, 0)
		if err != nil {
			return nil, err
		}
		doc.Root = root
		panel.document = &doc
	}
	return &out, nil
}
