package d2sketch_test

import (
	"context"
	"encoding/xml"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func TestSketch(t *testing.T) {
	t.Parallel()

	tcs := []testCase{
		{
			name: "basic",
			script: `a -> b
`,
		},
		{
			name: "child to child",
			script: `winter.snow -> summer.sun
		`,
		},
		{
			name:   "elk corners",
			engine: "elk",
			script: `a -> b
b -> c
a -> c
c -> a
		`,
		},
		{
			name: "animated",
			script: `winter.snow -> summer.sun -> trees -> winter.snow: { style.animated: true }
		`,
		},
		{
			name: "connection label",
			script: `a -> b: hello
		`,
		},
		{
			name: "crows feet",
			script: `a1 <-> b1: {
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
			name: "twitter",
			script: `timeline mixer: "" {
  explanation: |md
    ## **Timeline mixer**
    - Inject ads, who-to-follow, onboarding
    - Conversation module
    - Cursoring,pagination
    - Tweat deduplication
    - Served data logging
  |
}
People discovery: "People discovery \nservice"
admixer: Ad mixer {
  style.fill: "#c1a2f3"
}

onboarding service: "Onboarding \nservice"
timeline mixer -> People discovery
timeline mixer -> onboarding service
timeline mixer -> admixer
container0: "" {
  graphql
  comment
  tlsapi
}
container0.graphql: GraphQL\nFederated Strato Column {
  shape: image
  icon: https://upload.wikimedia.org/wikipedia/commons/thumb/1/17/GraphQL_Logo.svg/1200px-GraphQL_Logo.svg.png
}
container0.comment: |md
  ## Tweet/user content hydration, visibility filtering
|
container0.tlsapi: TLS-API (being deprecated)
container0.graphql -> timeline mixer
timeline mixer <- container0.tlsapi
twitter fe: "Twitter Frontend " {
  icon: https://icons.terrastruct.com/social/013-twitter-1.svg
  shape: image
}
twitter fe -> container0.graphql: iPhone web
twitter fe -> container0.tlsapi: HTTP Android
web: Web {
  icon: https://icons.terrastruct.com/azure/Web%20Service%20Color/App%20Service%20Domains.svg
  shape: image
}

Iphone: {
  icon: 'https://ss7.vzw.com/is/image/VerizonWireless/apple-iphone-12-64gb-purple-53017-mjn13ll-a?$device-lg$'
  shape: image
}
Android: {
  icon: https://cdn4.iconfinder.com/data/icons/smart-phones-technologies/512/android-phone.png
  shape: image
}

web -> twitter fe
timeline scorer: "Timeline\nScorer" {
	style.fill: "#ffdef1"
}
home ranker: Home Ranker

timeline service: Timeline Service
timeline mixer -> timeline scorer: Thrift RPC
timeline mixer -> home ranker: {
  style.stroke-dash: 4
  style.stroke: "#000E3D"
}
timeline mixer -> timeline service
home mixer: Home mixer {
  # style.fill "#c1a2f3"
}
container0.graphql -> home mixer: {
  style.stroke-dash: 4
  style.stroke: "#000E3D"
}
home mixer -> timeline scorer
home mixer -> home ranker: {
  style.stroke-dash: 4
  style.stroke: "#000E3D"
}
home mixer -> timeline service
manhattan 2: Manhattan
gizmoduck: Gizmoduck
socialgraph: Social graph
tweetypie: Tweety Pie
home mixer -> manhattan 2
home mixer -> gizmoduck
home mixer -> socialgraph
home mixer -> tweetypie
Iphone -> twitter fe
Android -> twitter fe
prediction service2: Prediction Service {
  shape: image
  icon: https://cdn-icons-png.flaticon.com/512/6461/6461819.png
}
home scorer: Home Scorer {
	style.fill: "#ffdef1"
}
manhattan: Manhattan
memcache: Memcache {
  icon: https://d1q6f0aelx0por.cloudfront.net/product-logos/de041504-0ddb-43f6-b89e-fe04403cca8d-memcached.png
}

fetch: Fetch {
  style.multiple: true
  shape: step
}
feature: Feature {
  style.multiple: true
  shape: step
}
scoring: Scoring {
  style.multiple: true
  shape: step
}
fetch -> feature
feature -> scoring

prediction service: Prediction Service {
  shape: image
  icon: https://cdn-icons-png.flaticon.com/512/6461/6461819.png
}
scoring -> prediction service
fetch -> container2.crmixer

home scorer -> manhattan: ""

home scorer -> memcache: ""
home scorer -> prediction service2
home ranker -> home scorer
home ranker -> container2.crmixer: Candidate Fetch
container2: "" {
  style.stroke: "#000E3D"
  style.fill: "#ffffff"
  crmixer: CrMixer {
    style.fill: "#F7F8FE"
  }
  earlybird: EarlyBird
  utag: Utag
  space: Space
  communities: Communities
}
etc: ...etc

home scorer -> etc: Feature Hydration

feature -> manhattan
feature -> memcache
feature -> etc: Candidate sources
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
			name: "sql_tables",
			script: `users: {
	shape: sql_table
	id: int
	name: string
	email: string
	password: string
	last_login: datetime
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
	tracking_number: string {constraint: primary_key}
	status: string
}

users.id <-> orders.user_id
products.id <-> orders.product_id
shipments.order_id <-> orders.id`,
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
		},
		{
			name: "arrowheads",
			script: `
a: ""
b: ""
a.1 -- b.1: none
a.2 <-> b.2: arrow {
	source-arrowhead.shape: arrow
	target-arrowhead.shape: arrow
}
a.3 <-> b.3: triangle {
	source-arrowhead.shape: triangle
	target-arrowhead.shape: triangle
}
a.4 <-> b.4: diamond {
	source-arrowhead.shape: diamond
	target-arrowhead.shape: diamond
}
a.5 <-> b.5: diamond filled {
	source-arrowhead: {
		shape: diamond
		style.filled: true
	}
	target-arrowhead: {
		shape: diamond
		style.filled: true
	}
}
a.6 <-> b.6: cf-many {
	source-arrowhead.shape: cf-many
	target-arrowhead.shape: cf-many
}
a.7 <-> b.7: cf-many-required {
	source-arrowhead.shape: cf-many-required
	target-arrowhead.shape: cf-many-required
}
a.8 <-> b.8: cf-one {
	source-arrowhead.shape: cf-one
	target-arrowhead.shape: cf-one
}
a.9 <-> b.9: cf-one-required {
	source-arrowhead.shape: cf-one-required
	target-arrowhead.shape: cf-one-required
}
a.10 <-> b.10: box {
	source-arrowhead.shape: box
	target-arrowhead.shape: box
}
a.11 <-> b.11: box-filled {
	source-arrowhead: {
		shape: box
		style.filled: true
	}
	target-arrowhead: {
		shape: box
		style.filled: true
	}
}
`,
		},
		{
			name: "opacity",
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
users: {
	shape: sql_table
	last_login: datetime
	style.opacity: 0.4
}
`,
		},
		{
			name: "overlay",
			script: `bright: {
	style.stroke: "#000"
	style.font-color: "#000"
	style.fill: "#fff"
}
normal: {
	style.stroke: "#000"
	style.font-color: "#000"
	style.fill: "#ccc"
}
dark: {
	style.stroke: "#000"
	style.font-color: "#fff"
	style.fill: "#555"
}
darker: {
	style.stroke: "#000"
	style.font-color: "#fff"
	style.fill: "#000"
}
`,
		},
		{
			name:    "terminal",
			themeID: d2themescatalog.Terminal.ID,
			script: `network: {
  cell tower: {
		satellites: {
			shape: stored_data
      style.multiple: true
		}

		transmitter

		satellites -> transmitter: send
		satellites -> transmitter: send
		satellites -> transmitter: send
  }

  online portal: {
    ui: { shape: hexagon }
  }

  data processor: {
    storage: {
      shape: cylinder
      style.multiple: true
    }
  }

  cell tower.transmitter -> data processor.storage: phone logs
}

user: {
  shape: person
  width: 130
}

user -> network.cell tower: make call
user -> network.online portal.ui: access {
  style.stroke-dash: 3
}

api server -> network.online portal.ui: display
api server -> logs: persist
logs: { shape: page; style.multiple: true }

network.data processor -> api server
`,
		},
		{
			name:    "basic dark",
			themeID: 200,
			script: `a -> b
`,
		},
		{
			name:    "child to child dark",
			themeID: 200,
			script: `winter.snow -> summer.sun
		`,
		},
		{
			name:    "animated dark",
			themeID: 200,
			script: `winter.snow -> summer.sun -> trees -> winter.snow: { style.animated: true }
		`,
		},
		{
			name:    "connection label dark",
			themeID: 200,
			script: `a -> b: hello
		`,
		},
		{
			name:    "crows feet dark",
			themeID: 200,
			script: `a1 <-> b1: {
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
			name:    "twitter dark",
			themeID: 200,
			script: `timeline mixer: "" {
  explanation: |md
    ## **Timeline mixer**
    - Inject ads, who-to-follow, onboarding
    - Conversation module
    - Cursoring,pagination
    - Tweat deduplication
    - Served data logging
  |
}
People discovery: "People discovery \nservice"
admixer: Ad mixer {
  style.fill: "#c1a2f3"
}

onboarding service: "Onboarding \nservice"
timeline mixer -> People discovery
timeline mixer -> onboarding service
timeline mixer -> admixer
container0: "" {
  graphql
  comment
  tlsapi
}
container0.graphql: GraphQL\nFederated Strato Column {
  shape: image
  icon: https://upload.wikimedia.org/wikipedia/commons/thumb/1/17/GraphQL_Logo.svg/1200px-GraphQL_Logo.svg.png
}
container0.comment: |md
  ## Tweet/user content hydration, visibility filtering
|
container0.tlsapi: TLS-API (being deprecated)
container0.graphql -> timeline mixer
timeline mixer <- container0.tlsapi
twitter fe: "Twitter Frontend " {
  icon: https://icons.terrastruct.com/social/013-twitter-1.svg
  shape: image
}
twitter fe -> container0.graphql: iPhone web
twitter fe -> container0.tlsapi: HTTP Android
web: Web {
  icon: https://icons.terrastruct.com/azure/Web%20Service%20Color/App%20Service%20Domains.svg
  shape: image
}

Iphone: {
  icon: 'https://ss7.vzw.com/is/image/VerizonWireless/apple-iphone-12-64gb-purple-53017-mjn13ll-a?$device-lg$'
  shape: image
}
Android: {
  icon: https://cdn4.iconfinder.com/data/icons/smart-phones-technologies/512/android-phone.png
  shape: image
}

web -> twitter fe
timeline scorer: "Timeline\nScorer" {
	style.fill: "#ffdef1"
}
home ranker: Home Ranker

timeline service: Timeline Service
timeline mixer -> timeline scorer: Thrift RPC
timeline mixer -> home ranker: {
  style.stroke-dash: 4
  style.stroke: "#000E3D"
}
timeline mixer -> timeline service
home mixer: Home mixer {
  # style.fill "#c1a2f3"
}
container0.graphql -> home mixer: {
  style.stroke-dash: 4
  style.stroke: "#000E3D"
}
home mixer -> timeline scorer
home mixer -> home ranker: {
  style.stroke-dash: 4
  style.stroke: "#000E3D"
}
home mixer -> timeline service
manhattan 2: Manhattan
gizmoduck: Gizmoduck
socialgraph: Social graph
tweetypie: Tweety Pie
home mixer -> manhattan 2
home mixer -> gizmoduck
home mixer -> socialgraph
home mixer -> tweetypie
Iphone -> twitter fe
Android -> twitter fe
prediction service2: Prediction Service {
  shape: image
  icon: https://cdn-icons-png.flaticon.com/512/6461/6461819.png
}
home scorer: Home Scorer {
	style.fill: "#ffdef1"
}
manhattan: Manhattan
memcache: Memcache {
  icon: https://d1q6f0aelx0por.cloudfront.net/product-logos/de041504-0ddb-43f6-b89e-fe04403cca8d-memcached.png
}

fetch: Fetch {
  style.multiple: true
  shape: step
}
feature: Feature {
  style.multiple: true
  shape: step
}
scoring: Scoring {
  style.multiple: true
  shape: step
}
fetch -> feature
feature -> scoring

prediction service: Prediction Service {
  shape: image
  icon: https://cdn-icons-png.flaticon.com/512/6461/6461819.png
}
scoring -> prediction service
fetch -> container2.crmixer

home scorer -> manhattan: ""

home scorer -> memcache: ""
home scorer -> prediction service2
home ranker -> home scorer
home ranker -> container2.crmixer: Candidate Fetch
container2: "" {
  style.stroke: "#000E3D"
  style.fill: "#ffffff"
  crmixer: CrMixer {
    style.fill: "#F7F8FE"
  }
  earlybird: EarlyBird
  utag: Utag
  space: Space
  communities: Communities
}
etc: ...etc

home scorer -> etc: Feature Hydration

feature -> manhattan
feature -> memcache
feature -> etc: Candidate sources
		`,
		},
		{
			name:    "all_shapes dark",
			themeID: 200,
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
			name:    "sql_tables dark",
			themeID: 200,
			script: `users: {
	shape: sql_table
	id: int
	name: string
	email: string
	password: string
	last_login: datetime
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
	tracking_number: string {constraint: primary_key}
	status: string
}

users.id <-> orders.user_id
products.id <-> orders.product_id
shipments.order_id <-> orders.id`,
		},
		{
			name:    "class dark",
			themeID: 200,
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
		},
		{
			name:    "arrowheads dark",
			themeID: 200,
			script: `
a: ""
b: ""
a.1 -- b.1: none
a.2 <-> b.2: arrow {
	source-arrowhead.shape: arrow
	target-arrowhead.shape: arrow
}
a.3 <-> b.3: triangle {
	source-arrowhead.shape: triangle
	target-arrowhead.shape: triangle
}
a.4 <-> b.4: diamond {
	source-arrowhead.shape: diamond
	target-arrowhead.shape: diamond
}
a.5 <-> b.5: diamond filled {
	source-arrowhead: {
		shape: diamond
		style.filled: true
	}
	target-arrowhead: {
		shape: diamond
		style.filled: true
	}
}
a.6 <-> b.6: cf-many {
	source-arrowhead.shape: cf-many
	target-arrowhead.shape: cf-many
}
a.7 <-> b.7: cf-many-required {
	source-arrowhead.shape: cf-many-required
	target-arrowhead.shape: cf-many-required
}
a.8 <-> b.8: cf-one {
	source-arrowhead.shape: cf-one
	target-arrowhead.shape: cf-one
}
a.9 <-> b.9: cf-one-required {
	source-arrowhead.shape: cf-one-required
	target-arrowhead.shape: cf-one-required
}
`,
		},
		{
			name:    "opacity dark",
			themeID: 200,
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
users: {
	shape: sql_table
	last_login: datetime
	style.opacity: 0.4
}
`,
		},
		{
			name: "root-fill",
			script: `style.fill: honeydew
style.stroke: LightSteelBlue
style.double-border: true

title: Flow-I (Warehousing, Installation) {
  near: top-center
  shape: text
  style: {
    font-size: 24
    bold: false
    underline: false
  }
}
OEM Factory
OEM Factory -> OEM Warehouse
OEM Factory -> Distributor Warehouse
OEM Factory -> company Warehouse

company Warehouse.Master -> company Warehouse.Regional-1
company Warehouse.Master -> company Warehouse.Regional-2
company Warehouse.Master -> company Warehouse.Regional-N
company Warehouse.Regional-1 -> company Warehouse.Regional-2
company Warehouse.Regional-2 -> company Warehouse.Regional-N
company Warehouse.Regional-N -> company Warehouse.Regional-1

company Warehouse.explanation: |md
  ### company Warehouse
  - Asset Tagging
  - Inventory
  - Staging
  - Dispatch to Site
|
`,
		},
		{
			name: "double-border",
			script: `a: {
  style.double-border: true
  b
}
c: {
  shape: oval
  style.double-border: true
  d
}
normal: {
  nested normal
}
something
`,
		},
		{
			name: "class_and_sqlTable_border_radius",
			script: `
				a: {
					shape: sql_table
					id: int {constraint: primary_key}
					disk: int {constraint: foreign_key}

					json: jsonb  {constraint: unique}
					last_updated: timestamp with time zone

					style: {
						fill: red
						border-radius: 0
					}
				}

				b: {
					shape: class

					field: "[]string"
					method(a uint64): (x, y int)

					style: {
						border-radius: 0
					}
				}

				c: {
					shape: class
					style: {
						border-radius: 0
					}
				}

				d: {
					shape: sql_table
					style: {
						border-radius: 0
					}
				}
			`,
		},
		{
			name: "paper-real",
			script: `style.fill-pattern: paper
style.fill: "#947A6D"
NETWORK: {
  style: {
	  stroke: black
    fill-pattern: dots
    double-border: true
    fill: "#E7E9EE"
    font: mono
  }
  CELL TOWER: {
		style: {
			stroke: black
			fill-pattern: dots
			fill: "#F5F6F9"
			font: mono
		}
		satellites: SATELLITES {
			shape: stored_data
			style: {
				font: mono
				fill: white
				stroke: black
				multiple: true
			}
		}

		transmitter: TRANSMITTER {
			style: {
				font: mono
				fill: white
				stroke: black
			}
		}

		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
  }
}
`},
		{
			name: "dots-real",
			script: `
NETWORK: {
  style: {
	  stroke: black
    fill-pattern: dots
    double-border: true
    fill: "#E7E9EE"
    font: mono
  }
  CELL TOWER: {
		style: {
			stroke: black
			fill-pattern: dots
			fill: "#F5F6F9"
			font: mono
		}
		satellites: SATELLITES {
			shape: stored_data
			style: {
				font: mono
				fill: white
				stroke: black
				multiple: true
			}
		}

		transmitter: TRANSMITTER {
			style: {
				font: mono
				fill: white
				stroke: black
			}
		}

		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
		satellites -> transmitter: SEND {
			style.stroke: black
			style.font: mono
		}
  }
}
D2 Parser: {
	style.fill-pattern: grain
  shape: class

  +reader: io.RuneReader
  # Default visibility is + so no need to specify.
  readerPos: d2ast.Position

  # Private field.
  -lookahead: "[]rune"

  # Escape the # to prevent being parsed as comment
  #lookaheadPos: d2ast.Position
  # Or just wrap in quotes
  "#peekn(n int)": (s string, eof bool)

  +peek(): (r rune, eof bool)
  rewind()
  commit()
}
`,
		},
		{
			name: "dots-3d",
			script: `x: {style.3d: true; style.fill-pattern: dots}
y: {shape: hexagon; style.3d: true; style.fill-pattern: dots}
`,
		},
		{
			name: "dots-multiple",
			script: `
rectangle: {shape: "rectangle"; style.fill-pattern: dots; style.multiple: true}
square: {shape: "square"; style.fill-pattern: dots; style.multiple: true}
page: {shape: "page"; style.fill-pattern: dots; style.multiple: true}
parallelogram: {shape: "parallelogram"; style.fill-pattern: dots; style.multiple: true}
document: {shape: "document"; style.fill-pattern: dots; style.multiple: true}
cylinder: {shape: "cylinder"; style.fill-pattern: dots; style.multiple: true}
queue: {shape: "queue"; style.fill-pattern: dots; style.multiple: true}
package: {shape: "package"; style.fill-pattern: dots; style.multiple: true}
step: {shape: "step"; style.fill-pattern: dots; style.multiple: true}
callout: {shape: "callout"; style.fill-pattern: dots; style.multiple: true}
stored_data: {shape: "stored_data"; style.fill-pattern: dots; style.multiple: true}
person: {shape: "person"; style.fill-pattern: dots; style.multiple: true}
diamond: {shape: "diamond"; style.fill-pattern: dots; style.multiple: true}
oval: {shape: "oval"; style.fill-pattern: dots; style.multiple: true}
circle: {shape: "circle"; style.fill-pattern: dots; style.multiple: true}
hexagon: {shape: "hexagon"; style.fill-pattern: dots; style.multiple: true}
cloud: {shape: "cloud"; style.fill-pattern: dots; style.multiple: true}

rectangle -> square -> page
parallelogram -> document -> cylinder
queue -> package -> step
callout -> stored_data -> person
diamond -> oval -> circle
hexagon -> cloud
`,
		},
		{
			name: "dots-all",
			script: `
rectangle: {shape: "rectangle"; style.fill-pattern: dots}
square: {shape: "square"; style.fill-pattern: dots}
page: {shape: "page"; style.fill-pattern: dots}
parallelogram: {shape: "parallelogram"; style.fill-pattern: dots}
document: {shape: "document"; style.fill-pattern: dots}
cylinder: {shape: "cylinder"; style.fill-pattern: dots}
queue: {shape: "queue"; style.fill-pattern: dots}
package: {shape: "package"; style.fill-pattern: dots}
step: {shape: "step"; style.fill-pattern: dots}
callout: {shape: "callout"; style.fill-pattern: dots}
stored_data: {shape: "stored_data"; style.fill-pattern: dots}
person: {shape: "person"; style.fill-pattern: dots}
diamond: {shape: "diamond"; style.fill-pattern: dots}
oval: {shape: "oval"; style.fill-pattern: dots}
circle: {shape: "circle"; style.fill-pattern: dots}
hexagon: {shape: "hexagon"; style.fill-pattern: dots}
cloud: {shape: "cloud"; style.fill-pattern: dots}

rectangle -> square -> page
parallelogram -> document -> cylinder
queue -> package -> step
callout -> stored_data -> person
diamond -> oval -> circle
hexagon -> cloud
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
			name: "unfilled_triangle",
			script: `
direction: right

A <-> B: default {
  source-arrowhead.style.filled: false
  target-arrowhead.style.filled: false
}
C <-> D: triangle {
  source-arrowhead: {
    shape: triangle
    style.filled: false
  }
  target-arrowhead: {
    shape: triangle
    style.filled: false
  }
}`,
		},
		{
			name: "connection-style-fill",
			script: `
shape: sequence_diagram
customer
employee
rental
item

(* -> *)[*].style.fill: black
(* -> *)[*].style.font-color: white

customer -> employee: "rent(this, i, p)"
employee -> rental: "new(this, i, p)"
rental -> employee
employee -> rental: isValid()
rental -> item: isRentable(c)
item -> customer: is(Adult)
customer -> item: true
`,
		},
		{
			name: "test-gradient-fill-values-in-sketch-mode",
			script: `
				x->y
				x.style.fill: "linear-gradient(#000000,#ffffff)"
				y.style.fill: "linear-gradient(#ffffff,#000000)"
			`,
		},
	}
	runa(t, tcs)
}

type testCase struct {
	name    string
	themeID int64
	script  string
	skip    bool
	engine  string
}

func runa(t *testing.T, tcs []testCase) {
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}
			t.Parallel()

			run(t, tc)
		})
	}
}

