package d2cli

import (
	"bytes"
	"context"
	"image/gif"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"
)

func isometricCLI(t *testing.T, source string, env []string, args ...string) ([]byte, string, error) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "input.d2")
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	state := &xmain.TestState{Run: Run, Args: append([]string{"d2", input}, args...), PWD: dir, Stdout: &stdout, Env: xos.NewEnv(append([]string{"PATH="}, env...))}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	state.Start(t, ctx)
	defer state.Cleanup(t)
	err := state.Wait(ctx)
	return stdout.Bytes(), dir, err
}

func TestIsometricModePrecedence(t *testing.T) {
	const sourceOn = "vars: {d2-config: {isometric: true}}\na -> b"
	const sourceOff = "vars: {d2-config: {isometric: false}}\na -> b"
	for _, test := range []struct {
		name, source string
		env, args    []string
		want         bool
	}{
		{"source", sourceOn, nil, nil, true},
		{"source disabled", sourceOff, nil, nil, false},
		{"flag", "a -> b", nil, []string{"--isometric"}, true},
		{"flag disables source", sourceOn, nil, []string{"--isometric=false"}, false},
		{"flag enables source", sourceOff, nil, []string{"--isometric"}, true},
		{"environment", "a -> b", []string{"D2_ISOMETRIC=true"}, nil, true},
		{"environment disables source", sourceOn, []string{"D2_ISOMETRIC=false"}, nil, false},
		{"flag overrides environment", sourceOn, []string{"D2_ISOMETRIC=true"}, []string{"--isometric=false"}, false},
		{"flag enables over environment", sourceOff, []string{"D2_ISOMETRIC=false"}, []string{"--isometric"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append(append([]string{}, test.args...), "--scale=.2", "--stdout-format=svg", "-")
			data, _, err := isometricCLI(t, test.source, test.env, args...)
			if err != nil {
				t.Fatal(err)
			}
			got := bytes.Contains(data, []byte("D2 isometric diagram"))
			if got != test.want {
				t.Fatalf("isometric output = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIsometricDefaultSVGAndSourceParity(t *testing.T) {
	source := "system: {first: {a}; second: {b}}\nsystem.first.a -> system.second.b"
	_, dir, err := isometricCLI(t, source, nil, "--isometric", "--scale=.2")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "input.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("D2 isometric diagram")) {
		t.Fatal("default export is not isometric SVG")
	}
	baseline, _, err := isometricCLI(t, source, nil, "--isometric", "--scale=.2", "--stdout-format=svg", "-")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, baseline) {
		t.Fatal("implicit and explicit SVG output must match")
	}
	data, _, err = isometricCLI(t, "vars: {d2-config: {isometric: true}}\n"+source, nil, "--scale=.2", "--stdout-format=svg", "-")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, baseline) {
		t.Fatal("source and flag isometric output must match")
	}
}

func TestIsometricModeFailures(t *testing.T) {
	for _, test := range []struct {
		name, source, want string
		args               []string
	}{
		{"source sketch", "vars: {d2-config: {sketch: true; isometric: true}}\na", "sketch cannot", []string{"out.svg"}},
		{"flag sketch", "a", "sketch cannot", []string{"--sketch", "--isometric", "out.svg"}},
		{"source unsupported format", "vars: {d2-config: {isometric: true}}\na", "SVG, PNG, GIF, PDF or PPTX", []string{"out.txt"}},
		{"source HTML filename", "vars: {d2-config: {isometric: true}}\na", "SVG, PNG, GIF, PDF or PPTX", []string{"out.html"}},
		{"source zero scale", "vars: {d2-config: {isometric: true}}\na", "finite and positive", []string{"--scale=0", "out.svg"}},
		{"source non-finite scale", "vars: {d2-config: {isometric: true}}\na", "finite and positive", []string{"--scale=NaN", "out.svg"}},
		{"source appendix", "vars: {d2-config: {isometric: true}}\na", "--force-appendix", []string{"--force-appendix", "out.svg"}},
		{"source dark theme", "vars: {d2-config: {isometric: true; dark-theme-id: 200}}\na", "dark-theme", []string{"out.svg"}},
		{"flag dark theme", "a", "dark-theme", []string{"--isometric", "--dark-theme=200", "out.svg"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, dir, err := isometricCLI(t, test.source, nil, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if len(data) != 0 {
				t.Fatal("failed export wrote stdout")
			}
			if _, err := os.Stat(filepath.Join(dir, "out.svg")); !os.IsNotExist(err) {
				t.Fatalf("failed export created output: %v", err)
			}
		})
	}
}

func TestIsometricSourceGIFUsesSameAnimationAsFlag(t *testing.T) {
	args := []string{"--scale=.2", "--stdout-format=gif", "-"}
	fromSource, _, err := isometricCLI(t, "vars: {d2-config: {isometric: true}}\na -> b: {style.animated: true}", nil, args...)
	if err != nil {
		t.Fatal(err)
	}
	fromFlag, _, err := isometricCLI(t, "a -> b: {style.animated: true}", nil, append([]string{"--isometric"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{fromSource, fromFlag} {
		animation, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(animation.Image) < 2 {
			t.Fatal("isometric GIF lost its animated traffic cycle")
		}
	}
	if !bytes.Equal(fromSource, fromFlag) {
		t.Fatal("source and flag mode selection produced different GIF animation")
	}
}

func TestIsometricSourceConfigAppliesToSelectedBoards(t *testing.T) {
	source := "vars: {d2-config: {isometric: true}}\na\nlayers: {detail: {b}}\nscenarios: {alternate: {c}}\nsteps: {next: {d}}"
	for _, target := range []string{"", "layers.detail", "scenarios.alternate", "steps.next"} {
		t.Run(target, func(t *testing.T) {
			data, _, err := isometricCLI(t, source, nil, "--target="+target, "--scale=.2", "--stdout-format=svg", "-")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("D2 isometric diagram")) {
				t.Fatal("selected board did not inherit root isometric mode")
			}
		})
	}
}

func TestIsometricSVGScaleIsIndependentOfRasterLimits(t *testing.T) {
	for _, scale := range []string{".001", "10"} {
		t.Run(scale, func(t *testing.T) {
			data, _, err := isometricCLI(t, "a -> b", nil, "--isometric", "--scale="+scale, "--stdout-format=svg", "-")
			if err != nil {
				t.Fatal(err)
			}
			attrs, _ := inspectIsometricSVG(t, data)
			width, err := strconv.Atoi(attrs["width"])
			if err != nil || width < 1 {
				t.Fatalf("invalid SVG width %q: %v", attrs["width"], err)
			}
			if _, _, err := isometricCLI(t, "a -> b", nil, "--isometric", "--scale="+scale, "--stdout-format=png", "-"); err == nil {
				t.Fatal("PNG bypassed its bitmap allocation limits")
			}
		})
	}
}
