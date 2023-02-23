package e2etests

import (
	_ "embed"
	"testing"
)

func testTodo(t *testing.T) {
	tcs := []testCase{
		{
			// issue https://github.com/terrastruct/d2/issues/71
			name: "container_child_edge",
			script: `
container.first -> container.second: 1->2
container -> container.second: c->2
`,
			dagreFeatureError: `Connection "(container -> container.second)[0]" goes from a container to a descendant, but layout engine "dagre" does not support this.`,
		},
		{
			name: "child_parent_edges",
			script: `a.b -> a
a.b -> a.b.c
a.b.c.d -> a.b`,
			dagreFeatureError: `Connection "(a.b -> a)[0]" goes from a container to a descendant, but layout engine "dagre" does not support this.`,
		},
		{
			name: "container_label_loop",
			script: `a: "If we were meant to fly, we wouldn't keep losing our luggage" {
  b -> c
}
a -> a`,
			dagreFeatureError: `Connection "(a -> a)[0]" is a self loop on a container, but layout engine "dagre" does not support this.`,
		},
		{
			// as nesting gets deeper, the groups advance towards `c` and may overlap its lifeline
			// needs to consider the group size when computing the distance from `a` to `c`
			// a similar effect can be seen for spans
			name: "sequence_diagram_actor_padding_nested_groups",
			script: `shape: sequence_diagram
b;a;c
b -> c
this is a message group: {
    a -> b
    and this is a nested message group: {
        a -> b
        what about more nesting: {
            a -> b
            yo: {
                a -> b
                yo: {
                    a -> b
                }
            }
        }
    }
}`,
		},
		{
			// dimensions set on containers are ignored
			name: "shape_set_width_height",
			script: `
containers: {
	circle container: {
		shape: circle

		diamond: {
			shape: diamond
			width: 128
			height: 64
		}
	}
	diamond container: {
		shape: diamond

		circle: {
			shape: circle
			width: 128
		}
	}
	oval container: {
		shape: oval

		hexagon: {
			shape: hexagon
			width: 128
			height: 64
		}
	}
	hexagon container: {
		shape: hexagon

		oval: {
			shape: oval
			width: 128
			height: 64
		}
	}
}

cloud: {
	shape: cloud
	width: 512
	height: 256
}
tall cylinder: {
	shape: cylinder
	width: 256
	height: 512
}
cloud -> class -> tall cylinder ->  users

users: {
	shape: sql_table
	id: int
	name: string
	email: string
	password: string
	last_login: datetime

	width: 800
	height: 400
}

class: {
	shape: class
	-num: int
	-timeout: int
	-pid

	+getStatus(): Enum
	+getJobs(): "Job[]"
	+setTimeout(seconds int)

	width: 800
	height: 400
}

container -> text -> code -> small code

text: {
	label: |md
	markdown text expanded to 800x400
|
	height: 800
	width: 400
}

code: |go
    a := 5
    b := a + 7
    fmt.Printf("%d", b)
| {
	width: 400
	height: 300
}

small code: |go
    a := 5
    b := a + 7
    fmt.Printf("%d", b)
| {
	width: 4
	height: 3
}
`,
		},
		{
			// issue https://github.com/terrastruct/d2/issues/748
			name: "sequence_diagram_edge_group_span_field",
			script: `
Office chatter: {
  shape: sequence_diagram
  alice: Alice
  bob: Bobby
	alice.a
  awkward small talk: {
    alice -> bob: uhm, hi
    bob -> alice: oh, hello
    icebreaker attempt: {
      alice -> bob: what did you have for lunch?
    }
    unfortunate outcome: {
      bob.a -> alice.a: that's personal
    }
  }
}
`,
		},
		{
			// issue https://github.com/terrastruct/d2/issues/748
			skip: true,
			name: "sequence_diagram_ambiguous_edge_group",
			script: `
Office chatter: {
  shape: sequence_diagram
  alice: Alice
  bob: Bobby
  awkward small talk: {
    awkward small talk.ok
    alice -> bob: uhm, hi
    bob -> alice: oh, hello
    icebreaker attempt: {
      alice -> bob: what did you have for lunch?
    }
    unfortunate outcome: {
      bob -> alice: that's personal
    }
  }
}
`,
		},
		{
			// https://github.com/terrastruct/d2/issues/791
			name: "container_icon_label",
			script: `a: Big font {
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
	style.font-size: 30
  a -> b -> c
  a: {
    a
  }
}
`,
		},
		{
			name: "container_label_edge_adjustment",
			script: `
a -> b.c -> d: {style.stroke-width: 8; target-arrowhead.shape: diamond; target-arrowhead.style.filled: true}
b.shape: cloud
e -> b.c: {style.stroke-width: 8; target-arrowhead.shape: diamond; target-arrowhead.style.filled: true}
f -> b: {
	style: {
		stroke: red
		stroke-width: 8
	}
	target-arrowhead.shape: diamond
	target-arrowhead.style.filled: true
}
g -> b: {style.stroke-width: 8; target-arrowhead.shape: diamond; target-arrowhead.style.filled: true}
b: a container label
`,
		},
	}

	runa(t, tcs)
}