func run(t *testing.T, tc testCase) {
	ctx := context.Background()
	ctx = log.WithTB(ctx, t)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	ruler, err := textmeasure.NewRuler()
	if !tassert.Nil(t, err) {
		return
	}

	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		if strings.EqualFold(engine, "elk") {
			return d2elklayout.DefaultLayout, nil
		}
		return d2dagrelayout.DefaultLayout, nil
	}
	renderOpts := &d2svg.RenderOpts{
		Sketch:  go2.Pointer(true),
		ThemeID: go2.Pointer(tc.themeID),
	}
	diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
		Ruler:          ruler,
		Layout:         &tc.engine,
		LayoutResolver: layoutResolver,
		FontFamily:     go2.Pointer(d2fonts.HandDrawn),
	}, renderOpts)
	if !tassert.Nil(t, err) {
		return
	}

	dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestSketch/"))
	pathGotSVG := filepath.Join(dataPath, "sketch.got.svg")

	svgBytes, err := d2svg.Render(diagram, renderOpts)
	assert.Success(t, err)
	err = os.MkdirAll(dataPath, 0755)
	assert.Success(t, err)
	err = os.WriteFile(pathGotSVG, svgBytes, 0600)
	assert.Success(t, err)
	defer os.Remove(pathGotSVG)

	var xmlParsed interface{}
	err = xml.Unmarshal(svgBytes, &xmlParsed)
	assert.Success(t, err)

	// We want the visual diffs to compare, but there's floating point precision differences between CI and user machines, so don't compare raw strings
	err = diff.Testdata(filepath.Join(dataPath, "sketch"), ".svg", svgBytes)
	assert.Success(t, err)
}
