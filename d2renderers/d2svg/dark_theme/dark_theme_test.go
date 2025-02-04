package dark_theme_test

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
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func TestDarkTheme(t *testing.T) {
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
  style.fill: "#cba6f7"
  style.font-color: "#000000"
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
  style.fill: "#fab387"
  style.font-color: "#000000"
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
  # style.fill: "#c1a2f3"
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
  style.fill: "#eba0ac"
  style.font-color: "#000000"
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
  style.stroke: "#b4befe"
  style.fill: "#000000"
  crmixer: CrMixer {
    style.fill: "#11111b"
	style.font-color: "#cdd6f4"
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
			name: "code",
			script: `code: |go
func main() {
  panic("TODO")
}
|

text: |md
Five is a sufficiently close approximation to infinity.
|
unknown: |asdf
Don't hit me!!  I'm in the Twilight Zone!!!
|
code -- unknown
`,
		},
	}
	runa(t, tcs)
}

type testCase struct {
	name   string
	script string
	skip   bool
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

	renderOpts := &d2svg.RenderOpts{
		ThemeID: go2.Pointer(int64(200)),
	}
	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}

	diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
		Ruler:          ruler,
		LayoutResolver: layoutResolver,
		FontFamily:     go2.Pointer(d2fonts.HandDrawn),
	}, renderOpts)
	if !tassert.Nil(t, err) {
		return
	}

	dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestDarkTheme/"))
	pathGotSVG := filepath.Join(dataPath, "dark_theme.got.svg")

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

	err = diff.Testdata(filepath.Join(dataPath, "dark_theme"), ".svg", svgBytes)
	assert.Success(t, err)
}
