package e2etests_cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
	"oss.terrastruct.com/util-go/xmain"
	"oss.terrastruct.com/util-go/xos"

	"oss.terrastruct.com/d2/d2cli"
	"oss.terrastruct.com/d2/lib/pptx"
	"oss.terrastruct.com/d2/lib/xgif"
)

func TestCLI_E2E(t *testing.T) {
	t.Parallel()

	tca := []struct {
		name   string
		skipCI bool
		skip   bool
		run    func(t *testing.T, ctx context.Context, dir string, env *xos.Env)
	}{
		{
			name:   "hello_world_png",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				png := readFile(t, dir, "hello-world.png")
				testdataIgnoreDiff(t, ".png", png)
			},
		},
		{
			name:   "hello_world_png_pad",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--pad=400", "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				png := readFile(t, dir, "hello-world.png")
				testdataIgnoreDiff(t, ".png", png)
			},
		},
		{
			name: "center",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--center=true", "hello-world.d2")
				assert.Success(t, err)
				svg := readFile(t, dir, "hello-world.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "flags-panic",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "layout", "dagre", "--dagre-nodesep", "50", "hello-world.d2")
				assert.ErrorString(t, err, `failed to wait xmain test: e2etests-cli/d2: failed to unmarshal input to graph: `)
			},
		},
		{
			name: "empty-layer",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "empty-layer.d2", `layers: { x: {} }`)
				err := runTestMain(t, ctx, dir, env, "empty-layer.d2")
				assert.Success(t, err)
			},
		},
		{
			name: "layer-link",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "test.d2", `doh: { link: layers.test2 }; layers: { test2: @test2.d2 }`)
				writeFile(t, dir, "test2.d2", `x: I'm a Mac { link: https://example.com }`)
				err := runTestMain(t, ctx, dir, env, "test.d2", "layer-link.svg")
				assert.Success(t, err)

				assert.TestdataDir(t, filepath.Join(dir, "layer-link"))
			},
		},
		{
			// Skip the empty base board so the animation doesn't show blank for 1400ms
			name: "empty-base",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "empty-base.d2", `steps: {
  1: {
    a -> b
  }
  2: {
    b -> d
    c -> d
  }
  3: {
    d -> e
  }
}`)

				err := runTestMain(t, ctx, dir, env, "--animate-interval=1400", "empty-base.d2")
				assert.Success(t, err)
				svg := readFile(t, dir, "empty-base.svg")
				assert.Testdata(t, ".svg", svg)
				assert.Equal(t, 3, getNumBoards(string(svg)))
			},
		},
		{
			name: "animation",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "animation.d2", `Chicken's plan: {
  style.font-size: 35
  near: top-center
  shape: text
}

steps: {
  1: {
    Approach road
  }
  2: {
    Approach road -> Cross road
  }
  3: {
    Cross road -> Make you wonder why
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "--animate-interval=1400", "animation.d2")
				assert.Success(t, err)
				svg := readFile(t, dir, "animation.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "vars-animation",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "animation.d2", `vars: {
  d2-config: {
    theme-id: 300
  }
}
Chicken's plan: {
  style.font-size: 35
  near: top-center
  shape: text
}

