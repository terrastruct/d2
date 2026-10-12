package d2isometricimg

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

// Build the intended world-space barrel directly, independently of the
// inverse transform used by contact routing. Its cap retains the previous
// height and its bottom follows the supporting plate.
func contactSupportBarrel(n d2isometric.Node, drop float64) []Triangle {
	floor := n.Position.Y - n.Size.Y/2
	height := nativeSolidHeight(n) * hierarchyNodeRelief(n)
	material := nativeMaterial(n.Fill, .68, 0, n.Opacity)
	paint := nativeSolidPaint{cap: material, ink: material, wall: material}
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.solidBarrel(nv(n.Position.X, floor+drop, n.Position.Z), n.Size.X, n.Size.Z, height-drop, paint, nativeQueueCrown(n))
	return b.triangles
}

func contactSupportTouches(p Vec, triangles []Triangle) bool {
	for _, triangle := range triangles {
		if contactTestOnTriangle(p, triangle) {
			return true
		}
	}
	return false
}

func TestSolidContactsFollowLowerSupportsAtOriginalRouteHeight(t *testing.T) {
	for _, drop := range []float64{-.2, -.4, -.6} {
		for _, printed := range []bool{false, true} {
			t.Run(fmt.Sprintf("drop=%g/printed=%v", drop, printed), func(t *testing.T) {
				source := contactTestQueue()
				source.BoardID = "outer-board"
				if printed {
					source.Label, source.Metadata.Original.Label = "Work queue", "Work queue"
					source.Metadata.Original.FontSize, source.Metadata.Original.LabelWidth, source.Metadata.Original.LabelHeight = 16, 100, 28
					source.Metadata.Original.LabelPosition = "INSIDE_TOP_CENTER"
					source.Metadata.Original.ThreeDee, source.Metadata.Original.Multiple = true, true
					source.Opacity = .6
					if nativeQueueCrown(source) >= 1 {
						t.Fatal("fixture does not exercise the flattened print crown")
					}
				}
				target := source
				target.ID, target.BoardID, target.Position.Z = "target", "inner-board", 3
				points := []Vec{nv(0, .08, .6), nv(0, .08, 1.4), nv(.2, .08, 1.4), nv(.2, .08, 2.4)}
				edges := []d2isometric.Edge{{ID: "connection", Source: source.ID, Target: target.ID, Opacity: 1, Points: points}}
				nodes := []d2isometric.Node{source, target}
				support := map[string]float64{source.BoardID: drop, target.BoardID: drop / 2}
				before, _ := json.Marshal([]any{edges, nodes, support})
				paths, err := nativeSolidContactRoutes(context.Background(), edges, nodes, [][]Vec{points}, support)
				if err != nil {
					t.Fatal(err)
				}
				path := paths[0]
				if len(path) != len(points)+2 || !reflect.DeepEqual(path[1:len(path)-1], points) {
					t.Fatal("support contact changed a source endpoint or exterior bend")
				}
				for _, p := range path {
					if p.Y != .08 {
						t.Fatal("support contact left the original flat route plane")
					}
				}
				for i, n := range nodes {
					contact := path[0]
					if i == 1 {
						contact = path[len(path)-1]
					}
					if contact.X < n.Position.X-n.Size.X/2 || contact.X > n.Position.X+n.Size.X/2 || contact.Z < n.Position.Z-n.Size.Z/2 || contact.Z > n.Position.Z+n.Size.Z/2 {
						t.Fatal("support contact escaped the compiled component footprint")
					}
					if !contactSupportTouches(contact, contactSupportBarrel(n, support[n.BoardID])) {
						t.Fatalf("contact misses extended barrel: node=%s point=%+v", n.ID, contact)
					}
				}
				baseline, err := nativeSolidContactRoutes(context.Background(), edges, nodes, [][]Vec{points})
				if err != nil {
					t.Fatal(err)
				}
				if contactSupportTouches(baseline[0][0], contactSupportBarrel(source, drop)) {
					t.Fatal("regression fixture does not distinguish the original contact from the extended body")
				}
				after, _ := json.Marshal([]any{edges, nodes, support})
				if string(before) != string(after) {
					t.Fatal("support contact modified source geometry, metadata, or support offsets")
				}
			})
		}
	}
}

func TestSolidContactSupportDefaultsPreserveExistingRoutes(t *testing.T) {
	n := contactTestQueue()
	n.BoardID = "board"
	points := []Vec{nv(0, .08, .6), nv(0, .08, 1.4)}
	edges := []d2isometric.Edge{{Source: n.ID, Target: "other", Opacity: 1, Points: points}}
	baseline, err := nativeSolidContactRoutes(context.Background(), edges, []d2isometric.Node{n}, [][]Vec{points})
	if err != nil {
		t.Fatal(err)
	}
	for _, support := range []map[string]float64{nil, {}, {"unrelated": -.4}, {"board": 0}, {"board": .2}} {
		got, err := nativeSolidContactRoutes(context.Background(), edges, []d2isometric.Node{n}, [][]Vec{points}, support)
		if err != nil || !reflect.DeepEqual(got, baseline) {
			t.Fatalf("inactive support changed existing contact behavior: support=%v err=%v", support, err)
		}
	}
}

func TestEdgeRoutesApplySupportOnlyToOrdinarySolidContacts(t *testing.T) {
	n := contactTestQueue()
	n.BoardID = "board"
	ordinary := sequenceTestEdge("ordinary", "", nv(0, .08, .6), nv(0, .08, 1.4))
	ordinary.Source = n.ID
	message := sequenceTestEdge("message", "message", nv(0, .08, .6), nv(0, .08, 1.4))
	message.Source = n.ID
	edges := []d2isometric.Edge{ordinary, message}
	support := map[string]float64{"board": -.4}
	lanes, paths, err := nativeEdgeRoutes(context.Background(), edges, []d2isometric.Node{n}, nil, support)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths[0]) != len(ordinary.Points)+1 || !contactSupportTouches(paths[0][0], contactSupportBarrel(n, -.4)) {
		t.Fatal("ordinary routing did not pass the supporting plate into solid contacts")
	}
	if !reflect.DeepEqual(lanes[0], ordinary.Points) {
		t.Fatal("support contact changed the ordinary lane geometry")
	}
	if len(paths[1]) != len(message.Points) || !reflect.DeepEqual(lanes[1], paths[1]) {
		t.Fatal("sequence message acquired solid contact vertices")
	}
	for i, p := range paths[1] {
		if p.X != message.Points[i].X || p.Z != message.Points[i].Z || p.Y != nativeSequenceMessageY {
			t.Fatal("support descent changed a sequence message's source path or semantic elevation")
		}
	}
}
