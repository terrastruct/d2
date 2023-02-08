package e2etests

import (
	"testing"
)

func testRegression(t *testing.T) {
	tcs := []testCase{
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
		}, {
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

class -> users -> code

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

class: RefreshAuthorizationPolicyProtocolServerSideTranslatorProtocolBuffer {
	shape: class
}

table: RefreshAuthorizationPolicyCache {
	shape: sql_table
}

table with short col: RefreshAuthorizationPolicyCache {
	shape: sql_table
	ok
}

class -> table -> table with short col
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
	}

	runa(t, tcs)
}
