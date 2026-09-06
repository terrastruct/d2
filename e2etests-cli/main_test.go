package e2etests_cli

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/d2lang/util-go/assert"
	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"

	"github.com/d2lang/d2/d2cli"
	"github.com/d2lang/d2/internal/testutil"
	"github.com/d2lang/d2/lib/compression"
	"github.com/d2lang/d2/lib/pptx"
)

func TestCLI_E2E(t *testing.T) {
	t.Parallel()

	tca := []struct {
		name   string
		serial bool
		skip   bool
		run    func(t *testing.T, ctx context.Context, dir string, env *xos.Env)
	}{
		{
			name: "hello_world_png",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				validatePNG(t, readFile(t, dir, "hello-world.png"), 512, 868)
			},
		},
		{
			name: "hello_world_png_pad",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--pad=400", "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				validatePNG(t, readFile(t, dir, "hello-world.png"), 1712, 2068)
			},
		},
		{
			name: "png-with-local-icons",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "icon.svg", string(mustReadRasterFixture(t, "icon.svg")))
				writeFile(t, dir, "hello-world.d2", `direction: right

title: {
  label: Normal deployment
  near: bottom-center
  shape: text
  style.font-size: 40
  style.underline: true
}

local: {
  code: {
    icon: ./icon.svg
  }
}
local.code -> github.dev: commit

github: {
  icon: ./icon.svg
  dev
  master: {
    workflows
  }

  dev -> master.workflows: merge trigger
}

github.master.workflows -> aws.builders: upload and run

aws: {
  builders -> s3: upload binaries
  ec2 <- s3: pull binaries

  builders: {
    icon: ./icon.svg
  }
  s3: {
    icon: ./icon.svg
  }
  ec2: {
    icon: ./icon.svg
  }
}

local.code -> aws.ec2: {
  style.opacity: 0.0
}
`)
				err := runTestMain(t, ctx, dir, env, "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				validatePNG(t, readFile(t, dir, "hello-world.png"), 0, 0)
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
			name: "layout-extra-args",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "layout", "dagre", "--dagre-nodesep", "50", "hello-world.d2")
				assert.ErrorString(t, err, `failed to wait xmain test: e2etests-cli/d2: bad usage: layout subcommand accepts at most one argument`)
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
			name: "markdown-animation",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "animation.d2", `intro: |md
# Native **Markdown**

[documentation](https://d2lang.com)
|

steps: {
  1: {
    list: |md
      - first
      - second
    |
  }
  2: {
    table: ||md
      | Status | Count |
      | ------ | ----- |
      | Done   | 42    |
    ||
  }
}
`)
				err := runTestMain(t, ctx, dir, env, "--animate-interval=1400", "animation.d2")
				assert.Success(t, err)
				svg := readFile(t, dir, "animation.svg")
				assert.Testdata(t, ".svg", svg)
				assert.Equal(t, 3, getNumBoards(string(svg)))
				assert.Equal(t, 6, strings.Count(string(svg), `class="md md-native"`))
				assert.Equal(t, 6, strings.Count(string(svg), `href="https://d2lang.com"`))
				assert.False(t, strings.Contains(string(svg), "<foreignObject"))
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
			name: "hello_world_png_sketch",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "hello-world.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--sketch", "hello-world.d2", "hello-world.png")
				assert.Success(t, err)
				validatePNG(t, readFile(t, dir, "hello-world.png"), 512, 868)
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

				validatePDF(t, readFile(t, dir, "out.pdf"), 2)
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
			name: "how_to_solve_problems_pptx",
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
				err = testutil.ValidatePPTX(file, pptx.PPTX_TEMPLATE, 4)
				assert.Success(t, err)
			},
		},
		{
			name: "how_to_solve_problems_gif",
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
				err = testutil.ValidateGIF(gifBytes, 4, 10)
				assert.Success(t, err)
				validateGIF(t, gifBytes, 4, 4, true)
			},
		},
		{
			name: "pptx-theme-overrides",
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
				validatePPTX(t, readFile(t, dir, "all_red.pptx"), 1)
			},
		},
		{
			name: "one-layer-gif",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x`)
				err := runTestMain(t, ctx, dir, env, "--animate-interval=10", "in.d2", "out.gif")
				assert.Success(t, err)

				gifBytes := readFile(t, dir, "out.gif")
				err = testutil.ValidateGIF(gifBytes, 1, 10)
				assert.Success(t, err)
				validateGIF(t, gifBytes, 1, 1, false)
			},
		},
		{
			name:   "animated-gif",
			serial: true,
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `bank:   {
  style.fill: white
  Corporate:   {
    style.fill: white
    app14506: Data Source\ntco:      100,000\nowner: Lakshmi  {
      style:  {
        fill: '#fce7c6'
      }
    }
  }
  Equities:   {
    app14491: Risk Global\ntco:      600,000\nowner: Wendy  {
      style:  {
        fill: '#f6c889'
      }
    }
    app14492: Credit guard\ntco:      100,000\nowner: Lakshmi  {
      style:  {
        fill: '#fce7c6'
      }
    }
    app14520: Seven heaven\ntco:      100,000\nowner: Tomos  {
      style:  {
        fill: '#fce7c6'
      }
    }
    app14522: Apac Ace\ntco:      400,000\nowner: Wendy  {
      style:  {
        fill: '#f9d8a7'
      }
    }
    app14527: Risk Global\ntco:      900,000\nowner: Tomos  {
      style:  {
        fill: '#f4b76c'
      }
    }
  }
  Securities:   {
    style.fill: white
    app14517: Zone out\ntco:      500,000\nowner: Wendy  {
      style:  {
        fill: '#f6c889'
      }
    }
  }
  Finance:   {
    style.fill: white
    app14488: Credit guard\ntco:      700,000\nowner: India  {
      style:  {
        fill: '#f6c889'
      }
    }
    app14502: Ark Crypto\ntco:    1,500,000\nowner: Wendy  {
      style:  {
        fill: '#ed800c'
      }
    }
    app14510: Data Solar\ntco:    1,200,000\nowner: Deepak  {
      style:  {
        fill: '#f1a64f'
      }
    }
  }
  Risk:   {
    style.fill: white
    app14490: Seven heaven\ntco:            0\nowner: Joesph  {
      style:  {
        fill: '#fce7c6'
      }
    }
    app14507: Crypto Bot\ntco:    1,100,000\nowner: Wendy  {
      style:  {
        fill: '#f1a64f'
      }
    }
  }
  Funds:   {
    style.fill: white
    app14497: Risk Global\ntco:      500,000\nowner: Joesph  {
      style:  {
        fill: '#f6c889'
      }
    }
  }
  Fixed Income:   {
    style.fill: white
    app14523: ARC3\ntco:      600,000\nowner: Wendy  {
      style:  {
        fill: '#f6c889'
      }
    }
    app14500: Acmaze\ntco:      100,000\nowner: Tomos  {
      style:  {
        fill: '#fce7c6'
      }
    }
  }
}
bank.Risk.app14490 -> bank.Equities.app14527: client master
bank.Equities.app14491 -> bank.Equities.app14527: greeks  {
  style:  {
    stroke-dash: 5
    animated: true
    stroke: red
  }
}
bank.Funds.app14497 -> bank.Equities.app14520: allocations  {
  style:  {
    stroke-dash: 5
    animated: true
    stroke: brown
  }
}
bank.Equities.app14527 -> bank.Corporate.app14506: trades  {
  style:  {
    stroke-dash: 5
    animated: false
    stroke: blue
  }
}
bank.Fixed Income.app14523 -> bank.Equities.app14491: orders  {
  style:  {
    stroke-dash: 10
    animated: false
    stroke: green
  }
}
bank.Finance.app14488 -> bank.Equities.app14527: greeks  {
  style:  {
    stroke-dash: 5
    animated: true
    stroke: red
  }
}
bank.Equities.app14527 -> bank.Equities.app14522: orders  {
  style:  {
    stroke-dash: 10
    animated: false
    stroke: green
  }
}
bank.Equities.app14522 -> bank.Finance.app14510: orders  {
  style:  {
    stroke-dash: 10
    animated: false
    stroke: green
  }
}
bank.Equities.app14527 -> bank.Finance.app14502: greeks  {
  style:  {
    stroke-dash: 5
    animated: true
    stroke: red
  }
}
bank.Equities.app14527 -> bank.Risk.app14507: allocations  {
  style:  {
    stroke-dash: 5
    animated: true
    stroke: brown
  }
}
bank.Securities.app14517 -> bank.Equities.app14492: trades  {
  style:  {
    stroke-dash: 5
    animated: false
    stroke: blue
  }
}
bank.Equities.app14522 -> bank.Fixed Income.app14500: security reference
`)
				err := runTestMain(t, ctx, dir, env, "--timeout=300", "--scale=0.25", "--animate-interval=1000", "in.d2", "out.gif")
				assert.Success(t, err)

				validateGIF(t, readFile(t, dir, "out.gif"), 30, 100, true)
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
			name: "stdout_pdf",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x -> y`)
				stdout := &bytes.Buffer{}
				tms := testMain(dir, env, "--stdout-format=pdf", "in.d2", "-")
				tms.Stdout = stdout
				tms.Start(t, ctx)
				defer tms.Cleanup(t)
				err := tms.Wait(ctx)
				assert.Success(t, err)

				if stdout.Len() == 0 || !bytes.HasPrefix(stdout.Bytes(), []byte("%PDF-")) {
					t.Fatalf("PDF stdout is empty or missing its signature: %q", stdout.Bytes())
				}
				validatePDF(t, stdout.Bytes(), 1)
				if _, err := os.Stat(filepath.Join(dir, "-")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("PDF stdout created a literal '-' file: %v", err)
				}
			},
		},
		{
			name: "stdout_pptx",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x -> y`)
				stdout := &bytes.Buffer{}
				tms := testMain(dir, env, "--stdout-format=pptx", "in.d2", "-")
				tms.Stdout = stdout
				tms.Start(t, ctx)
				defer tms.Cleanup(t)
				err := tms.Wait(ctx)
				assert.Success(t, err)

				if stdout.Len() == 0 {
					t.Fatal("PPTX stdout is empty")
				}
				validatePPTX(t, stdout.Bytes(), 1)
				core := readPPTXMember(t, stdout.Bytes(), "docProps/core.xml")
				if !bytes.Contains(core, []byte("<dc:title>in</dc:title>")) {
					t.Fatalf("PPTX stdout title is not derived from the input: %s", core)
				}
				if _, err := os.Stat(filepath.Join(dir, "-")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("PPTX stdout created a literal '-' file: %v", err)
				}
			},
		},
		{
			name: "stdout_pdf_partial_write_error",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x -> y`)
				wantErr := errors.New("PDF stdout partial write failed")
				stdout := &failingStdout{writeLimit: 7, writeErr: wantErr}
				tms := testMain(dir, env, "--stdout-format=pdf", "in.d2", "-")
				tms.Stdout = stdout
				tms.Start(t, ctx)
				defer tms.Cleanup(t)
				err := tms.Wait(ctx)
				if !errors.Is(err, wantErr) {
					t.Fatalf("PDF stdout error = %v, want %v", err, wantErr)
				}
				if !strings.Contains(err.Error(), "failed to fully compile (partial render written)") {
					t.Fatalf("PDF stdout error does not report partial output: %v", err)
				}
				if stdout.Len() != stdout.writeLimit || stdout.writeCalls != 1 {
					t.Fatalf("PDF stdout accepted %d bytes in %d calls, want %d bytes in one call", stdout.Len(), stdout.writeCalls, stdout.writeLimit)
				}
			},
		},
		{
			name: "stdout_pptx_close_error",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x -> y`)
				wantErr := errors.New("PPTX stdout close failed")
				stdout := &failingStdout{writeLimit: -1, closeErr: wantErr}
				tms := testMain(dir, env, "--stdout-format=pptx", "in.d2", "-")
				tms.Stdout = stdout
				tms.Start(t, ctx)
				defer tms.Cleanup(t)
				err := tms.Wait(ctx)
				if !errors.Is(err, wantErr) {
					t.Fatalf("PPTX stdout error = %v, want %v", err, wantErr)
				}
				if !strings.Contains(err.Error(), "failed to fully compile (partial render written)") {
					t.Fatalf("PPTX stdout error does not report partial output: %v", err)
				}
				if stdout.Len() == 0 || stdout.writeCalls != 1 || stdout.closeCalls == 0 {
					t.Fatalf("PPTX stdout accepted %d bytes in %d writes and %d closes", stdout.Len(), stdout.writeCalls, stdout.closeCalls)
				}
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
				svg = compression.UnzipEmbeddedSVGImages(svg)
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
			name: "theme-pdf",
			run: func(t *testing.T, ctx context.Context, dir string, env *xos.Env) {
				writeFile(t, dir, "in.d2", `x -> y`)
				err := runTestMain(t, ctx, dir, env, "--theme=5", "in.d2", "out.pdf")
				assert.Success(t, err)

				validatePDF(t, readFile(t, dir, "out.pdf"), 1)
			},
		},
		{
			name: "renamed-board",
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

				validatePDF(t, readFile(t, dir, "out.pdf"), 2)
			},
		},
		{
			name: "no-nav-pdf",
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

				validatePDF(t, readFile(t, dir, "out.pdf"), 2)
			},
		},
		{
			name: "no-nav-pptx",
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

				validatePPTX(t, readFile(t, dir, "out.pptx"), 2)
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

func validatePNG(tb testing.TB, content []byte, wantWidth, wantHeight int) {
	tb.Helper()
	decoded, err := png.Decode(bytes.NewReader(content))
	assert.Success(tb, err)
	bounds := decoded.Bounds()
	if bounds.Empty() {
		tb.Fatalf("PNG has empty bounds %v", bounds)
	}
	if wantWidth != 0 && (bounds.Dx() != wantWidth || bounds.Dy() != wantHeight) {
		tb.Fatalf("PNG dimensions = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), wantWidth, wantHeight)
	}
}

func validateGIF(tb testing.TB, content []byte, wantFrames, wantDurationCentiseconds int, requireChange bool) {
	tb.Helper()
	inspection, err := testutil.InspectGIF(content)
	assert.Success(tb, err)
	if len(inspection.FrameHashes) != wantFrames || inspection.TotalDurationCentiseconds != wantDurationCentiseconds {
		tb.Fatalf("GIF frames/duration = %d/%dcs, want %d/%dcs", len(inspection.FrameHashes), inspection.TotalDurationCentiseconds, wantFrames, wantDurationCentiseconds)
	}
	if requireChange && inspection.ChangedFramePairs == 0 {
		tb.Fatal("GIF animation contains no pixel changes")
	}
}

func validatePPTX(tb testing.TB, content []byte, wantSlides int) {
	tb.Helper()
	assert.Success(tb, testutil.ValidatePPTX(content, pptx.PPTX_TEMPLATE, wantSlides))
	images, err := testutil.ExtractPPTXImages(content)
	assert.Success(tb, err)
	if len(images) != wantSlides {
		tb.Fatalf("PPTX embedded images = %d, want %d", len(images), wantSlides)
	}
	for index, encoded := range images {
		decoded, err := png.Decode(bytes.NewReader(encoded))
		assert.Success(tb, err)
		if decoded.Bounds().Empty() {
			tb.Fatalf("PPTX slide %d image has empty bounds", index+1)
		}
	}
}

func readPPTXMember(tb testing.TB, content []byte, name string) []byte {
	tb.Helper()
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	assert.Success(tb, err)
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		member, err := file.Open()
		assert.Success(tb, err)
		data, readErr := io.ReadAll(member)
		closeErr := member.Close()
		assert.Success(tb, readErr)
		assert.Success(tb, closeErr)
		return data
	}
	tb.Fatalf("PPTX member %q is missing", name)
	return nil
}

type failingStdout struct {
	bytes.Buffer
	writeLimit int
	writeErr   error
	closeErr   error
	writeCalls int
	closeCalls int
}

func (w *failingStdout) Write(p []byte) (int, error) {
	w.writeCalls++
	if w.writeLimit >= 0 && len(p) > w.writeLimit {
		p = p[:w.writeLimit]
	}
	n, _ := w.Buffer.Write(p)
	return n, w.writeErr
}

func (w *failingStdout) Close() error {
	w.closeCalls++
	return w.closeErr
}

func validatePDF(tb testing.TB, content []byte, wantPages int) {
	tb.Helper()
	inspection, err := testutil.InspectD2PDF(content)
	assert.Success(tb, err)
	if len(inspection.Pages) != wantPages {
		tb.Fatalf("PDF pages = %d, want %d", len(inspection.Pages), wantPages)
	}
	for index, page := range inspection.Pages {
		if page.Width <= 0 || page.Height <= 0 {
			tb.Fatalf("PDF page %d dimensions = %.2fx%.2f", index+1, page.Width, page.Height)
		}
	}
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
