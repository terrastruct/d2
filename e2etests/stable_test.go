package e2etests

import (
	_ "embed"
	"testing"
)

// based on https://github.com/mxstbr/markdown-test-file
//
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

rectangle.style.multiple: true
square.style.multiple: true
page.style.multiple: true
parallelogram.style.multiple: true
document.style.multiple: true
cylinder.style.multiple: true
queue.style.multiple: true
package.style.multiple: true
step.style.multiple: true
callout.style.multiple: true
stored_data.style.multiple: true
person.style.multiple: true
diamond.style.multiple: true
oval.style.multiple: true
circle.style.multiple: true
hexagon.style.multiple: true
cloud.style.multiple: true
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

rectangle.style.shadow: true
square.style.shadow: true
page.style.shadow: true
parallelogram.style.shadow: true
document.style.shadow: true
cylinder.style.shadow: true
queue.style.shadow: true
package.style.shadow: true
step.style.shadow: true
callout.style.shadow: true
stored_data.style.shadow: true
person.style.shadow: true
diamond.style.shadow: true
oval.style.shadow: true
circle.style.shadow: true
hexagon.style.shadow: true
cloud.style.shadow: true
`,
		},
		{
			name: "square_3d",
			script: `
rectangle: {shape: "rectangle"}
square: {shape: "square"}

rectangle -> square