steps: {
  1: {
    Approach road
  }
  2: {
    Approach road -> Cross road
  }
  3: {
    Cross road -> Make you wonder why
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "--animate-interval=1400", "animation.d2")
				assert.Success(t, err)
				svg := readFile(t, dir, "animation.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "linked-path",
			// TODO tempdir is random, resulting in different test results each time with the links
			skip: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "linked.d2", `cat: how does the cat go? {
  link: layers.cat
}
layers: {
  cat: {
    home: {
      link: _
    }
    the cat -> meow: goes

    scenarios: {
      big cat: {
        the cat -> roar: goes
      }
    }
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "linked.d2")
				assert.Success(t, err)

				assert.TestdataDir(t, filepath.Join(dir, "linked"))
			},
		},
		{
			name: "with-font",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "font.d2", `a: Why do computers get sick often?
b: Because their Windows are always open!
a -> b: italic font
`)
				err := runTestMain(t, ctx, dir, env, "--font-bold=./RockSalt-Regular.ttf", "font.d2")
				assert.Success(t, err)
				svg := readFile(t, dir, "font.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "incompatible-animation",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "x.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--animate-interval=2", "x.d2", "x.png")
				assert.ErrorString(t, err, `failed to wait xmain test: e2etests-cli/d2: bad usage: -animate-interval can only be used when exporting to SVG or GIF.
You provided: .png`)
			},
		},
		{
			name:   "hello_world_png_sketch",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--sketch", "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				png := readFile(t, dir, "hello-world.png")
				// https://github.com/terrastruct/d2/pull/963#pullrequestreview-1323089392
				testdataIgnoreDiff(t, ".png", png)
			},
		},
		{
			name: "target-root",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "target-root.d2", `title: {
	label: Main Plan
}
scenarios: {
	b: {
	title.label: Backup Plan
	}
}`)
				err := runTestMain(t, ctx, dir, env, "--target", "", "target-root.d2", "target-root.svg")
				assert.Success(t, err)
				svg := readFile(t, dir, "target-root.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "target-b",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "target-b.d2", `title: {
	label: Main Plan
}
scenarios: {
	b: {
	title.label: Backup Plan
	}
}`)
				err := runTestMain(t, ctx, dir, env, "--target", "b", "target-b.d2", "target-b.svg")
				assert.Success(t, err)
				svg := readFile(t, dir, "target-b.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "target-nested-with-special-chars",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "target-nested-with-special-chars.d2", `layers: {
	a: {
		layers: {
			"x / y . z": {
				mad
			}
		}
	}
}`)
				err := runTestMain(t, ctx, dir, env, "--target", `layers.a.layers."x / y . z"`, "target-nested-with-special-chars.d2", "target-nested-with-special-chars.svg")
				assert.Success(t, err)
				svg := readFile(t, dir, "target-nested-with-special-chars.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "target-invalid",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "target-invalid.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--target", "b", "target-invalid.d2", "target-invalid.svg")
				assert.ErrorString(t, err, `failed to wait xmain test: e2etests-cli/d2: failed to compile target-invalid.d2: render target "b" not found`)
			},
		},
		{
			name: "multiboard/life",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "life.d2", `x -> y
layers: {
  core: {
    belief
    food
    diet
  }
  broker: {
    mortgage
    realtor
  }
  stocks: {
    TSX
    NYSE
    NASDAQ
  }
}

scenarios: {
  why: {
    y -> x
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "life.d2")
				assert.Success(t, err)

				assert.TestdataDir(t, filepath.Join(dir, "life"))
			},
		},
		{
			name: "multiboard/life_index_d2",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "life/index.d2", `x -> y
layers: {
  core: {
    belief
    food
    diet
  }
  broker: {
    mortgage
    realtor
  }
  stocks: {
    TSX
    NYSE
    NASDAQ
  }
}

scenarios: {
  why: {
    y -> x
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "life")
				assert.Success(t, err)

				assert.TestdataDir(t, filepath.Join(dir, "life"))
			},
		},
		{
			name: "internal_linked_pdf",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `cat: how does the cat go? {
  link: layers.cat
}
layers: {
  cat: {
    home: {
      link: _
    }
    the cat -> meow: goes
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "in.d2", "out.pdf")
				assert.Success(t, err)

				pdf := readFile(t, dir, "out.pdf")
				testdataIgnoreDiff(t, ".pdf", pdf)
			},
		},
		{
			name: "export_ppt",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "x.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "x.d2", "x.ppt")
				assert.ErrorString(t, err, `failed to wait xmain test: e2etests-cli/d2: bad usage: D2 does not support ppt exports, did you mean "pptx"?`)
			},
		},
		{
			name:   "how_to_solve_problems_pptx",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `how to solve a hard problem? {
	link: steps.2
}
steps: {
	1: {
		w: write down the problem
	}
	2: {
		w -> t
		t: think really hard about it
	}
	3: {
		t -> w2
		w2: write down the solution
		w2: {
			link: https://d2lang.com
		}
	}
}
`)
				err := runTestMain(t, ctx, dir, env, "in.d2", "how_to_solve_problems.pptx")
				assert.Success(t, err)

				file := readFile(t, dir, "how_to_solve_problems.pptx")
				err = pptx.Validate(file, 4)
				assert.Success(t, err)
			},
		},
		{
			name:   "how_to_solve_problems_gif",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `how to solve a hard problem? {
	link: steps.2
}
steps: {
	1: {
		w: write down the problem
	}
	2: {
		w -> t
		t: think really hard about it
	}
	3: {
		t -> w2
		w2: write down the solution
		w2: {
			link: https://d2lang.com
		}
	}
}
`)
				err := runTestMain(t, ctx, dir, env, "--animate-interval=10", "in.d2", "how_to_solve_problems.gif")
				assert.Success(t, err)

				gifBytes := readFile(t, dir, "how_to_solve_problems.gif")
				err = xgif.Validate(gifBytes, 4, 10)
				assert.Success(t, err)
			},
		},
		{
			name:   "one-layer-gif",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x`)
				err := runTestMain(t, ctx, dir, env, "--animate-interval=10", "in.d2", "out.gif")
				assert.Success(t, err)

				gifBytes := readFile(t, dir, "out.gif")
				err = xgif.Validate(gifBytes, 1, 10)
				assert.Success(t, err)
			},
		},
		{
			name: "stdin",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				stdin := bytes.NewBufferString(`x -> y`)
				stdout := &bytes.Buffer{}
				tms := testMain(dir, env, "-")
				tms.Stdin = stdin
				tms.Stdout = stdout
				tms.Start(t, ctx)
				defer tms.Cleanup(t)
				err := tms.Wait(ctx)
				assert.Success(t, err)

				assert.Testdata(t, ".svg", stdout.Bytes())
			},
		},
		{
			name: "abspath",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, filepath.Join(dir, "hello-world.d2"))
				assert.Success(t, err)
				svg := readFile(t, dir, "hello-world.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "import",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x: @x; y: @y; ...@p`)
				writeFile(t, dir, "x.d2", `shape: circle`)
				writeFile(t, dir, "y.d2", `shape: square`)
				writeFile(t, dir, "p.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, filepath.Join(dir, "hello-world.d2"))
				assert.Success(t, err)
				svg := readFile(t, dir, "hello-world.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "import_vars",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `vars: { d2-config: @config }; x -> y`)
				writeFile(t, dir, "config.d2", `theme-id: 200`)
				err := runTestMain(t, ctx, dir, env, filepath.Join(dir, "hello-world.d2"))
				assert.Success(t, err)
				svg := readFile(t, dir, "hello-world.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "import_spread_nested",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `...@x.y`)
				writeFile(t, dir, "x.d2", `y: { jon; jan }`)
				err := runTestMain(t, ctx, dir, env, filepath.Join(dir, "hello-world.d2"))
				assert.Success(t, err)
				svg := readFile(t, dir, "hello-world.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "chain_import",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `...@x`)
				writeFile(t, dir, "x.d2", `...@y`)
				writeFile(t, dir, "y.d2", `meow`)
				err := runTestMain(t, ctx, dir, env, filepath.Join(dir, "hello-world.d2"))
				assert.Success(t, err)
				svg := readFile(t, dir, "hello-world.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "board_import",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x.link: layers.x; layers: { x: @x }`)
				writeFile(t, dir, "x.d2", `y.link: layers.y; layers: { y: @y }`)
				writeFile(t, dir, "y.d2", `meow`)
				err := runTestMain(t, ctx, dir, env, filepath.Join(dir, "hello-world.d2"))
				assert.Success(t, err)
				t.Run("hello-world-x-y", func(t *testing.T) {
					svg := readFile(t, dir, "hello-world/x/y.svg")
					assert.Testdata(t, ".svg", svg)
				})
				t.Run("hello-world-x", func(t *testing.T) {
					svg := readFile(t, dir, "hello-world/x/index.svg")
					assert.Testdata(t, ".svg", svg)
				})
				t.Run("hello-world", func(t *testing.T) {
					svg := readFile(t, dir, "hello-world/index.svg")
					assert.Testdata(t, ".svg", svg)
				})
			},
		},
		{
			name: "vars-config",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `vars: {
  d2-config: {
    sketch: true
    layout-engine: elk
  }
}
x -> y -> a.dream
it -> was -> all -> a.dream
i used to read
`)
				env.Setenv("D2_THEME", "1")
				err := runTestMain(t, ctx, dir, env, "--pad=10", "hello-world.d2")
				assert.Success(t, err)
				svg := readFile(t, dir, "hello-world.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "basic-fmt",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x ---> y`)
				err := runTestMainPersist(t, ctx, dir, env, "fmt", "hello-world.d2")
				assert.Success(t, err)
				got := readFile(t, dir, "hello-world.d2")
				assert.Equal(t, "x -> y\n", string(got))
			},
		},
		{
			name: "fmt-multiple-files",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "foo.d2", `a ---> b`)
				writeFile(t, dir, "bar.d2", `x ---> y`)
				err := runTestMainPersist(t, ctx, dir, env, "fmt", "foo.d2", "bar.d2")
				assert.Success(t, err)
				gotFoo := readFile(t, dir, "foo.d2")
				gotBar := readFile(t, dir, "bar.d2")
				assert.Equal(t, "a -> b\n", string(gotFoo))
				assert.Equal(t, "x -> y\n", string(gotBar))
			},
		},
		{
			name: "watch-regular",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "index.d2", `
a -> b
b.link: layers.cream

layers: {
    cream: {
        c -> b
    }
}`)
				stderr := &bytes.Buffer{}
				tms := testMain(dir, env, "--watch", "--browser=0", "index.d2")
				tms.Stderr = stderr

				tms.Start(t, ctx)
				defer func() {
					// Manually close, since watcher is daemon
					err := tms.Signal(ctx, os.Interrupt)
					assert.Success(t, err)
				}()

				// Wait for watch server to spin up and listen
				urlRE := regexp.MustCompile(`127.0.0.1:([0-9]+)`)
				watchURL, err := waitLogs(ctx, stderr, urlRE)
				assert.Success(t, err)
				stderr.Reset()

				// Start a client
				c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s/watch", watchURL), nil)
				assert.Success(t, err)
				defer c.CloseNow()

				// Get the link
				_, msg, err := c.Read(ctx)
				assert.Success(t, err)
				aRE := regexp.MustCompile(`href=\\"([^\"]*)\\"`)
				match := aRE.FindSubmatch(msg)
				assert.Equal(t, 2, len(match))
				linkedPath := match[1]

				err = getWatchPage(ctx, t, fmt.Sprintf("http://%s/%s", watchURL, linkedPath))
				assert.Success(t, err)

				successRE := regexp.MustCompile(`broadcasting update to 1 client`)
				_, err = waitLogs(ctx, stderr, successRE)
				assert.Success(t, err)
			},
		},
		{
			name: "watch-ok-link",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				// This link technically works because D2 interprets it as a URL,
				// and on local filesystem, that is whe path where the compilation happens
				// to output it to.
				writeFile(t, dir, "index.d2", `
a -> b
b.link: cream

layers: {
    cream: {
        c -> b
    }
}`)
				stderr := &bytes.Buffer{}
				tms := testMain(dir, env, "--watch", "--browser=0", "index.d2")
				tms.Stderr = stderr

				tms.Start(t, ctx)
				defer func() {
					// Manually close, since watcher is daemon
					err := tms.Signal(ctx, os.Interrupt)
					assert.Success(t, err)
				}()

				// Wait for watch server to spin up and listen
				urlRE := regexp.MustCompile(`127.0.0.1:([0-9]+)`)
				watchURL, err := waitLogs(ctx, stderr, urlRE)
				assert.Success(t, err)

				stderr.Reset()

				// Start a client
				c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s/watch", watchURL), nil)
				assert.Success(t, err)
				defer c.CloseNow()

				// Get the link
				_, msg, err := c.Read(ctx)
				assert.Success(t, err)
				aRE := regexp.MustCompile(`href=\\"([^\"]*)\\"`)
				match := aRE.FindSubmatch(msg)
				assert.Equal(t, 2, len(match))
				linkedPath := match[1]

				err = getWatchPage(ctx, t, fmt.Sprintf("http://%s/%s", watchURL, linkedPath))
				assert.Success(t, err)

				successRE := regexp.MustCompile(`broadcasting update to 1 client`)
				_, err = waitLogs(ctx, stderr, successRE)
				assert.Success(t, err)
			},
		},
		{
			name: "watch-bad-link",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				// Just verify we don't crash even with a bad link (it's treated as a URL, which users might have locally)
				writeFile(t, dir, "index.d2", `
a -> b
b.link: dream

layers: {
    cream: {
        c -> b
    }
}`)
				stderr := &bytes.Buffer{}
				tms := testMain(dir, env, "--watch", "--browser=0", "index.d2")
				tms.Stderr = stderr

				tms.Start(t, ctx)
				defer func() {
					// Manually close, since watcher is daemon
					err := tms.Signal(ctx, os.Interrupt)
					assert.Success(t, err)
				}()

				// Wait for watch server to spin up and listen
				urlRE := regexp.MustCompile(`127.0.0.1:([0-9]+)`)
				watchURL, err := waitLogs(ctx, stderr, urlRE)
				assert.Success(t, err)
				stderr.Reset()

				// Start a client
				c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s/watch", watchURL), nil)
				assert.Success(t, err)
				defer c.CloseNow()

				// Get the link
				_, msg, err := c.Read(ctx)
				assert.Success(t, err)
				aRE := regexp.MustCompile(`href=\\"([^\"]*)\\"`)
				match := aRE.FindSubmatch(msg)
				assert.Equal(t, 2, len(match))
				linkedPath := match[1]

				err = getWatchPage(ctx, t, fmt.Sprintf("http://%s/%s", watchURL, linkedPath))
				assert.Success(t, err)

				successRE := regexp.MustCompile(`broadcasting update to 1 client`)
				_, err = waitLogs(ctx, stderr, successRE)
				assert.Success(t, err)
			},
		},
		{
			name: "watch-imported-file",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "a.d2", `
...@b
`)
				writeFile(t, dir, "b.d2", `
x
`)
				stderr := &bytes.Buffer{}
				tms := testMain(dir, env, "--watch", "--browser=0", "a.d2")
				tms.Stderr = stderr

				tms.Start(t, ctx)
				defer func() {
					err := tms.Signal(ctx, os.Interrupt)
					assert.Success(t, err)
				}()

				// Wait for first compilation to finish
				doneRE := regexp.MustCompile(`successfully compiled a.d2`)
				_, err := waitLogs(ctx, stderr, doneRE)
				assert.Success(t, err)
				stderr.Reset()

				// Test that writing an imported file will cause recompilation
				writeFile(t, dir, "b.d2", `
x -> y
`)
				bRE := regexp.MustCompile(`detected change in b.d2`)
				_, err = waitLogs(ctx, stderr, bRE)
				assert.Success(t, err)
				stderr.Reset()

				// Test burst of both files changing
				writeFile(t, dir, "a.d2", `
...@b
hey
`)
				writeFile(t, dir, "b.d2", `
x
hi
`)
				bothRE := regexp.MustCompile(`detected change in a.d2, b.d2`)
				_, err = waitLogs(ctx, stderr, bothRE)
				assert.Success(t, err)

				// Wait for that compilation to fully finish
				_, err = waitLogs(ctx, stderr, doneRE)
				assert.Success(t, err)
				stderr.Reset()

				// Update the main file to no longer have that dependency
				writeFile(t, dir, "a.d2", `
a
`)
				_, err = waitLogs(ctx, stderr, doneRE)
				assert.Success(t, err)
				stderr.Reset()

				// Change b
				writeFile(t, dir, "b.d2", `
y
`)
				// Change a to retrigger compilation
				// The test works by seeing that the report only says "a" changed, otherwise testing for omission of compilation from "b" would require waiting
				writeFile(t, dir, "a.d2", `
c
`)

				_, err = waitLogs(ctx, stderr, doneRE)
				assert.Success(t, err)
			},
		},
	}

	ctx := context.Background()
	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.skipCI && os.Getenv("CI") != "" {
				t.SkipNow()
			}
			if tc.skip {
				t.SkipNow()
			}

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
	err := runTestMainPersist(tb, ctx, dir, env, args...)
	if err != nil {
		return err
	}
	removeD2Files(tb, dir)
	return nil
}

