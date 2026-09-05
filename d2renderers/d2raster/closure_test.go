package d2raster_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestRasterPackageHasNoIOCapabilities(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve raster test location")
	}
	rasterDirectory := filepath.Dir(filename)
	repositoryRoot := filepath.Clean(filepath.Join(rasterDirectory, "..", ".."))

	entries, err := os.ReadDir(rasterDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(rasterDirectory, entry.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("decode raster import %q: %v", spec.Path.Value, err)
			}
			if forbiddenDirectRasterImport(path) {
				t.Errorf("production raster imports forbidden I/O capability %q", path)
			}
		}
	}

	for _, target := range []struct {
		goos   string
		goarch string
	}{
		{goos: "darwin", goarch: "amd64"},
		{goos: "darwin", goarch: "arm64"},
		{goos: "linux", goarch: "amd64"},
		{goos: "linux", goarch: "arm64"},
		{goos: "windows", goarch: "amd64"},
		{goos: "windows", goarch: "arm64"},
	} {
		t.Run(target.goos+"_"+target.goarch, func(t *testing.T) {
			command := exec.Command("go", "list", "-mod=readonly", "-deps", "-f", "{{.ImportPath}}", "./d2renderers/d2raster")
			command.Dir = repositoryRoot
			command.Env = append(os.Environ(),
				"CGO_ENABLED=0",
				"GOOS="+target.goos,
				"GOARCH="+target.goarch,
				"GOWORK=off",
			)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("list production raster dependencies: %v\n%s", err, output)
			}
			for _, path := range strings.Fields(string(output)) {
				if path == "os/exec" || path == "plugin" || path == "net" {
					t.Errorf("production raster dependency closure contains forbidden I/O capability %q", path)
				}
			}
		})
	}
}

func forbiddenDirectRasterImport(path string) bool {
	return path == "C" || path == "os" || path == "os/exec" || path == "plugin" ||
		path == "syscall" || path == "net" || strings.HasPrefix(path, "net/") ||
		path == "golang.org/x/sys" || strings.HasPrefix(path, "golang.org/x/sys/")
}
