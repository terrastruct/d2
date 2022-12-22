package d2sketch_test

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cdr.dev/slog"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
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
			name: "connection label",
			script: `a -> b: hello
		`,
		},
		{
			name: "chess",
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
  fill: "#c1a2f3"
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
  fill: "#ffdef1"
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
  # fill: "#c1a2f3"
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
  fill: "#ffdef1"
}
manhattan: Manhattan
memcache: Memcache {
  icon: https://d1q6f0aelx0por.cloudfront.net/product-logos/de041504-0ddb-43f6-b89e-fe04403cca8d-memcached.png
}

fetch: Fetch {
  multiple: true
  shape: step
}
feature: Feature {
  multiple: true
  shape: step
}
scoring: Scoring {
  multiple: true
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
	ctx = log.WithTB(ctx, t, nil)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	ruler, err := textmeasure.NewRuler()
	if !tassert.Nil(t, err) {
		return
	}

	diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
		Ruler:      ruler,
		ThemeID:    0,
		Layout:     d2dagrelayout.Layout,
		FontFamily: go2.Pointer(d2fonts.HandDrawn),
	})
	if !tassert.Nil(t, err) {
		return
	}

	dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestSketch/"))
	pathGotSVG := filepath.Join(dataPath, "sketch.got.svg")

	svgBytes, err := d2svg.Render(diagram, &d2svg.RenderOpts{
		Pad:    d2svg.DEFAULT_PADDING,
		Sketch: true,
	})
	assert.Success(t, err)
	err = os.MkdirAll(dataPath, 0755)
	assert.Success(t, err)
	err = ioutil.WriteFile(pathGotSVG, svgBytes, 0600)
	assert.Success(t, err)
	defer os.Remove(pathGotSVG)

	var xmlParsed interface{}
	err = xml.Unmarshal(svgBytes, &xmlParsed)
	assert.Success(t, err)

	err = diff.Testdata(filepath.Join(dataPath, "sketch"), ".svg", svgBytes)
	assert.Success(t, err)
}
