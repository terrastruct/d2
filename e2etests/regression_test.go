package e2etests

import (
	"testing"
)

func testRegression(t *testing.T) {
	tcs := []testCase{
		{
			// https://github.com/terrastruct/d2/issues/919
			name: "hex-fill",
			script: `x: {
  style.fill: "#0D32B2"
}
`,
		},
		{
			name: "dagre_special_ids",
			script: `
ninety\nnine
eighty\reight
seventy\r\nseven
a\\yode -> there
a\\"ode -> there
a\\node -> there
`,
		},
		{
			name: "empty_sequence",
			script: `
A: hello {
  shape: sequence_diagram
}

B: goodbye {
  shape: sequence_diagram
}

A->B`,
		},
		{
			name: "undeclared_nested_sequence",
			script: `shape: sequence_diagram
group.nested: {
  a -> b
}
`,
			expErr: "no actors declared in sequence diagram",
		},
		{
			name: "class_font_style_sequence",
			script: `shape: sequence_diagram
a: {
  shape: class
  style: {
    font-color: red
  }
}
`,
		},
		{
			name: "nested_steps",
			script: `a: {
  a: {
    shape: step
  }
  b: {
    shape: step
  }
  a -> b
}

c: {
  shape: step
}
d: {
  shape: step
}
c -> d
`,
		},
		{
			name: "class_span_sequence",
			script: `shape: sequence_diagram
a: { shape: class }
b

group: {
  a.t -> b.t
}
`,
		},
		{
			name: "sequence_diagram_span_cover",
			script: `shape: sequence_diagram
b.1 -> b.1
b.1 -> b.1`,
		}, {
			name: "sequence_diagram_no_message",
			script: `shape: sequence_diagram
a: A
b: B`,
		},
		{
			name: "root-container",
			script: `main: {
  x -> y
  y <- z
}

root: {
  x -> y
  y <- z
}`,
		},
		{
			name: "sequence_diagram_name_crash",
			script: `foo: {
	shape: sequence_diagram
	a -> b
}
foobar: {
	shape: sequence_diagram
	c -> d
}
foo -> foobar`,
		},
		{
			name: "sql_table_overflow",
			script: `
table: sql_table_overflow {
	shape: sql_table
	short: loooooooooooooooooooong
	loooooooooooooooooooong: short
}
table_constrained: sql_table_constrained_overflow {
	shape: sql_table
	short: loooooooooooooooooooong {
		constraint: unique
	}
	loooooooooooooooooooong: short {
		constraint: foreign_key
	}
}
`,
		},
		{
			name: "elk_alignment",
			script: `
direction: down

build_workflow: lambda-build.yaml {

	push: Push to main branch {
		style.font-size: 25
	}

	GHA: GitHub Actions {
		style.font-size: 25
	}

	S3.style.font-size: 25
	Terraform.style.font-size: 25
	AWS.style.font-size: 25

	push -> GHA: Triggers {
		style.font-size: 20
	}

	GHA -> S3: Builds zip and pushes it {
		style.font-size: 20
	}

	S3 <-> Terraform: Pulls zip to deploy {
		style.font-size: 20
	}

	Terraform -> AWS: Changes live lambdas {
		style.font-size: 20
	}
}

deploy_workflow: lambda-deploy.yaml {

	manual: Manual Trigger {
		style.font-size: 25
	}

	GHA: GitHub Actions {
		style.font-size: 25
	}

	AWS.style.font-size: 25

	Manual -> GHA: Launches {
		style.font-size: 20
	}

	GHA -> AWS: Builds zip\npushes them to S3.\n\nDeploys lambdas\nusing Terraform {
		style.font-size: 20
	}
}

apollo_workflow: apollo-deploy.yaml {

	apollo: Apollo Repo {
		style.font-size: 25
	}

	GHA: GitHub Actions {
		style.font-size: 25
	}

	AWS.style.font-size: 25

	apollo -> GHA: Triggered manually/push to master test test test test test test test {
		style.font-size: 20
	}

	GHA -> AWS: test {
		style.font-size: 20
	}
}
`,
		},
		{
			name: "dagre_edge_label_spacing",
			script: `direction: right

build_workflow: lambda-build.yaml {

	push: Push to main branch {
		style.font-size: 25
	}
	GHA: GitHub Actions {
		style.font-size: 25
	}
	S3.style.font-size: 25
	Terraform.style.font-size: 25
	AWS.style.font-size: 25

	push -> GHA: Triggers
	GHA -> S3: Builds zip & pushes it
	S3 <-> Terraform: Pulls zip to deploy
	Terraform -> AWS: Changes the live lambdas
}
`,
		},
		{
			name: "query_param_escape",
			script: `my network: {
  icon: https://icons.terrastruct.com/infra/019-network.svg?fuga=1&hoge
}
`,
		},
		{
			name: "elk_order",
			script: `queue: {
  shape: queue
  label: ''

  M0
  M1
  M2
  M3
  M4
  M5
  M6
}

m0_desc: |md
  Oldest message
|
m0_desc -> queue.M0

m2_desc: |md
  Offset
|
m2_desc -> queue.M2

m5_desc: |md
  Last message
|
m5_desc -> queue.M5

m6_desc: |md
  Next message will be\
  inserted here
|
m6_desc -> queue.M6
`,
		},
		{
			name: "unnamed_class_table_code",
			script: `

class2 -> users -> code

class2: "" {
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
`,
		},
		{
			name: "elk_img_empty_label_panic",
			script: `
img: {
	label: ""
	shape: image
	icon: https://icons.terrastruct.com/infra/019-network.svg
}
ico: {
	label: ""
	icon: https://icons.terrastruct.com/infra/019-network.svg
}
`,
		},
		{
			name: "only_header_class_table",
			script: `

class2: RefreshAuthorizationPolicyProtocolServerSideTranslatorProtocolBuffer {
	shape: class
}

table: RefreshAuthorizationPolicyCache {
	shape: sql_table
}

table with short col: RefreshAuthorizationPolicyCache {
	shape: sql_table
	ok
}

class2 -> table -> table with short col
`,
		},
		{
			name: "overlapping-edge-label",
			script: `k8s: Kubernetes
k8s.m1: k8s-master1
k8s.m2: k8s-master2
k8s.m3: k8s-master3
k8s.w1: k8s-worker1
k8s.w2: k8s-worker2
k8s.w3: k8s-worker3

osvc: opensvc
osvc.vm1: VM1
osvc.vm2: VM2

k8s -> osvc: keycloak
k8s -> osvc: heptapod
k8s -> osvc: harbor
k8s -> osvc: vault
`,
		},
		{
			name: "no-lexer",
			script: `x: |d2
  x -> y
|
`,
		},
		{
			name: "dagre_broken_arrowhead",
			script: `
a.b -> a.c: "line 1\nline 2\nline 3\nline 4" {
	style: {
		font-color: red
		stroke: red
	}
	target-arrowhead: {
		shape: diamond
	}
}
a.1 -> a.c
a.2 <-> a.c
a.c {
	style.stroke: white
	d
}
`,
		},
		{
			name: "code_leading_trailing_newlines",
			script: `
hello world: |python


  # 2 leading, 2 trailing
  def hello():

    print "world"


|

no trailing: |python


  # 2 leading
  def hello():

    print "world"
|

no leading: |python
  # 2 trailing
  def hello():

    print "world"


|
`,
		},
		{
			name: "code_leading_newlines",
			script: `
5 leading: |python





  def hello():

    print "world"
|
8 total: |python
  # 1 leading
  # 2 leading
  # 3 leading
  # 4 leading
  # 5 leading
  def hello():

    print "world"
|
1 leading: |python

  def hello():

    print "world"
|
4 total: |python
  # 1 leading
  def hello():

    print "world"
|
2 leading: |python


  def hello():

    print "world"
|
5 total: |python
  # 1 leading
  # 2 leading
  def hello():

    print "world"
|
`,
		},
		{
			name: "code_trailing_newlines",
			script: `
5 trailing: |python
  def hello():

    print "world"





|
8 total: |python
  def hello():

    print "world"
  # 1 trailing
  # 2 trailing
  # 3 trailing
  # 4 trailing
  # 5 trailing
|
1 trailing: |python
  def hello():

    print "world"

|
4 total: |python
  def hello():

    print "world"
  # 1 trailing
|
2 trailing: |python
  def hello():

    print "world"


|
5 total: |python
  def hello():

    print "world"
  # 1 trailing
  # 2 trailing
|
`,
		},
		{
			name: "md_h1_li_li",
			script: mdTestScript(`
# hey
- they
	1. they
`),
		},
		{
			name: "elk_loop_panic",
			script: `x: {
  a
  b
}

x.a -> x.a
`,
		},
		{
			name: "opacity-on-label",
			script: `x.style.opacity: 0.4
y: |md
  linux: because a PC is a terrible thing to waste
| {
	style.opacity: 0.4
}
x -> a: {
  label: You don't have to know how the computer works,\njust how to work the computer.
  style.opacity: 0.4
}
`,
		},
		{
			name: "sequence_diagram_self_edge_group_overlap",
			script: `
shape: sequence_diagram
a: A
b: B
c: C
group 1: {
	a -> a
}
group 2: {
	a -> b
}
group 3: {
	a -> a.a
}
group 4: {
	a.a -> b
}
group 5: {
	b -> b
	b -> b
}
group 6: {
	b -> a
}
group 7: {
	a -> a
}
group 8: {
	a -> a
}
a -> a
group 9: {
	a -> a
}
a -> a

b -> c
group 10: {
	c -> c
}
b -> c
group 11: {
	c -> c
}
b -> c

`,
		},
		{
			name: "empty_class_height",
			script: `
class1: class with rows {
	shape: class
	-num: int
	-timeout: int
}

class2: class without rows {
	shape: class
}
`,
		},
		{
			name: "just-width",
			script: `x: "teamwork: having someone to blame" {
  width: 100
}
`,
		},
		{
			name: "sequence-panic",
			script: `
shape: sequence_diagram

a

group: {
  inner_group: {
    a -> b
  }
}
`,
			expErr: "could not find center of b. Is it declared as an actor?",
		},
		{
			name: "ampersand-escape",
			script: `hy: &∈ {
  tooltip: beans & rice
}
`,
		},
		{
			name: "dagre-disconnect",
			script: `a: {
  k.t -> f.i
  f.g -> _.s.n
}
k
k.s <-> u.o
h.m.s -> a.f.g

a.f.j -> u.s.j
u: {
  c -> _.s.z.c
}

s: {
  n: {
    style.stroke: red
    f
  }
}

s.n -> y.r: {style.stroke-width: 8; style.stroke: red}
y.r -> a.g.i: 1\n2\n3\n4
`,
		},
		{
			name: "sequence-note-escape-group",
			script: `shape: sequence_diagram
a
b

"04:20,11:20": {
  "loop through each table": {
    a."start_time = datetime.datetime.now"
    a -> b
  }
}
`,
		},
		loadFromFile(t, "unconnected"),
		{
			name: "straight_hierarchy_container_direction_right",
			script: `
direction: right
a
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
			name:   "link_with_ampersand",
			script: `a.link: https://calendar.google.com/calendar/u/0/r?tab=mc&pli=1`,
		},
		{
			name: "bold_edge_label",
			script: `
direction: right
x -> y: sync
y -> z: sync {
	style.bold: true
}
`,
		},
		{
			name: "grid_in_constant_near",
			script: `
a
b
c
x: {
	near: top-right
	grid-columns: 1
	y
	z
}
`,
		},
		{
			name: "md_font_weight",
			script: `
explanation: |md
# I can do headers

- lists
- lists

And other normal markdown stuff
|
`,
		},
		{
			name: "grid_panic",
			script: `
			2 rows 1 obj: {
				grid-rows: 2

				one
			}
			3 rows 2 obj: {
				grid-rows: 3

				one
				two
			}
			4 columns 2 obj: {
				grid-columns: 4

				one
				two
			}
`,
		},
		{
			name: "long_arrowhead_label",
			script: `
a -> b: {
	target-arrowhead: "a to b with unexpectedly long target arrowhead label"
}
`,
		},
		{
			name: "arrowhead_sizes_with_labels",
			script: `
triangle: {
	a <-> b: {
		source-arrowhead: 1
		target-arrowhead: 1
	}
	c <-> d: {
		source-arrowhead: 1
		target-arrowhead: 1
		style.stroke-width: 8
	}
}
none: {
	a -- b: {
		source-arrowhead: 1
		target-arrowhead: 1
	}
	c -- d: {
		source-arrowhead: 1
		target-arrowhead: 1
		style.stroke-width: 8
	}
}
arrow: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: arrow
		}
		target-arrowhead: 1 {
			shape: arrow
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: arrow
		}
		target-arrowhead: 1 {
			shape: arrow
		}
		style.stroke-width: 8
	}
}
diamond: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: diamond
		}
		target-arrowhead: 1 {
			shape: diamond
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: diamond
		}
		target-arrowhead: 1 {
			shape: diamond
		}
		style.stroke-width: 8
	}
}
filled diamond: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: diamond
			style.filled: true
		}
		target-arrowhead: 1 {
			shape: diamond
			style.filled: true
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: diamond
			style.filled: true
		}
		target-arrowhead: 1 {
			shape: diamond
			style.filled: true
		}
		style.stroke-width: 8
	}
}
circle: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: circle
		}
		target-arrowhead: 1 {
			shape: circle
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: circle
		}
		target-arrowhead: 1 {
			shape: circle
		}
		style.stroke-width: 8
	}
}
filled circle: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: circle
			style.filled: true
		}
		target-arrowhead: 1 {
			shape: circle
			style.filled: true
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: circle
			style.filled: true
		}
		target-arrowhead: 1 {
			shape: circle
			style.filled: true
		}
		style.stroke-width: 8
	}
}
cf one: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: cf-one
		}
		target-arrowhead: 1 {
			shape: cf-one
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: cf-one
		}
		target-arrowhead: 1 {
			shape: cf-one
		}
		style.stroke-width: 8
	}
}
cf one required: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: cf-one-required
		}
		target-arrowhead: 1 {
			shape: cf-one-required
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: cf-one-required
		}
		target-arrowhead: 1 {
			shape: cf-one-required
		}
		style.stroke-width: 8
	}
}
cf many: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: cf-many
		}
		target-arrowhead: 1 {
			shape: cf-many
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: cf-many
		}
		target-arrowhead: 1 {
			shape: cf-many
		}
		style.stroke-width: 8
	}
}
cf many required: {
	a <-> b: {
		source-arrowhead: 1 {
			shape: cf-many-required
		}
		target-arrowhead: 1 {
			shape: cf-many-required
		}
	}
	c <-> d: {
		source-arrowhead: 1 {
			shape: cf-many-required
		}
		target-arrowhead: 1 {
			shape: cf-many-required
		}
		style.stroke-width: 8
	}
}
`,
		},
		{
			name:   "dagre_child_id_id",
			script: `direction:right; id -> x.id -> y.z.id`,
		},
		loadFromFile(t, "slow_grid"),
		loadFromFile(t, "grid_oom"),
		loadFromFile(t, "cylinder_grid_label"),
		loadFromFile(t, "grid_with_latex"),
		loadFromFile(t, "icons_on_top"),
		loadFromFile(t, "dagre_disconnected_edge"),
		loadFromFile(t, "outside_grid_label_position"),
		loadFromFile(t, "arrowhead_font_color"),
		loadFromFile(t, "multiple_constant_nears"),
		loadFromFile(t, "empty_nested_grid"),
		loadFromFile(t, "code_font_size"),
		loadFromFile(t, "disclaimer"),
		loadFromFile(t, "grid_rows_gap_bug"),
		loadFromFile(t, "grid_image_label_position"),
		loadFromFile(t, "glob_dimensions"),
		loadFromFile(t, "shaped_grid_positioning"),
		loadFromFile(t, "cloud_shaped_grid"),
		loadFromFileWithOptions(t, "nested_layout_bug", testCase{testSerialization: true}),
		loadFromFile(t, "disconnect_direction_right"),
	}

	runa(t, tcs)
}