func runTestMainPersist(tb testing.TB, ctx context.Context, dir string, env *xos.Env, args ...string) error {
	tms := testMain(dir, env, args...)
	tms.Start(tb, ctx)
	defer tms.Cleanup(tb)
	err := tms.Wait(ctx)
	if err != nil {
		return err
	}
	return nil
}

func writeFile(tb testing.TB, dir, fp, data string) {
	tb.Helper()
	err := os.MkdirAll(filepath.Dir(filepath.Join(dir, fp)), 0755)
	assert.Success(tb, err)
	assert.WriteFile(tb, filepath.Join(dir, fp), []byte(data), 0644)
}

func readFile(tb testing.TB, dir, fp string) []byte {
	tb.Helper()
	return assert.ReadFile(tb, filepath.Join(dir, fp))
}

func removeD2Files(tb testing.TB, dir string) {
	ea, err := os.ReadDir(dir)
	assert.Success(tb, err)

	for _, e := range ea {
		if e.IsDir() {
			removeD2Files(tb, filepath.Join(dir, e.Name()))
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext == ".d2" {
			assert.Remove(tb, filepath.Join(dir, e.Name()))
		}
	}
}

func testdataIgnoreDiff(tb testing.TB, ext string, got []byte) {
	_ = diff.Testdata(filepath.Join("testdata", tb.Name()), ext, got)
}

// getNumBoards gets the number of boards in an SVG file through a non-robust pattern search
// If the renderer changes, this must change
func getNumBoards(svg string) int {
	return strings.Count(svg, `class="d2`)
}

var errRE = regexp.MustCompile(`err:`)

func waitLogs(ctx context.Context, buf *bytes.Buffer, pattern *regexp.Regexp) (string, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var match string
	for i := 0; i < 100 && match == ""; i++ {
		select {
		case <-ticker.C:
			out := buf.String()
			match = pattern.FindString(out)
			errMatch := errRE.FindString(out)
			if errMatch != "" {
				return "", errors.New(buf.String())
			}
		case <-ctx.Done():
			ticker.Stop()
			return "", fmt.Errorf("could not match pattern in log. logs: %s", buf.String())
		}
	}
	if match == "" {
		return "", errors.New(buf.String())
	}

	return match, nil
}

func getWatchPage(ctx context.Context, t *testing.T, page string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", page, nil)
	if err != nil {
		return err
	}

	var httpClient = &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}
	return nil
}
