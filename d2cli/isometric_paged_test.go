package d2cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/internal/testutil"
	"github.com/d2lang/d2/internal/testutil/imagediff"
	"github.com/d2lang/d2/lib/pptx"
)

const isometricPagedSource = `label: Architecture
website: Website {
  link: https://example.com
  tooltip: Open website
}
next: Detail {
  link: layers.detail
  tooltip: Open detail
}
docs: |md
  [Documentation](https://example.com/docs "Read documentation")
|
website -> next: Request {
  link: https://example.com/request
}
layers: {
  detail: {
    label: Detail
    x -> y
  }
}
`

func TestIsometricPagedCLIImagesLinksNavigationAndStdout(t *testing.T) {
	dir := t.TempDir()
	common := []string{"--isometric", "--scale=.25"}
	if _, err := imageCLI(t, dir, isometricPagedSource, append(common, "pages.png")...); err != nil {
		t.Fatal(err)
	}
	var expected [][]byte
	for _, path := range []string{"index.png", "detail.png"} {
		data, err := os.ReadFile(filepath.Join(dir, "pages", path))
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, data)
	}
	for _, format := range []string{"pdf", "pptx"} {
		for _, stdout := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stdout=%v", format, stdout), func(t *testing.T) {
				args := append([]string{}, common...)
				name := "pages." + format
				if stdout {
					args = append(args, "--stdout-format="+format, "-")
				} else {
					args = append(args, name)
				}
				data, err := imageCLI(t, dir, isometricPagedSource, args...)
				if err != nil {
					t.Fatal(err)
				}
				if !stdout {
					data, err = os.ReadFile(filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
				}
				if format == "pdf" {
					if _, err := testutil.ComparePDFImages(data, expected, imagediff.Options{}); err != nil {
						t.Fatal(err)
					}
					info, err := testutil.InspectD2PDF(data)
					if err != nil {
						t.Fatal(err)
					}
					if len(info.Pages) != 2 || info.InternalLinks < 2 {
						t.Fatalf("PDF pages/navigation/body links: %+v", info)
					}
					for _, url := range []string{"https://example.com", "https://example.com/docs", "https://example.com/request"} {
						if !containsIsometricString(info.ExternalLinks, url) {
							t.Fatalf("PDF omitted %q: %v", url, info.ExternalLinks)
						}
					}
				} else {
					if err := pptx.Validate(data, 2); err != nil {
						t.Fatal(err)
					}
					for i, pixels := range expected {
						encoded := readZipMember(t, data, fmt.Sprintf("ppt/media/slide%dImage.png", i+1))
						if !bytes.Equal(encoded, pixels) {
							t.Fatalf("PPTX page %d did not embed the exact standalone native PNG", i)
						}
					}
					rels := string(readZipMember(t, data, "ppt/slides/_rels/slide1.xml.rels"))
					for _, destination := range []string{"https://example.com", "https://example.com/docs", "https://example.com/request", "slide2.xml"} {
						if !strings.Contains(rels, `Target="`+destination+`"`) {
							t.Fatalf("PPTX omitted %q: %s", destination, rels)
						}
					}
					childRels := string(readZipMember(t, data, "ppt/slides/_rels/slide2.xml.rels"))
					if !strings.Contains(childRels, `Target="slide1.xml"`) {
						t.Fatalf("PPTX omitted parent navigation: %s", childRels)
					}
					xml := string(readZipMember(t, data, "ppt/slides/slide1.xml"))
					if !strings.Contains(xml, `tooltip="Open website"`) {
						t.Fatalf("PPTX omitted link tooltip: %s", xml)
					}
				}
			})
		}
	}
}

