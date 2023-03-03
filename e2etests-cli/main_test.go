package e2etests_cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"oss.terrastruct.com/d2/d2cli"
	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/xmain"
	"oss.terrastruct.com/util-go/xos"
)

func TestCLI_E2E(t *testing.T) {
	t.Parallel()

	tca := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, dir string, env *xos.Env)
	}{
		{
			name: "hello_world_png",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				png := readFile(t, dir, "hello-world.png")
				assert.Testdata(t, ".png", png)
			},
		},
		{
			name: "hello_world_png_pad",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--pad=400", "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				png := readFile(t, dir, "hello-world.png")
				assert.Testdata(t, ".png", png)
			},
		},
		{
			name: "hello_world_png_sketch",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--sketch", "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				png := readFile(t, dir, "hello-world.png")
				assert.Testdata(t, ".png", png)
			},
		},
	}

	ctx := context.Background()
	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
			defer cancel()

			dir, cleanup := assert.TempDir(t)
			defer cleanup()

			env := xos.NewEnv(nil)

			tc.run(t, ctx, dir, env)
		})
	}
}

// We do not run the CLI in its own process even though that makes it not truly e2e to
// test whether we're cleaning up state correctly.
func testMain(dir string, env *xos.Env, args ...string) *xmain.TestState {
	return &xmain.TestState{
		Run:  d2cli.Run,
		Env:  env,
		Args: append([]string{"e2etests-cli/d2"}, args...),
		PWD:  dir,
	}
}

func runTestMain(tb testing.TB, ctx context.Context, dir string, env *xos.Env, args ...string) error {
	tms := testMain(dir, env, args...)
	tms.Start(tb, ctx)
	defer tms.Cleanup(tb)
	return tms.Wait(ctx)
}

func writeFile(tb testing.TB, dir, fp, data string) {
	tb.Helper()
	assert.WriteFile(tb, filepath.Join(dir, fp), []byte(data), 0644)
}

func readFile(tb testing.TB, dir, fp string) []byte {
	tb.Helper()
	return assert.ReadFile(tb, filepath.Join(dir, fp))
}
