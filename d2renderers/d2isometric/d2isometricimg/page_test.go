package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
)

func TestPagePreservesImageAndProjectsTypedLinks(t *testing.T) {
	d := captureDiagram()
	d.Shapes[0].Link = "https://example.test/docs"
	d.Shapes[1].Link = "root.steps.next"
	o := Options{Width: 400, Height: 250, FitContent: true, Render: d2isometric.RenderOpts{}}
	before, _ := json.Marshal(d)
	page, err := RenderPage(context.Background(), d, &o)
	if err != nil {
		t.Fatal(err)
	}
	png, err := Render(context.Background(), d, &o)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(page.PNG, png) {
		t.Fatal("page annotations changed visible PNG")
	}
	if len(page.Links) != 2 {
		t.Fatalf("links: %+v", page.Links)
	}
	urls, targets := map[string]bool{}, map[string]bool{}
	for _, l := range page.Links {
		urls[l.URL], targets[l.Target] = true, true
		if l.Box.Width <= 0 || l.Box.Height <= 0 || l.Box.X < 0 || l.Box.Y < 0 || l.Box.X+l.Box.Width > float64(page.Width) || l.Box.Y+l.Box.Height > float64(page.Height) {
			t.Fatalf("invalid projected box: %+v", l)
		}
	}
	if !urls["https://example.test/docs"] || !targets["root.steps.next"] {
		t.Fatal("destinations changed")
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("page mutated source")
	}
}

func TestPageMarkdownURLsStayDistinctFromBoardTargets(t *testing.T) {
	d := framingDiagram(0)
	s := &d.Shapes[0]
	s.Label, s.Language, s.FontSize, s.LabelWidth, s.LabelHeight = "[Docs](root.html) and [Guide](https://example.test/guide)", "markdown", 16, 300, 45
	s.Width, s.Height = 400, 150
	s.Link = "root.steps.next"
	o := Options{Width: 480, Height: 300, FitContent: true, Render: d2isometric.RenderOpts{}}
	p, err := RenderPage(context.Background(), d, &o)
	if err != nil {
		t.Fatal(err)
	}
	urls := map[string]bool{}
	for _, l := range p.Links {
		urls[l.URL] = true
		if l.URL == "root.html" && l.Target != "" {
			t.Fatal("relative Markdown URL became board target")
		}
	}
	if !urls["root.html"] || !urls["https://example.test/guide"] {
		t.Fatalf("inline links missing: %+v", p.Links)
	}
	if len(p.Links) < 3 || p.Links[0].Target != "root.steps.next" {
		t.Fatal("broad shape annotation should precede inline regions")
	}
}

func TestPageLinkBudgetsAndCancellation(t *testing.T) {
	d := captureDiagram()
	for i := range d.Shapes {
		d.Shapes[i].Link = "https://example.test/docs"
	}
	for _, budget := range []d2scenebuild.LinkBudget{{MaxRegions: 1, MaxStringBytes: 1000}, {MaxRegions: 10, MaxStringBytes: 2}, {MaxRegions: -1, MaxStringBytes: 10}} {
		p, err := RenderPage(context.Background(), d, &Options{Width: 160, Height: 100, LinkBudget: &budget})
		if err == nil || p != nil {
			t.Fatal("page exceeded metadata limit")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p, err := RenderPage(ctx, d, nil); p != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("page ignored cancellation", err)
	}
}

func TestPageRejectsMetadataThatWritersCannotPreserve(t *testing.T) {
	for _, value := range []string{"https://example.test/\x01", "https://example.test/\ufffe"} {
		d := framingDiagram(0)
		d.Shapes[0].Link = value
		if p, err := RenderPage(context.Background(), d, nil); err == nil || p != nil {
			t.Fatal("invalid XML destination accepted")
		}
	}
	d := framingDiagram(0)
	d.Shapes[0].Link = "https://example.test"
	d.Shapes[0].Tooltip = "https://different.test"
	if p, err := RenderPage(context.Background(), d, nil); err == nil || p != nil {
		t.Fatal("URL tooltip with link accepted")
	}
	d = captureDiagram()
	d.Connections[0].Link = "https://example.test"
	if p, err := RenderPage(context.Background(), d, nil); err == nil || p != nil {
		t.Fatal("connection silently dropped metadata without a hit area")
	}
}

func TestPagePreservesInlineMarkdownOnEdgeCaptions(t *testing.T) {
	d := captureDiagram()
	e := &d.Connections[0]
	e.Label, e.Language, e.LabelWidth, e.LabelHeight = "[Protocol](https://example.test/protocol)", "markdown", 110, 25
	p, err := RenderPage(context.Background(), d, &Options{Width: 320, Height: 200, Render: d2isometric.RenderOpts{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range p.Links {
		if l.URL == "https://example.test/protocol" {
			return
		}
	}
	t.Fatalf("rich edge caption link missing: %+v", p.Links)
}

func TestNativeInteractiveExportsRejectUnsafeLinks(t *testing.T) {
	for _, format := range []string{"svg", "page"} {
		for _, object := range []string{"shape", "connection"} {
			t.Run(format+"/"+object, func(t *testing.T) {
				for _, link := range []string{
					"javascript:alert(1)", "JAVA\tSCRIPT:alert(1)", "vBsCrIpT:msgbox(1)",
					"data:text/html,<script>alert(1)</script>", "data:image/svg+xml,<svg onload=alert(1)>",
					"file:///etc/passwd", "j&#x61;vascript:alert(1)", "java%0Ascript:alert(1)", "java%250Ascript:alert(1)",
				} {
					d := captureDiagram()
					if object == "shape" {
						d.Shapes[0].Link = link
					} else {
						d.Connections[0].Link, d.Connections[0].Label = link, "details"
					}
					o := &Options{Width: 160, Height: 100}
					var err error
					if format == "svg" {
						o.Format = SVG
						var data []byte
						data, err = Render(context.Background(), d, o)
						if len(data) != 0 {
							t.Fatalf("unsafe destination %q produced SVG", link)
						}
					} else {
						var page *Page
						page, err = RenderPage(context.Background(), d, o)
						if page != nil {
							t.Fatalf("unsafe destination %q produced a page", link)
						}
					}
					if err == nil || !strings.Contains(err.Error(), "unsafe URL scheme") {
						t.Fatalf("unsafe destination %q: %v", link, err)
					}
				}
			})
		}
	}
}

func TestNativeLinkDestinationDistinguishesBoardTargetsAndFiles(t *testing.T) {
	for _, tc := range []struct {
		link   string
		target bool
	}{
		{"root", true}, {"root.layers.detail", true}, {"root.scenarios.alternate", true},
		{"root.steps.next", true}, {`root.layers."root.svg".steps."next.step"`, true},
		{"root.svg", false}, {"root.html", false}, {"root.layers.svg", true},
		{"root.layers", false}, {"root.layers.detail.svg", false},
		{"../root.svg", false}, {"root.svg?view=1#detail", false},
		{"https://example.com/root.svg", false}, {"vscode://file/example.go:10:2", false},
	} {
		t.Run(tc.link, func(t *testing.T) {
			r := nativeLinkDestination(tc.link, "details")
			if r.Tooltip != "details" || tc.target && (r.Target != tc.link || r.URL != "") || !tc.target && (r.URL != tc.link || r.Target != "") {
				t.Fatalf("destination changed: %+v", r)
			}
		})
	}
}