rectangle.style.3d: true
square.style.3d: true
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
			name: "font_sizes_containers_large",
			script: `
ninety nine: {
	style.font-size: 99
	sixty four: {
		style.font-size: 64
		thirty two:{
			style.font-size: 32
			sixteen: {
				style.font-size: 16
				eight: {
					style.font-size: 8
				}
			}
		}
	}
}
`,
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
		{
			name: "class",
			script: `manager: BatchManager {
  shape: class
  -num: int
  -timeout: int
  -pid

  +getStatus(): Enum
  +getJobs(): "Job[]"
  +setTimeout(seconds int)
}
`,
		}, {
			name: "sql_tables",
			script: `users: {
	shape: sql_table
	id: int
	name: string
	email: string
	password: string
	last_login: datetime { constraint: primary_key }
}

products: {
	shape: sql_table
	id: int
	price: decimal
	sku: string
	name: string
}

orders: {
	shape: sql_table
	id: int
	user_id: int
	product_id: int
}

shipments: {
	shape: sql_table
	id: int
	order_id: int
	tracking_number: string
	status: string
}

users.id <-> orders.user_id
products.id <-> orders.product_id
shipments.order_id <-> orders.id`,
		}, {
			name: "images",
			script: `a: {
  shape: image
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
}

b: {
  shape: image
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
}
a -> b
`,
		},
		{
			name: "arrowhead_labels",
			script: `
a -> b: To err is human, to moo bovine {
	source-arrowhead: 1
	target-arrowhead: * {
		shape: diamond
	}
}
`,
		},
		{
			name: "stylish",
			script: `
x: {
  style: {
    opacity: 0.6
    fill: orange
    stroke: "#53C0D8"
    stroke-width: 5
    shadow: true
  }
}

y: {
  style: {
    stroke-dash: 5
    opacity: 0.6
    fill: red
    3d: true
    stroke: black
  }
}

x -> y: in style {
  style: {
    stroke: green
    opacity: 0.5
    stroke-width: 2
    stroke-dash: 5
	fill: lavender
  }
}
`,
		},
		{
			name: "md_2space_newline",
			script: `
markdown: {
  md: |md
Lorem ipsum dolor sit amet, consectetur adipiscing elit,  ` + `
sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
|
}
`,
		},
		{
			name: "md_backslash_newline",
			script: `
markdown: {
  md: |md
Lorem ipsum dolor sit amet, consectetur adipiscing elit,\
sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
|
}
`,
		},
		{
			name: "font_colors",
			script: `
alpha: {
	style.font-color: '#4A6FF3'
}
beta: {
	style.font-color: red
}
alpha -> beta: gamma {
	style.font-color: green
}
`,
		},
		{
			name: "latex",
			script: `a: |latex
\\Huge{\\frac{\\alpha g^2}{\\omega^5} e^{[ -0.74\\bigl\\{\\frac{\\omega U_\\omega 19.5}{g}\\bigr\\}^{\\!-4}\\,]}}
|

b: |latex
e = mc^2
|

z: |latex
gibberish\\; math:\\sum_{i=0}^\\infty i^2
|

z -> a
z -> b

a -> c
b -> c
sugar -> c
c: mixed together

c -> solution: we get
`,
		},
		{
			name: "direction",
			script: `a -> b -> c -> d -> e
b: {
  direction: right
  1 -> 2 -> 3 -> 4 -> 5

  2: {
    direction: up
    a -> b -> c -> d -> e
  }
}
`,
		},
		{
			name: "transparent_3d",
			script: `
cube: {
	style: {
		3d: true
		opacity: 0.5
		fill: orange
		stroke: "#53C0D8"
		stroke-width: 7
	}
}
`,
		},
		{
			name: "font_sizes",
			script: `
size XS.style.font-size: 13
size S.style.font-size: 14
size M.style.font-size: 16
size L.style.font-size: 20
size XL.style.font-size: 24
size XXL.style.font-size: 28
size XXXL.style.font-size: 32

custom 8.style.font-size: 8
custom 12.style.font-size: 12
custom 18.style.font-size: 18
custom 21.style.font-size: 21
custom 64.style.font-size: 64

custom 8 -> size XS: custom 10 {
	style.font-size: 10
}
size S -> size M: custom 15 {
	style.font-size: 15
}
size XXXL -> custom 64: custom 48 {
	style.font-size: 48
	style.fill: lavender
}
`,
		}, {
			name: "sequence_diagram_simple",
			script: `shape: sequence_diagram
alice: "Alice\nline\nbreaker" {
    shape: person
    style.stroke: red
}
bob: "Bob" {
    shape: person
    style.stroke-width: 5
}
db: {
    shape: cylinder
}
queue: {
    shape: queue
}
service: "an\nodd\nservice\nwith\na\nname\nin\nmultiple lines"

alice -> bob: "Authentication Request"
bob -> service: "make request for something that is quite far away and requires a really long label to take all the space between the objects"
service -> db: "validate credentials"
db -> service: {
    style.stroke-dash: 4
}
service -> bob: {
    style.stroke-dash: 4
}
bob -> alice: "Authentication Response"
alice -> bob: "Another authentication Request"
bob -> queue: "do it later"
queue -> bob: "stored" {
    style.stroke-dash: 3
    style.stroke-width: 5
    style.stroke: green
}

bob -> alice: "Another authentication Response"`,
		}, {
			name: "sequence_diagram_span",
			script: `shape: sequence_diagram

scorer.t -> itemResponse.t: getItem()
scorer.t <- itemResponse.t: item

scorer.t -> item.t1: getRubric()
scorer.t <- item.t1: rubric

scorer.t -> essayRubric.t: applyTo(essayResp)
itemResponse -> essayRubric.t.c
essayRubric.t.c -> concept.t: match(essayResponse)
scorer <- essayRubric.t: score

scorer.t -> itemOutcome.t1: new
scorer.t -> item.t2: getNormalMinimum()
scorer.t -> item.t3: getNormalMaximum()

scorer.t -> itemOutcome.t2: setScore(score)
scorer.t -> itemOutcome.t3: setFeedback(missingConcepts)`,
		}, {
			name: "sequence_diagram_nested_span",
			script: `shape: sequence_diagram

scorer: {
    style.stroke: red
    style.stroke-width: 5
}

scorer.abc: {
    style.fill: yellow
    style.stroke-width: 7
}

scorer -> itemResponse.a: {
    style.stroke-width: 10
}
itemResponse.a -> item.a.b
item.a.b -> essayRubric.a.b.c
essayRubric.a.b.c -> concept.a.b.c.d
item.a -> essayRubric.a.b
concept.a.b.c.d -> itemOutcome.a.b.c.d.e

scorer.abc -> item.a

itemOutcome.a.b.c.d.e -> scorer
scorer -> itemResponse.c`,
		}, {
			name: "sequence_diagrams",
			script: `a_shape.shape: circle
a_sequence: {
    shape: sequence_diagram

    scorer.t -> itemResponse.t: getItem()
    scorer.t <- itemResponse.t: item

    scorer.t -> item.t1: getRubric()
    scorer.t <- item.t1: rubric

    scorer.t -> essayRubric.t: applyTo(essayResp)
    itemResponse -> essayRubric.t.c
    essayRubric.t.c -> concept.t: match(essayResponse)
    scorer <- essayRubric.t: score

    scorer.t <-> itemOutcome.t1: new
    scorer.t <-> item.t2: getNormalMinimum()
    scorer.t -> item.t3: getNormalMaximum()

    scorer.t -- itemOutcome.t2: setScore(score)
    scorer.t -- itemOutcome.t3: setFeedback(missingConcepts)
}

another: {
    sequence: {
        shape: sequence_diagram

		# scoped edges
        scorer.t -> itemResponse.t: getItem()
        scorer.t <- itemResponse.t: item

        scorer.t -> item.t1: getRubric()
        scorer.t <- item.t1: rubric

        scorer.t -> essayRubric.t: applyTo(essayResp)
        itemResponse -> essayRubric.t.c
        essayRubric.t.c -> concept.t: match(essayResponse)
        scorer <- essayRubric.t: score

        scorer.t -> itemOutcome.t1: new
        scorer.t <-> item.t2: getNormalMinimum()
        scorer.t -> item.t3: getNormalMaximum()

        scorer.t -> itemOutcome.t2: setScore(score)
        scorer.t -> itemOutcome.t3: setFeedback(missingConcepts)
    }
}

a_shape -> a_sequence
a_shape -> another.sequence
a_sequence -> sequence
another.sequence <-> finally.sequence
a_shape -- finally


finally: {
    shape: queue
    sequence: {
        shape: sequence_diagram
		# items appear in this order
        scorer
        concept
        essayRubric
        item
        itemOutcome
        itemResponse
    }
}

# full path edges
finally.sequence.itemResponse.a -> finally.sequence.item.a.b
finally.sequence.item.a.b -> finally.sequence.essayRubric.a.b.c
finally.sequence.essayRubric.a.b.c -> finally.sequence.concept.a.b.c.d
finally.sequence.item.a -> finally.sequence.essayRubric.a.b
finally.sequence.concept.a.b.c.d -> finally.sequence.itemOutcome.a.b.c.d.e
finally.sequence.scorer.abc -> finally.sequence.item.a
finally.sequence.itemOutcome.a.b.c.d.e -> finally.sequence.scorer
finally.sequence.scorer -> finally.sequence.itemResponse.c`,
		},
		{
			name: "number_connections",
			script: `1 -> 2
foo baz: Foo Baz

foo baz -> hello
`,
		}, {
			name: "sequence_diagram_all_shapes",
			script: `shape: sequence_diagram

a: "a label" {
    shape: callout
}
b: "b\nlabels" {
    shape: circle
}
c: "a class" {
    shape: class
    +public() bool
    -private() int
}
d: "cloudyyyy" {
    shape: cloud
}
e: |go
    a := 5
    b := a + 7
    fmt.Printf("%d", b)
|
f: "cyl" {
    shape: cylinder
}
g: "dia" {
    shape: diamond
}
h: "docs" {
    shape: document
}
i: "six corners" {
    shape: hexagon
}
j: "a random icon" {
    shape: image
    icon: https://icons.terrastruct.com/essentials/004-picture.svg
}
k: "over" {
    shape: oval
}
l: "pack" {
    shape: package
}
m: "docs page" {
    shape: page
}
n: "too\nhard\to say" {
    shape: parallelogram
}
o: "single\nperson" {
    shape: person
}
p: "a queue" {
    shape: queue
}
q: "a square" {
    shape: square
}
r: "a step at a time" {
    shape: step
}
s: "data" {
    shape: stored_data
}

t: "users" {
    shape: sql_table
    id: int
    name: varchar
}

a -> b: |go
    result := callThisFunction(obj, 5)
|
b <-> c: "mid" {
    source-arrowhead: "this side" {
        shape: diamond
    }
    target-arrowhead: "other side" {
        shape: triangle
    }
}
c -> d
d -> e
e -> f
f -> g
g -> h
h -> i
i -> j
j -> k
k -> l
l -> m
m -> n
n -> o
o -> p
p -> q
q -> r
r -> s
s -> t`,
		},
		{
			name: "self-referencing",
			script: `x -> x -> x -> y
z -> y
z -> z: hello
`,
		}, {
			name: "sequence_diagram_self_edges",
			script: `shape: sequence_diagram
a -> a: a self edge here
a -> b: between actors
b -> b.1: to descendant
b.1 -> b.1.2: to deeper descendant
b.1.2 -> b: to parent
b -> a.1.2: actor
a.1 -> b.3`,
		},
		{
			name: "icon-label",
			script: `ww: {
  label: hello
  icon: https://icons.terrastruct.com/essentials/time.svg
}
`,
		},
		{
			name: "sequence_diagram_note",
			script: `shape: sequence_diagram
a; b; c; d
a -> b
a.explanation
a.another explanation
b -> c
b."Some one who believes imaginary things\n appear right before your i's."
c -> b: okay
d."The earth is like a tiny grain of sand, only much, much heavier"
`,
		},
		{
			name: "sequence_diagram_groups",
			script: `shape: sequence_diagram
a;b;c;d
a -> b
ggg: {
	a -> b: lala
}
group 1: {
  b -> c
	c -> b: ey
  nested guy: {
    c -> b: okay
  }
  b.t1 -> c.t1
  b.t1.t2 -> c.t1
  c.t1 -> b.t1
}
group b: {
  b -> c
	c."what would arnold say"
  c -> b: okay
}
choo: {
  d."this note"
}
`,
		},
		{
			name: "sequence_diagram_nested_groups",
			script: `shape: sequence_diagram

a; b; c

this is a message group: {
    a -> b
    and this is a nested message group: {
        a -> b
        what about more nesting: {
            a -> b
						crazy town: {
								a."a note"
								a -> b
							whoa: {
									a -> b
							}
            }
        }
    }
}

alt: {
    case 1: {
        b -> c
    }
    case 2: {
        b -> c
    }
    case 3: {
        b -> c
    }
    case 4: {
        b -> c
    }
}

b.note: "a note here to remember that padding must consider notes too"
a.note: "just\na\nlong\nnote\nhere"
c: "just an actor"
`,
		},
		{
			name: "sequence_diagram_real",
			script: `How this is rendered: {
  shape: sequence_diagram

	CLI; d2ast; d2compiler; d2layout; d2exporter; d2themes; d2renderer; d2sequencelayout; d2dagrelayout

  CLI -> d2ast: "'How this is rendered: {...}'"
  d2ast -> CLI: tokenized AST
  CLI -> d2compiler: compile AST
  d2compiler."measurements also take place"
  d2compiler -> CLI: objects and edges
  CLI -> d2layout.layout: run layout engines
  d2layout.layout -> d2sequencelayout: run engine on shape: sequence_diagram, temporarily remove
  only if root is not sequence: {
    d2layout.layout -> d2dagrelayout: run core engine on rest
  }
  d2layout.layout <- d2sequencelayout: add back in sequence diagrams
  d2layout -> CLI: diagram with correct positions and dimensions
  CLI -> d2exporter: export diagram with chosen theme and renderer
  d2exporter.export -> d2themes: get theme styles
  d2exporter.export -> d2renderer: render to SVG
  d2exporter.export -> CLI: resulting SVG
}
`,
		},
		{
			name: "sequence_diagram_actor_distance",
			script: `shape: sequence_diagram
a: "an actor with a really long label that will break everything"
c: "an\nactor\nwith\na\nreally\nlong\nlabel\nthat\nwill\nbreak\neverything"
d: "simple"
e: "a short one"
b: "far away"
f: "what if there were no labels between this actor and the previous one"
a -> b: "short"
a -> b: "long label for testing purposes and it must be really, really long"
c -> d: "short"
a -> d: "this should span many actors lifelines so we know how it will look like when redering a long label over many actors"
d -> e: "long label for testing purposes and it must be really, really long"
a -> f`,
		}, {
			name: "sequence_diagram_long_note",
			script: `shape: sequence_diagram
a -> b
b.note: "a note here to remember that padding must consider notes too"
a.note: "just\na\nlong\nnote\nhere"`,
		},
		{
			name: "sequence_diagram_distance",
			script: `shape: sequence_diagram
alice -> bob: what does it mean to be well-adjusted
bob -> alice: The ability to play bridge or golf as if they were games
`,
		},
		{
			name: "markdown_stroke_fill",
			script: `
container.md: |md
# a header

a line of text and an

	{
		indented: "block",
		of: "json",
	}

walk into a bar.
| {
	style.stroke: darkorange
}

container -> no container

no container: |md
they did it in style
|

no container.style: {
	stroke: red
	fill: "#CEEDEE"
}
`,
		},
		{
			name: "overlapping_image_container_labels",
			script: `
root: {
	shape: image
	icon: https://icons.terrastruct.com/essentials/004-picture.svg
}

root -> container.root

container: {
	root: {
		shape: image
		icon: https://icons.terrastruct.com/essentials/004-picture.svg
	}

	left: {
		root: {
			shape: image
			icon: https://icons.terrastruct.com/essentials/004-picture.svg
		}
		inner: {
			left: {
				shape: image
				icon: https://icons.terrastruct.com/essentials/004-picture.svg
			}
			right: {
				shape: image
				icon: https://icons.terrastruct.com/essentials/004-picture.svg
			}
		}
		root -> inner.left: {
			label: to inner left
		}
		root -> inner.right: {
			label: to inner right
		}
	}

	right: {
		root: {
			shape: image
			icon: https://icons.terrastruct.com/essentials/004-picture.svg
		}
		inner: {
			left: {
				shape: image
				icon: https://icons.terrastruct.com/essentials/004-picture.svg
			}
			right: {
				shape: image
				icon: https://icons.terrastruct.com/essentials/004-picture.svg
			}
		}
		root -> inner.left: {
			label: to inner left
		}
		root -> inner.right: {
			label: to inner right
		}
	}

	root -> left.root: {
		label: to left container root
	}

	root -> right.root: {
		label: to right container root
	}
}
`,
		},
		{
			name: "constant_near_stress",
			script: `x -> y
The top of the mountain: { shape: text; near: top-center }
Joe: { shape: person; near: center-left }
Donald: { shape: person; near: center-right }
bottom: |md
	# Cats, no less liquid than their shadows, offer no angles to the wind.

  If we can't fix it, it ain't broke.

  Dieters live life in the fasting lane.
| { near: bottom-center }
i am top left: { shape: text; near: top-left }
i am top right: { shape: text; near: top-right }
i am bottom left: { shape: text; near: bottom-left }
i am bottom right: { shape: text; near: bottom-right }
`,
		},
		{
			name: "constant_near_title",
			script: `title: |md
  # A winning strategy
| { near: top-center }

poll the people -> results
results -> unfavorable -> poll the people
results -> favorable -> will of the people
`,
		},
		{
			name: "text_font_sizes",
			script: `bear: { shape: text; style.font-size: 22; style.bold: true }
mama bear: { shape: text; style.font-size: 28; style.italic: true }
papa bear: { shape: text; style.font-size: 32; style.underline: true }
mama bear -> bear
papa bear -> bear
`,
		},
		{
			name: "tooltips",
			script: `x: { tooltip: Total abstinence is easier than perfect moderation }
y: { tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
`,
		},
		{
			name: "links",
			script: `x: { link: https://d2lang.com }
			y: { link: https://terrastruct.com; tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
`,
		},
		{
			name: "unnamed_only_width",
			script: `

class -> users -> code -> package -> no width

class: "" {
	shape: class
	-num: int
	-timeout: int
	-pid

	+getStatus(): Enum
	+getJobs(): "Job[]"
	+setTimeout(seconds int)
}

users: "" {
	shape: sql_table
	id: int
	name: string
	email: string
	password: string
	last_login: datetime
}

code: |go
    a := 5
    b := a + 7
    fmt.Printf("%d", b)
|

package: "" { shape: package }
no width: ""


class.width: 512
users.width: 512
code.width: 512
package.width: 512
`,
		},
		{
			name: "unnamed_only_height",
			script: `

class -> users -> code -> package -> no height

class: "" {
	shape: class
	-num: int
	-timeout: int
	-pid

	+getStatus(): Enum
	+getJobs(): "Job[]"
	+setTimeout(seconds int)
}

users: "" {
	shape: sql_table
	id: int
	name: string
	email: string
	password: string
	last_login: datetime
}

code: |go
    a := 5
    b := a + 7
    fmt.Printf("%d", b)
|

package: "" { shape: package }
no height: ""


class.height: 512
users.height: 512
code.height: 512
package.height: 512
`,
		},
		{
			name: "crow_foot_arrowhead",
			script: `
a1 <-> b1: {
	style.stroke-width: 1
	source-arrowhead: {
		shape: cf-many
	}
	target-arrowhead: {
		shape: cf-many
	}
}
a2 <-> b2: {
	style.stroke-width: 3
	source-arrowhead: {
		shape: cf-many
	}
	target-arrowhead: {
		shape: cf-many
	}
}
a3 <-> b3: {
	style.stroke-width: 6
	source-arrowhead: {
		shape: cf-many
	}
	target-arrowhead: {
		shape: cf-many
	}
}

c1 <-> d1: {
	style.stroke-width: 1
	source-arrowhead: {
		shape: cf-many-required
	}
	target-arrowhead: {
		shape: cf-many-required
	}
}
c2 <-> d2: {
	style.stroke-width: 3
	source-arrowhead: {
		shape: cf-many-required
	}
	target-arrowhead: {
		shape: cf-many-required
	}
}
c3 <-> d3: {
	style.stroke-width: 6
	source-arrowhead: {
		shape: cf-many-required
	}
	target-arrowhead: {
		shape: cf-many-required
	}
}

e1 <-> f1: {
	style.stroke-width: 1
	source-arrowhead: {
		shape: cf-one
	}
	target-arrowhead: {
		shape: cf-one
	}
}
e2 <-> f2: {
	style.stroke-width: 3
	source-arrowhead: {
		shape: cf-one
	}
	target-arrowhead: {
		shape: cf-one
	}
}
e3 <-> f3: {
	style.stroke-width: 6
	source-arrowhead: {
		shape: cf-one
	}
	target-arrowhead: {
		shape: cf-one
	}
}

g1 <-> h1: {
	style.stroke-width: 1
	source-arrowhead: {
		shape: cf-one-required
	}
	target-arrowhead: {
		shape: cf-one-required
	}
}
g2 <-> h2: {
	style.stroke-width: 3
	source-arrowhead: {
		shape: cf-one-required
	}
	target-arrowhead: {
		shape: cf-one-required
	}
}
g3 <-> h3: {
	style.stroke-width: 6
	source-arrowhead: {
		shape: cf-one-required
	}
	target-arrowhead: {
		shape: cf-one-required
	}
}

c <-> d <-> f: {
	style.stroke-width: 1
	style.stroke: "orange"
	source-arrowhead: {
		shape: cf-many-required
	}
	target-arrowhead: {
		shape: cf-one
	}
}
`,
		},
		{
			name: "circle_arrowhead",
			script: `
a <-> b: circle {
  source-arrowhead: {
    shape: circle
  }
  target-arrowhead: {
    shape: circle
  }
}

c <-> d: filled-circle {
  source-arrowhead: {
    shape: circle
    style.filled: true
  }
  target-arrowhead: {
    shape: circle
    style.filled: true
  }
}`,
		},
		{
			name: "animated",
			script: `
your love life will be -> happy: { style.animated: true }
your love life will be -> harmonious: { style.animated: true }

boredom <- immortality: { style.animated: true }

Friday <-> Monday: { style.animated: true }

Insomnia -- Sleep: { style.animated: true }
Insomnia -- Wake: {
	style: {
		animated: true
		stroke-width: 2
	}
}

Insomnia -- Dream: {
	style: {
		animated: true
		stroke-width: 8
	}
}

Listen <-> Talk: {
	style.animated: true
	source-arrowhead.shape: cf-one
	target-arrowhead.shape: diamond
	label: hear
}
`,
		},
		{
			name: "sql_table_tooltip_animated",
			script: `
x: {
  shape: sql_table
	y
	tooltip: I like turtles
}

a: {
  shape: sql_table
	b
}

x.y -> a.b: {
  style.animated: true
	target-arrowhead.shape: cf-many
}
`,
		},
		{
			name: "sql_table_column_label",
			script: `x: {
  shape: sql_table
	w: int { label: width }
}
`,
		},
		{
			name: "sql_table_column_styles",
			script: `Humor in the Court: {
  shape: sql_table
	Could you see him from where you were standing?: "I could see his head."
	And where was his head?: Just above his shoulders.
  style.fill: red
  style.stroke: lightgray
  style.font-color: orange
  style.font-size: 20
}

Humor in the Court2: {
  shape: sql_table
	Could you see him from where you were standing?: "I could see his head."
	And where was his head?: Just above his shoulders.
  style.fill: red
  style.stroke: lightgray
  style.font-color: orange
  style.font-size: 30
}

manager: BatchManager {
  shape: class
	style.font-size: 20

  -num: int
  -timeout: int
  -pid

  +getStatus(): Enum
  +getJobs(): "Job[]"
  +setTimeout(seconds int)
}

manager2: BatchManager {
  shape: class
	style.font-size: 30

  -num: int
  -timeout: int
  -pid

  +getStatus(): Enum
  +getJobs(): "Job[]"
  +setTimeout(seconds int)
}
`,
		},
		{
			name: "near-alone",
			script: `
x: {
	near: top-center
}
y: {
	near: bottom-center
}
z: {
	near: center-left
}
`,
		},
		{
			name: "border-radius",
			script: `
x: {
	style.border-radius: 4
}
y: {
	style.border-radius: 10
}
multiple2: {
	style.border-radius: 6
	style.multiple: true
}
double: {
	style.border-radius: 6
	style.double-border: true
}
three-dee: {
	style.border-radius: 6
	style.3d: true
}
`,
		},
		{
			name: "sequence-inter-span-self",
			script: `
shape: sequence_diagram
a: A
b: B

a.sp1 -> b: foo
a.sp1 -> a.sp2: redirect
a.sp2 -> b: bar
`,
		},
		{
			name: "people",
			script: `
a.shape: person
b.shape: person
c.shape: person
d.shape: person
e.shape: person
f.shape: person
g.shape: person

a: -
b: --
c: ----
d: --------
e: ----------------
f: --------------------------------
g: ----------------------------------------------------------------

1.shape: person
2.shape: person
3.shape: person
4.shape: person
5.shape: person

1.width: 16
2.width: 64
3.width: 128
4.width: 512

# entering both width and height overrides aspect ratio limit
5.height: 256
5.width: 32
`,
		},
		{
			name: "complex-layers",
			script: `
desc: Multi-layer diagram of a home.

window: {
  style.double-border: true
}
roof
garage

layers: {
  window: {
    blinds
    glass
  }
  roof: {
    shingles
    starlink
    utility hookup
  }
  garage: {
    tools
    vehicles
  }
  repair: {
    desc: How to repair a home.

    steps: {
      1: {
        find contractors: {
          craigslist
          facebook
        }
      }
      2: {
        find contractors -> solicit quotes
      }
      3: {
        obtain quotes -> negotiate
      }
      4: {
        negotiate -> book the best bid
      }
    }
  }
}

scenarios: {
  storm: {
    water
    rain
    thunder
  }
}`,
		},
	}

	runa(t, tcs)
}
