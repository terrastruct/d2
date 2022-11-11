package e2etests

import (
	_ "embed"
	"testing"
)

// based on https://github.com/mxstbr/markdown-test-file
//go:embed markdowntest.md
var testMarkdown string

func testStable(t *testing.T) {
	tcs := []testCase{
		{
			name: "connected_container",
			script: `a.b -> c.d -> f.h.g
`,
		},
		{
			name: "circular_dependency",
			script: `a -> b -> c -> b -> a
`,
		},
		{
			name: "all_shapes",
			script: `
rectangle: {shape: "rectangle"}
square: {shape: "square"}
page: {shape: "page"}
parallelogram: {shape: "parallelogram"}
document: {shape: "document"}
cylinder: {shape: "cylinder"}
queue: {shape: "queue"}
package: {shape: "package"}
step: {shape: "step"}
callout: {shape: "callout"}
stored_data: {shape: "stored_data"}
person: {shape: "person"}
diamond: {shape: "diamond"}
oval: {shape: "oval"}
circle: {shape: "circle"}
hexagon: {shape: "hexagon"}
cloud: {shape: "cloud"}

rectangle -> square -> page
parallelogram -> document -> cylinder
queue -> package -> step
callout -> stored_data -> person
diamond -> oval -> circle
hexagon -> cloud
`,
		},
		{
			name: "all_shapes_multiple",
			script: `
rectangle: {shape: "rectangle"}
square: {shape: "square"}
page: {shape: "page"}
parallelogram: {shape: "parallelogram"}
document: {shape: "document"}
cylinder: {shape: "cylinder"}
queue: {shape: "queue"}
package: {shape: "package"}
step: {shape: "step"}
callout: {shape: "callout"}
stored_data: {shape: "stored_data"}
person: {shape: "person"}
diamond: {shape: "diamond"}
oval: {shape: "oval"}
circle: {shape: "circle"}
hexagon: {shape: "hexagon"}
cloud: {shape: "cloud"}

rectangle -> square -> page
parallelogram -> document -> cylinder
queue -> package -> step
callout -> stored_data -> person
diamond -> oval -> circle
hexagon -> cloud

rectangle.multiple: true
square.multiple: true
page.multiple: true
parallelogram.multiple: true
document.multiple: true
cylinder.multiple: true
queue.multiple: true
package.multiple: true
step.multiple: true
callout.multiple: true
stored_data.multiple: true
person.multiple: true
diamond.multiple: true
oval.multiple: true
circle.multiple: true
hexagon.multiple: true
cloud.multiple: true
`,
		},
		{
			name: "all_shapes_shadow",
			script: `
rectangle: {shape: "rectangle"}
square: {shape: "square"}
page: {shape: "page"}
parallelogram: {shape: "parallelogram"}
document: {shape: "document"}
cylinder: {shape: "cylinder"}
queue: {shape: "queue"}
package: {shape: "package"}
step: {shape: "step"}
callout: {shape: "callout"}
stored_data: {shape: "stored_data"}
person: {shape: "person"}
diamond: {shape: "diamond"}
oval: {shape: "oval"}
circle: {shape: "circle"}
hexagon: {shape: "hexagon"}
cloud: {shape: "cloud"}

rectangle -> square -> page
parallelogram -> document -> cylinder
queue -> package -> step
callout -> stored_data -> person
diamond -> oval -> circle
hexagon -> cloud

rectangle.shadow: true
square.shadow: true
page.shadow: true
parallelogram.shadow: true
document.shadow: true
cylinder.shadow: true
queue.shadow: true
package.shadow: true
step.shadow: true
callout.shadow: true
stored_data.shadow: true
person.shadow: true
diamond.shadow: true
oval.shadow: true
circle.shadow: true
hexagon.shadow: true
cloud.shadow: true
`,
		},
		{
			name: "square_3d",
			script: `
rectangle: {shape: "rectangle"}
square: {shape: "square"}

rectangle -> square

rectangle.3d: true
square.3d: true
`,
		},
		{
			name: "container_edges",
			script: `a -> g.b -> d.h.c
d -> g.e -> f -> g -> d.h
`,
		},
		{
			name: "one_three_one_container",
			script: `top.start -> a
top.start -> b
top.start -> c
a -> bottom.end
b -> bottom.end
c -> bottom.end
`,
		},
		{
			name: "straight_hierarchy_container",
			script: `a
c
b

l1: {
	b
	a
	c
}

b -> l1.b
a -> l1.a
c -> l1.c

l2c1: {
	a
}
l1.a -> l2c1.a

l2c3: {
	c
}
l1.c -> l2c3.c

l2c2: {
	b
}
l1.b -> l2c2.b

l3c1: {
	a
	b
}
l2c1.a -> l3c1.a
l2c2.b -> l3c1.b

l3c2: {
	c
}
l2c3.c -> l3c2.c

l4: {
	c1: {
		a
	}
	c2: {
		b
	}
	c3: {
		c
	}
}
l3c1.a -> l4.c1.a
l3c1.b -> l4.c2.b
l3c2.c -> l4.c3.c`,
		},
		{
			name: "different_subgraphs",
			script: `a -> tree
a -> and
a -> nodes
and -> some
tree -> more
tree -> many

then -> here
here -> you
have -> hierarchy
then -> hierarchy

finally -> another
another -> of
nesting -> trees
finally -> trees
finally: {
	a -> tree
	inside -> a
	tree -> hierarchy
	a -> root
}`,
		},
		{
			name: "binary_tree",
			script: `a -> b
a -> c
b -> d
b -> e
c -> f
c -> g
d -> h
d -> i
e -> j
e -> k
f -> l
f -> m
g -> n
g -> o`,
		},
		{
			name: "dense",
			script: `
a-> b
c -> b
d-> e
f-> e
b-> f
b-> g
g-> f
b-> h
b-> i
b-> d
j-> c
j-> a
b-> j
i-> k
d-> l
l-> e
m-> l
m-> n
n-> i
d-> n
f-> n
b-> o
p-> l
e-> q`,
		},
		{
			name: "multiple_trees",
			script: `
a-> b
a-> c
a-> d
a-> e
a-> f
g-> a
a-> h
i-> b
j-> b
k-> g
l-> g
c-> m
c-> n
d-> o
d-> p
e-> q
e-> r
p-> s
f-> t
f-> u
v-> h
w-> h
`,
		},
		{
			name: "one_container_loop",
			script: `
a.b-> c
d-> c
e-> c
f-> d
a-> e
g-> f
a.h-> g
`,
		},
		{
			name: "large_arch",
			script: `
a
b
c
d
e
f
g
h
i
i.j
i.j.k
i.j.l
i.m
i.n
i.o
i.o.p
q
r
r.s
r.s.t
r.s.u.v
r.s.w
r.s.x
r.s.y
r.z
r.aa
r.bb
r.bb.cc
r.bb.dd
r.ee
r.ff
r.gg

i.j.k-> i.m
i.j.l-> i.o.p
q-> i.m
i.m-> q
i.n-> q
i.m-> c
i.m-> d
i.m-> g
i.m-> f
d-> e
r.s.x-> r.s.t
r.s.x-> r.s.w
r.gg-> r.s.t
r.s.u.v-> r.z
r.aa-> r.s.t
r.s.w-> i.m
r.s.t-> g
r.s.t-> h
r.ee -> r.ff
`,
		},
		{
			name: "n22_e32",
			script: `
a-> b
c-> a
d-> a
d-> b
d-> e
e-> f
f-> b
c-> f
g-> c
g-> h
h-> i
i-> j
j-> k
k-> e
j-> f
l-> m
n-> l
n-> l
n-> m
n-> o
o-> p
p-> m
n-> p
q-> n
q-> r
r-> s
s-> t
t-> u
u-> o
t-> p
c-> t
s-> a
u-> a
`,
		},
		{
			name: "chaos1",
			script: `
aaa: {
	bbb.shape: callout
}
aaa.ccc -- aaa
(aaa.ccc -- aaa)[0]: '111'
ddd.shape: cylinder
eee.shape: document
eee <- aaa.ccc
(eee <- aaa.ccc)[0]: '222'
`,
		},
		{
			name: "chaos2",
			script: `
aa: {
	bb: {
		cc:  {
			dd: {
				shape: rectangle
				ee: {shape: text}
				ff
			}
			gg: {shape: text}
			hh
			dd.ee -- gg: '11'
			gg -- hh: '22'
		}
		ii: {
			shape: package
			jj: {shape: diamond}
		}
		ii -> cc.dd
		kk: {shape: circle}
	}
	ll
	mm: {shape: cylinder}
	ll <-> bb: '33'
	mm -> bb.cc: '44'
	mm->ll
	mm <-> bb: '55'
	ll <-> bb.cc.gg
	mm <- bb.ii: '66'
	bb.cc <- ll: '77'
	nn: {shape: text}
	oo
	bb.ii <-> ll: '88'
}
			`,
		},
		{
			name: "us_map",
			script: `
AL -- FL -- GA -- MS -- TN
AK
AZ -- CA -- NV -- NM -- UT
AR -- LA -- MS -- MO -- OK -- TN -- TX
CA -- NV -- OR
CO -- KS -- NE -- NM -- OK -- UT -- WY
CT -- MA -- NY -- RI
DE -- MD -- NJ -- PA
FL -- GA
GA -- NC -- SC -- TN
HI
ID -- MT -- NV -- OR -- UT -- WA -- WY
IL -- IN -- IA -- MI -- KY -- MO -- WI
IN -- KY -- MI -- OH
IA -- MN -- MO -- NE -- SD -- WI
KS -- MO -- NE -- OK
KY -- MO -- OH -- TN -- VA -- WV
LA -- MS -- TX
ME -- NH
MD -- PA -- VA -- WV
MA -- NH -- NY -- RI -- VT
MI -- MN -- OH -- WI
MN -- ND -- SD -- WI
MS -- TN
MO -- NE -- OK -- TN
MT -- ND -- SD -- WY
NE -- SD -- WY
NV -- OR -- UT
NH -- VT
NJ -- NY -- PA
NM -- OK -- TX
NY -- PA -- RI -- VT
NC -- SC -- TN -- VA
ND -- SD
OH -- PA -- WV
OK -- TX
OR -- WA
PA -- WV
SD -- WY
TN -- VA
UT -- WY
VA -- WV
`,
		},
		{
			name: "investigate",
			script: `
aa.shape: step
bb.shape: step
cc.shape: step
aa -- bb -- cc

aa -> dd.ee: 1
bb -> ff.gg: 2
cc -> dd.hh: 3

dd.ee.shape: diamond
dd.ee -> ii

ii -- jj -> kk

ll.mm.shape: circle
ff.mm.shape: circle
kk -> ff.mm: 4
ff.mm -> ll.mm: 5
ll.mm -> nn.oo: 6

ff.gg.shape: diamond
ff.gg -> ff.pp -> ll.qq -> ll.rr

dd.hh.shape: diamond
dd.hh -> ss.tt -> uu.vv

kk -> ww
uu.vv -> ww
ww -> rm

ww: {
	shape: queue
	icon: https://icons.terrastruct.com/essentials/time.svg
}

rm -> nn.xx
ll.rr -> yy.zz

rm -> yy.zz
yy.zz.shape: queue
yy.zz.icon: https://icons.terrastruct.com/essentials/time.svg

yy.zz -> yy.ab -> nn.ac -> ad

ad.style.fill: red
ad.shape: parallelogram

nn.shape: cylinder

ww -> ff.gg
`,
		},
		{
			name:   "multiline_text",
			script: `hey: this\ngoes\nmultiple lines`,
		},
		{
			name: "markdown",
			script: `hey: |md
# Every frustum longs to be a cone

- A continuing flow of paper is sufficient to continue the flow of paper
- Please remain calm, it's no use both of us being hysterical at the same time
- Visits always give pleasure: if not on arrival, then on the departure

*Festivity Level 1*: Your guests are chatting amiably with each other.
|

x -> hey -> y
`,
		},
		{
			name: "child_parent_edges",
			script: `a.b -> a
a.b -> a.b.c
a.b.c.d -> a.b`,
		},
		{
			name: "lone_h1",
			script: mdTestScript(`
# Markdown: Syntax
`),
		},
		// newlines should be ignored here in md text measurement
		{
			name: "p",
			script: mdTestScript(`
A paragraph is simply one or more consecutive lines of text, separated
by one or more blank lines. (A blank line is any line that looks like a
blank line -- a line containing nothing but spaces or tabs is considered
blank.) Normal paragraphs should not be indented with spaces or tabs.
`),
		},
		{
			name: "li1",
			script: mdTestScript(`
- [Overview](#overview)
  - [Philosophy](#philosophy)
  - [Inline HTML](#html)
    - [Automatic Escaping for Special Characters](#autoescape)
`),
		},
		{
			name: "li2",
			script: mdTestScript(`
- [Overview](#overview) ok _this is all measured_
	- [Philosophy](#philosophy)
	- [Inline HTML](#html)
`),
		},
		{
			name: "li3",
			script: mdTestScript(`
- [Overview](#overview)
  - [Philosophy](#philosophy)
  - [Inline HTML](#html)
  - [Automatic Escaping for Special Characters](#autoescape)
- [Block Elements](#block)
  - [Paragraphs and Line Breaks](#p)
  - [Headers](#header)
  - [Blockquotes](#blockquote)
  - [Lists](#list)
  - [Code Blocks](#precode)
  - [Horizontal Rules](#hr)
- [Span Elements](#span)
  - [Links](#link)
  - [Emphasis](#em)
  - [Code](#code)
  - [Images](#img)
- [Miscellaneous](#misc)
  - [Backslash Escapes](#backslash)
  - [Automatic Links](#autolink)
`),
		},
		{
			name: "li4",
			script: mdTestScript(`
List items may consist of multiple paragraphs. Each subsequent
paragraph in a list item must be indented by either 4 spaces
or one tab:

1.  This is a list item with two paragraphs. Lorem ipsum dolor
    sit amet, consectetuer adipiscing elit. Aliquam hendrerit
    mi posuere lectus.

    Vestibulum enim wisi, viverra nec, fringilla in, laoreet
    vitae, risus. Donec sit amet nisl. Aliquam semper ipsum
    sit amet velit.

2.  Suspendisse id sem consectetuer libero luctus adipiscing.

It looks nice if you indent every line of the subsequent
paragraphs, but here again, Markdown will allow you to be
lazy:

- This is a list item with two paragraphs.

      This is the second paragraph in the list item. You're

  only required to indent the first line. Lorem ipsum dolor
  sit amet, consectetuer adipiscing elit.

- Another item in the same list.
`),
		},
		{
			name: "hr",
			script: mdTestScript(`
**Note:** This document is itself written using Markdown; you
can [see the source for it by adding '.text' to the URL](/projects/markdown/syntax.text).

---

## Overview
`),
		},
		{
			name: "pre",
			script: mdTestScript(`
Here is an example of AppleScript:

    tell application "Foo"
        beep
    end tell

A code block continues until it reaches a line that is not indented
(or the end of the article).
`),
		},
		{
			name:   "giant_markdown_test",
			script: mdTestScript(testMarkdown),
		},
		{
			name: "code_snippet",
			script: `hey: |go
// RegisterHash registers a function that returns a new instance of the given
// hash function. This is intended to be called from the init function in
// packages that implement hash functions.
func RegisterHash(h Hash, f func() hash.Hash) {
	if h >= maxHash {
		panic("crypto: RegisterHash of unknown hash function")
	}
	hashes[h] = f
}
|
x -> hey -> y`,
		}, {
			name: "arrowhead_adjustment",
			script: `a <-> b: {
				style.stroke-width: 6
				style.stroke-dash: 4
				source-arrowhead: {
				  shape: arrow
				}
			  }

			  c -> b: {
				style.stroke-width: 7
				style.stroke: "#20222a"
			  }
			  c.style.stroke-width: 7
			  c.style.stroke: "#b2350d"
			  c.shape: document
			  b.style.stroke-width: 8
			  b.style.stroke: "#0db254"
			  a.style.border-radius: 10
			  a.style.stroke-width: 8
			  a.style.stroke: "#2bc3d8"
			  Oval: "" {
				shape: oval
				style.stroke-width: 6
				style.stroke: "#a1a4af"
			  }
			  a <-> Oval: {
				style.stroke-width: 6
				source-arrowhead: {
				  shape: diamond
				}
				target-arrowhead: * {
				  shape: diamond
				  style.filled: true
				}
			  }
			  c -- a: {style.stroke-width: 7}
			  Oval <-> c`,
		},
		{
			name: "md_code_inline",
			script: `md: |md
` + "`code`" + `
|
a -> md -> b
`,
		},
		{
			name: "md_code_block_fenced",
			script: `md: |md
` + "```" + `
{
	fenced: "block",
	of: "json",
}
` + "```" + `
|
a -> md -> b
`,
		},
		{
			name: "md_code_block_indented",
			script: `md: |md
a line of text and an

	{
		indented: "block",
		of: "json",
	}

|
a -> md -> b
`,
		},
	}

	runa(t, tcs)
}