func containsIsometricString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestIsometricPagedBoardKindsFoldersAndSelection(t *testing.T) {
	for _, format := range []string{"pdf", "pptx"} {
		for _, tc := range []struct {
			name, source, target string
			pages                int
		}{
			{"all board kinds", "a\nlayers: {detail: {b}}\nscenarios: {alternate: {c}}\nsteps: {next: {d}}", "*", 4},
			{"selected layer", "a\nlayers: {detail: {b}}\nscenarios: {alternate: {c}}", "layers.detail", 1},
			{"folder-only ancestors", "layers: {folder: {layers: {child: {x}}}}", "*", 1},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				data, err := imageCLI(t, t.TempDir(), tc.source, "--isometric", "--scale=.2", "--target="+tc.target, "--stdout-format="+format, "-")
				if err != nil {
					t.Fatal(err)
				}
				if format == "pdf" {
					info, err := testutil.InspectD2PDF(data)
					if err != nil || len(info.Pages) != tc.pages {
						t.Fatalf("PDF page selection: %+v %v", info, err)
					}
				} else if err := pptx.Validate(data, tc.pages); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	plain, err := imageCLI(t, t.TempDir(), "a -> b", "--stdout-format=svg", "-")
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := imageCLI(t, t.TempDir(), "a -> b", "--isometric=false", "--stdout-format=svg", "-")
	if err != nil || !bytes.Equal(plain, disabled) {
		t.Fatalf("disabled native mode changed SVG output: %v", err)
	}
}

func TestIsometricPagedFailurePreservesOutput(t *testing.T) {
	for _, format := range []string{"pdf", "pptx"} {
		for _, tc := range []struct {
			source string
			args   []string
		}{
			{source: "a.link: https://example.com\nlayers: {detail: {b.icon: \"data:image/png;base64,YmFk\"}}"},
			{source: "a.link: layers.detail\nlayers: {detail: {b}}", args: []string{"--target="}},
		} {
			source := tc.source
			dir := t.TempDir()
			name := "existing." + format
			path := filepath.Join(dir, name)
			old := []byte("previous reviewed document")
			if err := os.WriteFile(path, old, 0600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"--isometric", "--scale=.2"}, tc.args...)
			data, err := imageCLI(t, dir, source, append(args, name)...)
			if err == nil || len(data) != 0 {
				t.Fatalf("failed %s export for %q: %v, stdout=%d", format, source, err, len(data))
			}
			current, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(current, old) {
				t.Fatalf("failed %s export changed destination: %v", format, readErr)
			}
			data, err = imageCLI(t, dir, source, append(args, "--stdout-format="+format, "-")...)
			if err == nil || len(data) != 0 {
				t.Fatalf("failed %s stdout export: %v, bytes=%d", format, err, len(data))
			}
		}
	}
}

func TestIsometricPagedSelectedSubtreeKeepsInternalLinks(t *testing.T) {
	source := `layers: {
  parent: {
    label: Parent
    a.link: layers.child
    docs: |md
      [Child](root.layers.parent.layers.child)
    |
    layers: {child: {label: Child; b}}
  }
}`
	for _, format := range []string{"pdf", "pptx"} {
		data, err := imageCLI(t, t.TempDir(), source, "--isometric", "--scale=.2", "--target=layers.parent.*", "--stdout-format="+format, "-")
		if err != nil {
			t.Fatal(err)
		}
		if format == "pdf" {
			info, err := testutil.InspectD2PDF(data)
			if err != nil || len(info.Pages) != 2 || info.InternalLinks < 3 {
				t.Fatalf("selected PDF links: %+v %v", info, err)
			}
		} else {
			if err := pptx.Validate(data, 2); err != nil {
				t.Fatal(err)
			}
			rels := string(readZipMember(t, data, "ppt/slides/_rels/slide1.xml.rels"))
			if strings.Count(rels, `Target="slide2.xml"`) < 2 {
				t.Fatalf("selected PPTX body and Markdown links: %s", rels)
			}
		}
	}
	links := []d2scene.LinkRegion{{Target: "root.layers.parent.layers.child"}, {URL: "root.html"}}
	rebased, err := rebaseIsometricPageLinks(links, "root.layers.parent", map[string]int{"root": 0, "root.layers.child": 1})
	if err != nil || rebased[0].Target != "root.layers.child" || rebased[1].URL != "root.html" || links[0].Target != "root.layers.parent.layers.child" {
		t.Fatalf("link rebasing changed source or relative URL: %+v %+v %v", links, rebased, err)
	}
	if _, err := rebaseIsometricPageLinks([]d2scene.LinkRegion{{Target: "root"}}, "root.layers.parent", map[string]int{"root": 0}); err == nil {
		t.Fatal("outside source root became a self-link")
	}
}

func TestIsometricPagedAdmissionAndBoundedWriter(t *testing.T) {
	page := &d2isometricimg.Page{Width: 100, Height: 80, PNG: []byte("encoded"), Links: []d2scene.LinkRegion{{Box: d2scene.Box{X: 1, Y: 1, Width: 10, Height: 8}, URL: "https://example.com"}}}
	makeRenderer := func() *isometricPagedRenderer {
		return &isometricPagedRenderer{ctx: context.Background(), opts: d2isometricimg.Options{Width: 100, Height: 100},
			remainingPixels: 16000, remainingEncoded: 14, remainingLinks: 2, remainingLinkBytes: 100}
	}
	r := makeRenderer()
	if err := r.admitPage(page); err != nil {
		t.Fatal(err)
	}
	if err := r.admitPage(page); err != nil {
		t.Fatal(err)
	}
	if err := r.admitPage(page); err == nil {
		t.Fatal("page operation budgets reset between pages")
	}
	for _, tc := range []struct {
		name   string
		change func(*isometricPagedRenderer)
	}{
		{"pixels", func(r *isometricPagedRenderer) { r.remainingPixels = 1 }},
		{"encoded", func(r *isometricPagedRenderer) { r.remainingEncoded = 1 }},
		{"regions", func(r *isometricPagedRenderer) { r.remainingLinks = 0 }},
		{"strings", func(r *isometricPagedRenderer) { r.remainingLinkBytes = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := makeRenderer()
			tc.change(r)
			if err := r.admitPage(page); err == nil {
				t.Fatal("budget accepted")
			}
		})
	}
	var output bytes.Buffer
	w := &isometricDocumentWriter{ctx: context.Background(), output: &output, remaining: 2}
	if n, err := w.Write([]byte("too long")); err == nil || n != 0 || output.Len() != 0 {
		t.Fatal("output cap wrote bytes")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.ctx = ctx
	if n, err := w.Write([]byte("x")); !errors.Is(err, context.Canceled) || n != 0 {
		t.Fatalf("cancelled write: %d %v", n, err)
	}
}
