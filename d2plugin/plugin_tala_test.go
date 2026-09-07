package d2plugin

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout"
)

func TestTALAPluginInfo(t *testing.T) {
	plugin := &talaPlugin{opts: d2talalayout.DefaultOptions()}
	info, err := plugin.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "tala" || info.Type != "bundled" {
		t.Fatalf("TALA info = %#v", info)
	}
	want := []PluginFeature{
		DESCENDANT_EDGES,
		CONTAINER_DIMENSIONS,
		NEAR_OBJECT,
		TOP_LEFT,
		ROUTES_EDGES,
	}
	for _, feature := range want {
		if !slices.Contains(info.Features, feature) {
			t.Fatalf("TALA features = %v, missing %q", info.Features, feature)
		}
	}
}

func TestTALAPluginHydrateOpts(t *testing.T) {
	plugin := &talaPlugin{opts: d2talalayout.DefaultOptions()}
	if err := plugin.HydrateOpts([]byte(`{"tala-seeds":[7,11]}`)); err != nil {
		t.Fatal(err)
	}
	snapshot := plugin.optionsSnapshot()
	if !slices.Equal(snapshot.Seeds, []int64{7, 11}) {
		t.Fatalf("TALA seeds = %v, want [7 11]", snapshot.Seeds)
	}
	snapshot.Seeds[0] = 99
	if got := plugin.optionsSnapshot().Seeds; !slices.Equal(got, []int64{7, 11}) {
		t.Fatalf("mutating an options snapshot changed plugin state: %v", got)
	}
	if err := plugin.HydrateOpts([]byte(`{"tala-seeds":"invalid"}`)); err == nil {
		t.Fatal("expected invalid TALA options to fail")
	}
	if err := plugin.HydrateOpts([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if got, want := plugin.optionsSnapshot().Seeds, d2talalayout.DefaultOptions().Seeds; !slices.Equal(got, want) {
		t.Fatalf("empty hydration seeds = %v, want defaults %v", got, want)
	}
}

func TestTALAPluginConcurrentHydrationAndSnapshots(t *testing.T) {
	plugin := &talaPlugin{opts: d2talalayout.DefaultOptions()}
	var wait sync.WaitGroup
	for i := range 32 {
		wait.Add(2)
		go func() {
			defer wait.Done()
			raw := []byte(`{"tala-seeds":[1]}`)
			if i%2 == 1 {
				raw = []byte(`{"tala-seeds":[2]}`)
			}
			if err := plugin.HydrateOpts(raw); err != nil {
				t.Errorf("hydrate TALA options: %v", err)
			}
		}()
		go func() {
			defer wait.Done()
			seeds := plugin.optionsSnapshot().Seeds
			if len(seeds) == 0 {
				t.Error("options snapshot has no seeds")
			}
		}()
	}
	wait.Wait()
}

func TestTALAPluginInstancesIsolateOptions(t *testing.T) {
	plugins := []Plugin{&TALAPlugin}
	first, err := FindPlugin(context.Background(), plugins, "tala")
	if err != nil {
		t.Fatal(err)
	}
	second, err := FindPlugin(context.Background(), plugins, "tala")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == &TALAPlugin || second == &TALAPlugin {
		t.Fatal("FindPlugin returned shared TALA configuration")
	}
	if err := first.HydrateOpts([]byte(`{"tala-seeds":[7]}`)); err != nil {
		t.Fatal(err)
	}
	if err := second.HydrateOpts([]byte(`{"tala-seeds":[11]}`)); err != nil {
		t.Fatal(err)
	}
	if got := first.(*talaPlugin).optionsSnapshot().Seeds; !slices.Equal(got, []int64{7}) {
		t.Fatalf("first TALA instance seeds = %v, want [7]", got)
	}
	if got := second.(*talaPlugin).optionsSnapshot().Seeds; !slices.Equal(got, []int64{11}) {
		t.Fatalf("second TALA instance seeds = %v, want [11]", got)
	}
	if got, want := TALAPlugin.optionsSnapshot().Seeds, d2talalayout.DefaultOptions().Seeds; !slices.Equal(got, want) {
		t.Fatalf("registered TALA template seeds = %v, want defaults %v", got, want)
	}
}

func TestBundledTALAWinsBeforeExecutingExternalDuplicate(t *testing.T) {
	temp := t.TempDir()
	marker := filepath.Join(temp, "executed")
	name := binaryPrefix + "tala"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(temp, name)
	copyExecutable(t, binary)

	t.Setenv(pluginInfoHelperEnv, "1")
	t.Setenv(pluginInfoMarkerEnv, marker)
	t.Setenv("PATH", temp)
	listed, err := ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("shadowed external TALA affected plugin listing: %v", err)
	}
	selected, err := FindPlugin(context.Background(), listed, "tala")
	if err != nil {
		t.Fatal(err)
	}
	info, err := selected.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := selected.(*talaPlugin); !ok || info.Type != "bundled" {
		t.Fatalf("selected TALA plugin = %T with info %#v, want an isolated bundled instance", selected, info)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shadowed external TALA was executed before duplicate suppression: %v", err)
	}
}

func TestExternalPluginName(t *testing.T) {
	tests := map[string]string{
		"/plugins/d2plugin-tala":              "tala",
		"/plugins/d2plugin-tala.exe":          "tala",
		"/plugins/d2plugin-TALA.EXE":          "TALA",
		"/plugins/D2PLUGIN-TALA.EXE":          "TALA",
		"/plugins/d2plugin-engine.v3":         "engine.v3",
		"/plugins/d2plugin-engine.v3.exe":     "engine.v3",
		"/plugins/d2plugin-engine.exe.backup": "engine.exe.backup",
	}
	for path, want := range tests {
		if got := externalPluginName(path); got != want {
			t.Errorf("externalPluginName(%q) = %q, want %q", path, got, want)
		}
	}
}

func copyExecutable(t *testing.T, destination string) {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
