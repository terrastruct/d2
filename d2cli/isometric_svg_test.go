package d2cli

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2target"
)

func inspectIsometricSVG(t *testing.T, data []byte) (map[string]string, []string) {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root map[string]string
	var links []string
	geometry := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if root == nil {
			if start.Name.Local != "svg" || start.Name.Space != "http://www.w3.org/2000/svg" {
				t.Fatalf("unexpected document root: %v", start.Name)
			}
			root = make(map[string]string)
			for _, attr := range start.Attr {
				root[attr.Name.Local] = attr.Value
			}
		}
		switch start.Name.Local {
		case "path", "polygon":
			geometry++
		case "script", "foreignObject":
			t.Fatalf("native static SVG contains %s", start.Name.Local)
		case "a":
			for _, attr := range start.Attr {
				if attr.Name.Local == "href" {
					links = append(links, attr.Value)
				}
			}
		}
	}
	if root == nil || geometry == 0 {
		t.Fatal("native SVG is missing vector geometry")
	}
	return root, links
}

func TestIsometricSVGCLIStaticAndStdout(t *testing.T) {
	source := "a -> b"
	data, err := imageCLI(t, t.TempDir(), source, "--isometric", "--scale=.2", "--stdout-format=svg", "-")
	if err != nil {
		t.Fatal(err)
	}
	attrs, _ := inspectIsometricSVG(t, data)
	// SVG document options include the default 100px padding on both sides,
	// scaled together with the 1600x1000 native quality canvas.
	for attr, limit := range map[string]float64{"width": 360, "height": 240} {
		n, err := strconv.ParseFloat(strings.TrimSuffix(attrs[attr], "px"), 64)
		if err != nil || n < 64 || n > limit {
			t.Fatalf("SVG %s=%q outside scale bounds: %v", attr, attrs[attr], err)
		}
	}
	dir := t.TempDir()
	if _, err := imageCLI(t, dir, source, "--isometric", "--scale=.2", "view.svg"); err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(filepath.Join(dir, "view.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, file) {
		t.Fatal("file and stdout SVG export differ")
	}
}

func TestIsometricSVGCLIStaticBoards(t *testing.T) {
	dir := t.TempDir()
	source := "a: {link: layers.detail}\nlayers: {detail: {b: {link: root}}}\nscenarios: {alternate: {c}}\nsteps: {next: {d}}"
	if _, err := imageCLI(t, dir, source, "--isometric", "--scale=.1", "views.svg"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.svg", "layers/detail.svg", "scenarios/alternate.svg", "steps/next.svg"} {
		data, err := os.ReadFile(filepath.Join(dir, "views", name))
		if err != nil {
			t.Fatal(err)
		}
		_, links := inspectIsometricSVG(t, data)
		want := ""
		if name == "index.svg" {
			want = "layers/detail.svg"
		} else if name == "layers/detail.svg" {
			want = "../index.svg"
		}
		if want != "" && !containsIsometricString(links, want) {
			t.Fatalf("board %s: missing link %s in %v", name, want, links)
		}
	}
	folder := t.TempDir()
	if _, err := imageCLI(t, folder, "layers: {detail: {b}}", "--isometric", "--scale=.1", "views.svg"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, "views", "index.svg")); !os.IsNotExist(err) {
		t.Fatal("folder-only root produced an empty SVG")
	}
	if _, err := os.Stat(filepath.Join(folder, "views", "detail.svg")); err != nil {
		t.Fatal(err)
	}
	data, err := imageCLI(t, t.TempDir(), source, "--isometric", "--target=layers.detail", "--scale=.1", "--stdout-format=svg", "-")
	if err != nil {
		t.Fatal(err)
	}
	inspectIsometricSVG(t, data)
}

func TestIsometricSVGBoardLinksPreserveSource(t *testing.T) {
	board := &d2target.Diagram{
		Shapes:      []d2target.Shape{{Link: "root.layers.parent.layers.child"}, {Link: "https://example.com"}},
		Connections: []d2target.Connection{{Link: "root.layers.parent"}},
	}
	paths := map[string]string{"root": "/export/index.svg", "root.layers.child": "/export/child.svg"}
	rebased, err := relinkIsometricSVGBoard(board, "root", "root.layers.parent", paths)
	if err != nil {
		t.Fatal(err)
	}
	if rebased.Shapes[0].Link != "child.svg" || rebased.Shapes[1].Link != "https://example.com" || rebased.Connections[0].Link != "index.svg" {
		t.Fatalf("unexpected rebased board: %+v", rebased)
	}
	if board.Shapes[0].Link != "root.layers.parent.layers.child" || board.Connections[0].Link != "root.layers.parent" {
		t.Fatal("rebasing changed the source board")
	}
}

func TestIsometricSVGFailurePreservesOutput(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		args         []string
	}{
		{name: "invalid artwork", source: `a.icon: "data:image/png;base64,YmFk"`},
		{name: "annotation budget", source: "a.tooltip: " + strings.Repeat("x", (1<<20)+1)},
		{name: "invalid scale", source: "a", args: []string{"--scale=NaN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "previous.svg")
			previous := []byte("previous reviewed SVG")
			if err := os.WriteFile(path, previous, 0600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"--isometric", "--scale=.2"}, tc.args...)
			data, err := imageCLI(t, dir, tc.source, append(args, "previous.svg")...)
			if err == nil || len(data) != 0 {
				t.Fatalf("failed SVG file export produced output: err=%v, stdout=%d", err, len(data))
			}
			current, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(current, previous) {
				t.Fatalf("failed SVG export changed the previous file: %v", readErr)
			}
			data, err = imageCLI(t, dir, tc.source, append(args, "--stdout-format=svg", "-")...)
			if err == nil || len(data) != 0 {
				t.Fatalf("failed SVG stdout export leaked bytes: err=%v, stdout=%d", err, len(data))
			}
		})
	}
}
