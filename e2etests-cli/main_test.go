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

	"github.com/coder/websocket"

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
		serial bool
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
			name: "sequence-layer",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "index.d2", `k; layers: { seq: @seq.d2 }`)
				writeFile(t, dir, "seq.d2", `shape: sequence_diagram
a: me
b: github.com/terrastruct/d2

a -> b: issue about a bug
a."some note about the bug"

if i'm right: {
	a <- b: fix
}

if i'm wrong: {
	a <- b: nah, intended
}`)
				err := runTestMain(t, ctx, dir, env, "index.d2")
				assert.Success(t, err)

				assert.TestdataDir(t, filepath.Join(dir, "index"))
			},
		},
		{
			name: "sequence-spread-layer",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "index.d2", `k; layers: { seq: {...@seq.d2} }`)
				writeFile(t, dir, "seq.d2", `shape: sequence_diagram
a: me
b: github.com/terrastruct/d2

a -> b: issue about a bug
a."some note about the bug"

if i'm right: {
	a <- b: fix
}

if i'm wrong: {
	a <- b: nah, intended
}`)
				err := runTestMain(t, ctx, dir, env, "index.d2")
				assert.Success(t, err)

				assert.TestdataDir(t, filepath.Join(dir, "index"))
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
				assert.ErrorString(t, err, `failed to wait xmain test: e2etests-cli/d2: bad usage: --animate-interval can only be used when exporting to SVG or GIF.
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
			name: "target-nested-index",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "target-nested-index.d2", `a
layers: {
	l1: {
		b
		layers: {
			index: {
				c
				layers: {
					l3: {
						d
					}
				}
			}
		}
	}
}`)
				err := runTestMain(t, ctx, dir, env, "--target", `l1.index.l3`, "target-nested-index.d2", "target-nested-index.svg")
				assert.Success(t, err)
				svg := readFile(t, dir, "target-nested-index.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "target-nested-index2",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "target-nested-index2.d2", `a
layers: {
	index: {
		b
		layers: {
			nest1: {
				c
				scenarios: {
					nest2: {
						d
					}
				}
			}
		}
	}
}`)
				err := runTestMain(t, ctx, dir, env, "--target", `index.nest1.nest2`, "target-nested-index2.d2", "target-nested-index2.svg")
				assert.Success(t, err)
				svg := readFile(t, dir, "target-nested-index2.svg")
				assert.Testdata(t, ".svg", svg)
			},
		},
		{
			name: "theme-override",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "theme-override.d2", `
direction: right
vars: {
  d2-config: {
    theme-overrides: {
      B1: "#2E7D32"
      B2: "#66BB6A"
      B3: "#A5D6A7"
      B4: "#C5E1A5"
      B5: "#E6EE9C"
      B6: "#FFF59D"

      AA2: "#0D47A1"
      AA4: "#42A5F5"
      AA5: "#90CAF9"

      AB4: "#F44336"
      AB5: "#FFCDD2"

      N1: "#2E2E2E"
      N2: "#2E2E2E"
      N3: "#595959"
      N4: "#858585"
      N5: "#B1B1B1"
      N6: "#DCDCDC"
      N7: "#DCDCDC"
    }
    dark-theme-overrides: {
      B1: "#2E7D32"
      B2: "#66BB6A"
      B3: "#A5D6A7"
      B4: "#C5E1A5"
      B5: "#E6EE9C"
      B6: "#FFF59D"

      AA2: "#0D47A1"
      AA4: "#42A5F5"
      AA5: "#90CAF9"

      AB4: "#F44336"
      AB5: "#FFCDD2"

      N1: "#2E2E2E"
      N2: "#2E2E2E"
      N3: "#595959"
      N4: "#858585"
      N5: "#B1B1B1"
      N6: "#DCDCDC"
      N7: "#DCDCDC"
    }
  }
}

logs: {
  shape: page
  style.multiple: true
}
user: User {shape: person}
network: Network {
  tower: Cell Tower {
    satellites: {
      shape: stored_data
      style.multiple: true
    }

    satellites -> transmitter
    satellites -> transmitter
    satellites -> transmitter
    transmitter
  }
  processor: Data Processor {
    storage: Storage {
      shape: cylinder
      style.multiple: true
    }
  }
  portal: Online Portal {
    UI
  }

  tower.transmitter -> processor: phone logs
}
server: API Server

user -> network.tower: Make call
network.processor -> server
network.processor -> server
network.processor -> server

server -> logs
server -> logs
server -> logs: persist

server -> network.portal.UI: display
user -> network.portal.UI: access {
  style.stroke-dash: 3
}

costumes: {
  shape: sql_table
  id: int {constraint: primary_key}
  silliness: int
  monster: int
  last_updated: timestamp
}

monsters: {
  shape: sql_table
  id: int {constraint: primary_key}
  movie: string
  weight: int
  last_updated: timestamp
}

costumes.monster -> monsters.id
`)
				err := runTestMain(t, ctx, dir, env, "theme-override.d2", "theme-override.svg")
				assert.Success(t, err)
				svg := readFile(t, dir, "theme-override.svg")
				assert.Testdata(t, ".svg", svg)
				// theme color is used in SVG
				assert.NotEqual(t, -1, strings.Index(string(svg), "#2E2E2E"))
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
			name:   "pptx-theme-overrides",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `vars:{
  d2-config: {
    theme-overrides: {
			# All red
      N1:  "#ff0000"
      B1:  "#ff0000"
      B2:  "#ff0000"
      AA2: "#ff0000"
      N2:  "#ff0000"
      N6:  "#ff0000"
      B4:  "#ff0000"
      B5:  "#ff0000"
      B3:  "#ff0000"
      N4:  "#ff0000"
      N5:  "#ff0000"
      AA4: "#ff0000"
      AB4: "#ff0000"
      B6:  "#ff0000"
      N7:  "#ff0000"
      AA5: "#ff0000"
      AB5: "#ff0000"
    }
  }
}
a->z
a.b.c.d
`)
				err := runTestMain(t, ctx, dir, env, "in.d2", "all_red.pptx")
				assert.Success(t, err)
				pptx := readFile(t, dir, "all_red.pptx")
				testdataIgnoreDiff(t, ".pptx", pptx)
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
			name: "import_icon_relative",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `...@asdf/x`)
				writeFile(t, filepath.Join(dir, "asdf"), "x.d2", `y: { icon: ./blah.svg }; z: { icon: ../root.svg }`)
				writeFile(t, filepath.Join(dir, "asdf"), "blah.svg", ``)
				writeFile(t, dir, "root.svg", ``)
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
			name: "chain_icon_import",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `...@y
hello.class: Ecs`)
				writeFile(t, dir, "y.d2", `
...@x
classes: {
    Ecs: {
        shape: "circle"
        icon: ${icons.ecs}
    }
}
`)
				writeFile(t, dir, "x.d2", `
vars: {
    icons: {
        ecs: "https://icons.terrastruct.com/aws%2FCompute%2FAmazon-Elastic-Container-Service.svg"
    }
}
`)
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
			name:   "theme-pdf",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--theme=5", "in.d2", "out.pdf")
				assert.Success(t, err)

				pdf := readFile(t, dir, "out.pdf")
				testdataIgnoreDiff(t, ".pdf", pdf)
			},
		},
		{
			name:   "renamed-board",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `cat: how does the cat go? {
  link: layers.cat
}
a.link: "https://www.google.com/maps/place/Smoked+Out+BBQ/@37.3848007,-121.9513887,17z/data=!3m1!4b1!4m6!3m5!1s0x808fc9182ad4d38d:0x8e2f39c3e927b296!8m2!3d37.3848007!4d-121.9492!16s%2Fg%2F11gjt85zvf"
label: blah
layers: {
  cat: {
    label: dog
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
			name:   "no-nav-pdf",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `cat: how does the cat go? {
  link: layers.cat
}
a.link: "https://www.google.com/maps/place/Smoked+Out+BBQ/@37.3848007,-121.9513887,17z/data=!3m1!4b1!4m6!3m5!1s0x808fc9182ad4d38d:0x8e2f39c3e927b296!8m2!3d37.3848007!4d-121.9492!16s%2Fg%2F11gjt85zvf"
label: ""
layers: {
  cat: {
    label: dog
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
			name:   "no-nav-pptx",
			skipCI: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `cat: how does the cat go? {
  link: layers.cat
}
a.link: "https://www.google.com/maps/place/Smoked+Out+BBQ/@37.3848007,-121.9513887,17z/data=!3m1!4b1!4m6!3m5!1s0x808fc9182ad4d38d:0x8e2f39c3e927b296!8m2!3d37.3848007!4d-121.9492!16s%2Fg%2F11gjt85zvf"
label: ""
layers: {
  cat: {
    label: dog
    home: {
      link: _
    }
    the cat -> meow: goes
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "in.d2", "out.pptx")
				assert.Success(t, err)

				file := readFile(t, dir, "out.pptx")
				// err = pptx.Validate(file, 2)
				assert.Success(t, err)
				testdataIgnoreDiff(t, ".pptx", file)
			},
		},
		{
			name: "no_xml_tag",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "test.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--no-xml-tag", "test.d2", "no-xml.svg")
				assert.Success(t, err)
				noXMLSvg := readFile(t, dir, "no-xml.svg")
				assert.False(t, strings.Contains(string(noXMLSvg), "<?xml"))

				writeFile(t, dir, "test.d2", `x -> y`)
				err = runTestMain(t, ctx, dir, env, "test.d2", "with-xml.svg")
				assert.Success(t, err)
				withXMLSvg := readFile(t, dir, "with-xml.svg")
				assert.True(t, strings.Contains(string(withXMLSvg), "<?xml"))

				env.Setenv("D2_NO_XML_TAG", "1")
				writeFile(t, dir, "test.d2", `x -> y`)
				err = runTestMain(t, ctx, dir, env, "test.d2", "no-xml-env.svg")
				assert.Success(t, err)
				noXMLEnvSvg := readFile(t, dir, "no-xml-env.svg")
				assert.False(t, strings.Contains(string(noXMLEnvSvg), "<?xml"))
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
			name: "fmt-check-unformatted",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "foo.d2", `a ---> b`)
				writeFile(t, dir, "bar.d2", `x ---> y`)
				writeFile(t, dir, "baz.d2", "a -> z\n")
				err := runTestMainPersist(t, ctx, dir, env, "fmt", "--check", "foo.d2", "bar.d2", "baz.d2")
				assert.ErrorString(t, err, "failed to wait xmain test: e2etests-cli/d2: failed to fmt: exiting with code 1: found 2 unformatted files. Run d2 fmt to fix.")
				gotFoo := readFile(t, dir, "foo.d2")
				gotBar := readFile(t, dir, "bar.d2")
				assert.Equal(t, "a ---> b", string(gotFoo))
				assert.Equal(t, "x ---> y", string(gotBar))
			},
		},
		{
			name: "fmt-check-formatted",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "foo.d2", "a -> b\n")
				writeFile(t, dir, "bar.d2", "x -> y\n")
				err := runTestMainPersist(t, ctx, dir, env, "fmt", "--check", "foo.d2", "bar.d2")
				assert.Success(t, err)
			},
		},
		{
			name:   "watch-regular",
			serial: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "index.d2", `
a -> b
b.link: layers.cream

layers: {
    cream: {
        c -> b
    }
}`)
				stderr := &stderrWrapper{}
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
			name:   "watch-ok-link",
			serial: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "index.d2", `
a -> b
b.link: layers.cream

layers: {
    cream: {
        c -> b
    }
}`)
				stderr := &stderrWrapper{}
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
			name:   "watch-underscore-link",
			serial: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "index.d2", `
bobby

layers: {
    cream: {
			back.link: _
    }
}`)
				stderr := &stderrWrapper{}
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

				_, _, err = c.Read(ctx)
				assert.Success(t, err)

				err = getWatchPage(ctx, t, fmt.Sprintf("http://%s/%s", watchURL, "cream"))
				assert.Success(t, err)

				// Get the link
				_, msg, err := c.Read(ctx)
				aRE := regexp.MustCompile(`href=\\"([^\"]*)\\"`)
				match := aRE.FindSubmatch(msg)
				assert.Equal(t, 2, len(match))

				link := string(match[1])

				err = getWatchPage(ctx, t, fmt.Sprintf("http://%s/%s", watchURL, link))
				assert.Success(t, err)
				_, _, err = c.Read(ctx)
				assert.Success(t, err)
				successRE := regexp.MustCompile(`broadcasting update to 1 client`)
				_, err = waitLogs(ctx, stderr, successRE)
				assert.Success(t, err)
			},
		},
		{
			name:   "watch-nested-layer-link",
			serial: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "index.d2", `
a: {
  link: layers.b
}

layers: {
  b: {
    hi

    layers: {
      hey: {
        hey
      }
    }
  }
}`)
				stderr := &stderrWrapper{}
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
				aRE := regexp.MustCompile(`href=\\"([^\"]*)\\"`)
				match := aRE.FindSubmatch(msg)
				assert.Equal(t, 2, len(match))
				link := string(match[1])

				err = getWatchPage(ctx, t, fmt.Sprintf("http://%s/%s", watchURL, link))
				assert.Success(t, err)
				_, _, err = c.Read(ctx)
				assert.Success(t, err)
				successRE := regexp.MustCompile(`broadcasting update to 1 client`)
				_, err = waitLogs(ctx, stderr, successRE)
				assert.Success(t, err)
			},
		},
		{
			name:   "watch-imported-file",
			serial: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "a.d2", `
...@b
`)
				writeFile(t, dir, "b.d2", `
x
`)
				stderr := &stderrWrapper{}
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
		{
			name: "validate-against-correct-d2",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "correct.d2", `x -> y`)
				err := runTestMainPersist(t, ctx, dir, env, "validate", "correct.d2")
				assert.Success(t, err)
			},
		},
		{
			name: "validate-against-incorrect-d2",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "incorrect.d2", `x > y`)
				err := runTestMainPersist(t, ctx, dir, env, "validate", "incorrect.d2")
				assert.Error(t, err)
			},
		},
		{
			name: "omit-version",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "test.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--omit-version", "test.d2", "no-version.svg")
				assert.Success(t, err)
				noVersionSvg := readFile(t, dir, "no-version.svg")
				assert.False(t, strings.Contains(string(noVersionSvg), "data-d2-version="))

				writeFile(t, dir, "test.d2", `x -> y`)
				err = runTestMain(t, ctx, dir, env, "test.d2", "with-version.svg")
				assert.Success(t, err)
				withVersionSvg := readFile(t, dir, "with-version.svg")
				assert.True(t, strings.Contains(string(withVersionSvg), "data-d2-version="))

				env.Setenv("OMIT_VERSION", "1")
				writeFile(t, dir, "test.d2", `x -> y`)
				err = runTestMain(t, ctx, dir, env, "test.d2", "no-version-env.svg")
				assert.Success(t, err)
				noVersionEnvSvg := readFile(t, dir, "no-version-env.svg")
				assert.False(t, strings.Contains(string(noVersionEnvSvg), "data-d2-version="))
			},
		},
	}

	ctx := context.Background()
	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !tc.serial {
				t.Parallel()
			}

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
	re := regexp.MustCompile(`class="d2-\d+`)
	matches := re.FindAllString(svg, -1)
	return len(matches)
}

var errRE = regexp.MustCompile(`err:`)

func waitLogs(ctx context.Context, stream *stderrWrapper, pattern *regexp.Regexp) (string, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var match string
	for i := 0; i < 1000 && match == ""; i++ {
		select {
		case <-ticker.C:
			out := stream.Read()
			match = pattern.FindString(out)
			errMatch := errRE.FindString(out)
			if errMatch != "" {
				return "", errors.New(out)
			}
		case <-ctx.Done():
			ticker.Stop()
			return "", fmt.Errorf("could not match pattern in log. logs: %s", stream.Read())
		}
	}
	if match == "" {
		return "", errors.New(stream.Read())
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
