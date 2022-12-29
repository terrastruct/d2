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
		},
		{
			// issue https://github.com/terrastruct/d2/issues/263
			name: "tall_edge_label",
			script: `
a -> b: There\nonce\nwas\na\nvery\ntall\nedge\nlabel
`,
		},
		{
			// issue https://github.com/terrastruct/d2/issues/263
			name: "font_sizes_large",
			script: `
eight.style.font-size: 8
sixteen.style.font-size: 16
thirty two.style.font-size: 32
sixty four.style.font-size: 64
ninety nine.style.font-size: 99

eight -> sixteen : twelve {
	style.font-size: 12
}
sixteen -> thirty two : twenty four {
	style.font-size: 24
}
thirty two -> sixty four: forty eight {
	style.font-size: 48
}
sixty four -> ninety nine: eighty one {
	style.font-size: 81
}
`,
		},
		{
			// issue https://github.com/terrastruct/d2/issues/19
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
		}, {
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
		width: 512

		diamond: {
			shape: diamond
			width: 128
			height: 64
		}
	}
	diamond container: {
		shape: diamond
		width: 512
		height: 256

		circle: {
			shape: circle
			width: 128
		}
	}
	oval container: {
		shape: oval
		width: 512
		height: 256

		hexagon: {
			shape: hexagon
			width: 128
			height: 64
		}
	}
	hexagon container: {
		shape: hexagon
		width: 512
		height: 256

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
	}

	runa(t, tcs)
}
