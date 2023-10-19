package imgbundler

import (
	"context"
	"crypto/rand"
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/cmdlog"
	"oss.terrastruct.com/util-go/xos"

	"oss.terrastruct.com/util-go/xmain"
)

//go:embed test_png.png
var testPNGFile []byte

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestRegex(t *testing.T) {
	urls := []string{
		"https://icons.terrastruct.com/essentials/004-picture.svg",
		"http://icons.terrastruct.com/essentials/004-picture.svg",
	}

	notURLs := []string{
		"hi.png",
		"./cat.png",
		"/cat.png",
	}

	for _, href := range append(urls, notURLs...) {
		str := fmt.Sprintf(`<image href="%s" />`, href)
		matches := imageRegex.FindAllStringSubmatch(str, -1)
		if len(matches) != 1 {
			t.Fatalf("uri regex didn't match %s", str)
		}
	}
}

func TestInlineRemote(t *testing.T) {
	imgCache = sync.Map{}
	ctx := context.Background()
	svgURL := "https://icons.terrastruct.com/essentials/004-picture.svg"
	pngURL := "https://cdn4.iconfinder.com/data/icons/smart-phones-technologies/512/android-phone.png"

	sampleSVG := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<svg
id="d2-svg"
style="background: white;"
xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
width="328" height="587" viewBox="-100 -131 328 587"><style type="text/css">
<![CDATA[
.shape {
  shape-rendering: geometricPrecision;
  stroke-linejoin: round;
}
.connection {
  stroke-linecap: round;
  stroke-linejoin: round;
}

]]>
</style><g id="a"><g class="shape" ><image href="%s" x="0" y="0" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="-15.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">a</text></g><g id="b"><g class="shape" ><image href="%s" x="0" y="228" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="213.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">b</text></g><g id="(a -&gt; b)[0]"><marker id="mk-3990223579" markerWidth="10.000000" markerHeight="12.000000" refX="7.000000" refY="6.000000" viewBox="0.000000 0.000000 10.000000 12.000000" orient="auto" markerUnits="userSpaceOnUse"> <polygon class="connection" fill="#0D32B2" stroke-width="2" points="0.000000,0.000000 10.000000,6.000000 0.000000,12.000000" /> </marker><path d="M 64.000000 130.000000 C 64.000000 168.000000 64.000000 188.000000 64.000000 224.000000" class="connection" style="fill:none;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" marker-end="url(#mk-3990223579)" /></g><style type="text/css"><![CDATA[
.text-bold {
	font-family: "font-bold";
}
@font-face {
	font-family: font-bold;
	src: url("REMOVED");
}]]></style></svg>
`, svgURL, pngURL)

	ms := &xmain.State{
		Name: "test",

		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,

		Env: xos.NewEnv(os.Environ()),
	}
	ms.Log = cmdlog.NewTB(ms.Env, t)

	httpClient.Transport = roundTripFunc(func(req *http.Request) *http.Response {
		respRecorder := httptest.NewRecorder()
		switch req.URL.String() {
		case svgURL:
			respRecorder.WriteString(`<?xml version=\"1.0\" encoding=\"iso-8859-1\"?>\r\n<!-- Generator: Adobe Illustrator 19.0.0, SVG Export Plug-In . SVG Version: 6.00 Build 0)  -->\r\n<svg version=\"1.1\" id=\"Capa_1\" xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" x=\"0px\" y=\"0px\"\r\n\t viewBox=\"0 0 58 58\" style=\"enable-background:new 0 0 58 58;\" xml:space=\"preserve\">\r\n<rect x=\"1\" y=\"7\" style=\"fill:#C3E1ED;stroke:#E7ECED;stroke-width:2;stroke-miterlimit:10;\" width=\"56\" height=\"44\"/>\r\n<circle style=\"fill:#ED8A19;\" cx=\"16\" cy=\"17.569\" r=\"6.569\"/>\r\n<polygon style=\"fill:#1A9172;\" points=\"56,36.111 55,35 43,24 32.5,35.5 37.983,40.983 42,45 56,45 \"/>\r\n<polygon style=\"fill:#1A9172;\" points=\"2,49 26,49 21.983,44.983 11.017,34.017 2,41.956 \"/>\r\n<rect x=\"2\" y=\"45\" style=\"fill:#6B5B4B;\" width=\"54\" height=\"5\"/>\r\n<polygon style=\"fill:#25AE88;\" points=\"37.983,40.983 27.017,30.017 10,45 42,45 \"/>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n</svg>`)
		case pngURL:
			respRecorder.Write(testPNGFile)
		default:
			t.Fatal(req.URL)
		}
		respRecorder.WriteHeader(200)
		return respRecorder.Result()
	})

	out, err := BundleRemote(ctx, []byte(sampleSVG), false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "https://") {
		t.Fatal("links still exist")
	}
	if !strings.Contains(string(out), "image/svg+xml") {
		t.Fatal("no svg image inserted")
	}
	if !strings.Contains(string(out), "image/png") {
		t.Fatal("no png image inserted")
	}

	imgCache = sync.Map{}
	// Test almost too large response
	httpClient.Transport = roundTripFunc(func(req *http.Request) *http.Response {
		respRecorder := httptest.NewRecorder()
		bytes := make([]byte, maxImageSize)
		rand.Read(bytes)
		respRecorder.Write(bytes)
		respRecorder.WriteHeader(200)
		return respRecorder.Result()
	})
	_, err = BundleRemote(ctx, []byte(sampleSVG), false)
	if err != nil {
		t.Fatal(err)
	}

	imgCache = sync.Map{}
	// Test too large response
	httpClient.Transport = roundTripFunc(func(req *http.Request) *http.Response {
		respRecorder := httptest.NewRecorder()
		bytes := make([]byte, maxImageSize+1)
		rand.Read(bytes)
		respRecorder.Write(bytes)
		respRecorder.WriteHeader(200)
		return respRecorder.Result()
	})
	_, err = BundleRemote(ctx, []byte(sampleSVG), false)
	if err == nil {
		t.Fatal("expected error")
	}

	imgCache = sync.Map{}
	// Test error response
	httpClient.Transport = roundTripFunc(func(req *http.Request) *http.Response {
		respRecorder := httptest.NewRecorder()
		respRecorder.WriteHeader(500)
		return respRecorder.Result()
	})
	_, err = BundleRemote(ctx, []byte(sampleSVG), false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInlineLocal(t *testing.T) {
	imgCache = sync.Map{}
	ctx := context.Background()
	svgURL, err := filepath.Abs("./test_svg.svg")
	if err != nil {
		t.Fatal(err)
	}
	pngURL, err := filepath.Abs("./test_png.png")
	if err != nil {
		t.Fatal(err)
	}

	sampleSVG := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<svg
id="d2-svg"
style="background: white;"
xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
width="328" height="587" viewBox="-100 -131 328 587"><style type="text/css">
<![CDATA[
.shape {
  shape-rendering: geometricPrecision;
  stroke-linejoin: round;
}
.connection {
  stroke-linecap: round;
  stroke-linejoin: round;
}

]]>
</style><g id="a"><g class="shape" ><image href="%s" x="0" y="0" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="-15.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">a</text></g><g id="b"><g class="shape" ><image href="%s" x="0" y="228" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="213.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">b</text></g><g id="(a -&gt; b)[0]"><marker id="mk-3990223579" markerWidth="10.000000" markerHeight="12.000000" refX="7.000000" refY="6.000000" viewBox="0.000000 0.000000 10.000000 12.000000" orient="auto" markerUnits="userSpaceOnUse"> <polygon class="connection" fill="#0D32B2" stroke-width="2" points="0.000000,0.000000 10.000000,6.000000 0.000000,12.000000" /> </marker><path d="M 64.000000 130.000000 C 64.000000 168.000000 64.000000 188.000000 64.000000 224.000000" class="connection" style="fill:none;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" marker-end="url(#mk-3990223579)" /></g><style type="text/css"><![CDATA[
.text-bold {
	font-family: "font-bold";
}
@font-face {
	font-family: font-bold;
	src: url("REMOVED");
}]]></style></svg>
`, svgURL, pngURL)

	ms := &xmain.State{
		Name: "test",

		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,

		Env: xos.NewEnv(os.Environ()),
	}
	ms.Log = cmdlog.NewTB(ms.Env, t)
	out, err := BundleLocal(ctx, []byte(sampleSVG), false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), svgURL) {
		t.Fatal("links still exist")
	}
	if !strings.Contains(string(out), "image/svg+xml") {
		t.Fatal("no svg image inserted")
	}
	if !strings.Contains(string(out), "image/png") {
		t.Fatal("no png image inserted")
	}
}

// TestDuplicateURL ensures that we don't fetch the same image twice
func TestDuplicateURL(t *testing.T) {
	imgCache = sync.Map{}
	ctx := context.Background()
	url1 := "https://icons.terrastruct.com/essentials/004-picture.svg"
	url2 := "https://icons.terrastruct.com/essentials/004-picture.svg"

	sampleSVG := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<svg
id="d2-svg"
style="background: white;"
xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
width="328" height="587" viewBox="-100 -131 328 587"><style type="text/css">
<![CDATA[
.shape {
  shape-rendering: geometricPrecision;
  stroke-linejoin: round;
}
.connection {
  stroke-linecap: round;
  stroke-linejoin: round;
}

]]>
</style><g id="a"><g class="shape" ><image href="%s" x="0" y="0" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="-15.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">a</text></g><g id="b"><g class="shape" ><image href="%s" x="0" y="228" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="213.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">b</text></g><g id="(a -&gt; b)[0]"><marker id="mk-3990223579" markerWidth="10.000000" markerHeight="12.000000" refX="7.000000" refY="6.000000" viewBox="0.000000 0.000000 10.000000 12.000000" orient="auto" markerUnits="userSpaceOnUse"> <polygon class="connection" fill="#0D32B2" stroke-width="2" points="0.000000,0.000000 10.000000,6.000000 0.000000,12.000000" /> </marker><path d="M 64.000000 130.000000 C 64.000000 168.000000 64.000000 188.000000 64.000000 224.000000" class="connection" style="fill:none;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" marker-end="url(#mk-3990223579)" /></g><style type="text/css"><![CDATA[
.text-bold {
	font-family: "font-bold";
}
@font-face {
	font-family: font-bold;
	src: url("REMOVED");
}]]></style></svg>
`, url1, url2)

	ms := &xmain.State{
		Name: "test",

		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,

		Env: xos.NewEnv(os.Environ()),
	}
	ms.Log = cmdlog.NewTB(ms.Env, t)

	count := 0

	httpClient.Transport = roundTripFunc(func(req *http.Request) *http.Response {
		count++
		respRecorder := httptest.NewRecorder()
		respRecorder.WriteString(`<?xml version=\"1.0\" encoding=\"iso-8859-1\"?>\r\n<!-- Generator: Adobe Illustrator 19.0.0, SVG Export Plug-In . SVG Version: 6.00 Build 0)  -->\r\n<svg version=\"1.1\" id=\"Capa_1\" xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" x=\"0px\" y=\"0px\"\r\n\t viewBox=\"0 0 58 58\" style=\"enable-background:new 0 0 58 58;\" xml:space=\"preserve\">\r\n<rect x=\"1\" y=\"7\" style=\"fill:#C3E1ED;stroke:#E7ECED;stroke-width:2;stroke-miterlimit:10;\" width=\"56\" height=\"44\"/>\r\n<circle style=\"fill:#ED8A19;\" cx=\"16\" cy=\"17.569\" r=\"6.569\"/>\r\n<polygon style=\"fill:#1A9172;\" points=\"56,36.111 55,35 43,24 32.5,35.5 37.983,40.983 42,45 56,45 \"/>\r\n<polygon style=\"fill:#1A9172;\" points=\"2,49 26,49 21.983,44.983 11.017,34.017 2,41.956 \"/>\r\n<rect x=\"2\" y=\"45\" style=\"fill:#6B5B4B;\" width=\"54\" height=\"5\"/>\r\n<polygon style=\"fill:#25AE88;\" points=\"37.983,40.983 27.017,30.017 10,45 42,45 \"/>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n</svg>`)
		respRecorder.WriteHeader(200)
		return respRecorder.Result()
	})

	out, err := BundleRemote(ctx, []byte(sampleSVG), false)
	if err != nil {
		t.Fatal(err)
	}
	tassert.Equal(t, 1, count)
	if strings.Contains(string(out), url1) {
		t.Fatal("links still exist")
	}
	tassert.Equal(t, 2, strings.Count(string(out), "image/svg+xml"))
}

func TestImgCache(t *testing.T) {
	imgCache = sync.Map{}
	ctx := context.Background()
	url1 := "https://icons.terrastruct.com/essentials/004-picture.svg"
	url2 := "https://icons.terrastruct.com/essentials/004-picture.svg"

	sampleSVG := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<svg
id="d2-svg"
style="background: white;"
xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
width="328" height="587" viewBox="-100 -131 328 587"><style type="text/css">
<![CDATA[
.shape {
  shape-rendering: geometricPrecision;
  stroke-linejoin: round;
}
.connection {
  stroke-linecap: round;
  stroke-linejoin: round;
}

]]>
</style><g id="a"><g class="shape" ><image href="%s" x="0" y="0" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="-15.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">a</text></g><g id="b"><g class="shape" ><image href="%s" x="0" y="228" width="128" height="128" style="fill:#FFFFFF;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" /></g><text class="text-bold" x="64.000000" y="213.000000" style="text-anchor:middle;font-size:16px;fill:#0A0F25">b</text></g><g id="(a -&gt; b)[0]"><marker id="mk-3990223579" markerWidth="10.000000" markerHeight="12.000000" refX="7.000000" refY="6.000000" viewBox="0.000000 0.000000 10.000000 12.000000" orient="auto" markerUnits="userSpaceOnUse"> <polygon class="connection" fill="#0D32B2" stroke-width="2" points="0.000000,0.000000 10.000000,6.000000 0.000000,12.000000" /> </marker><path d="M 64.000000 130.000000 C 64.000000 168.000000 64.000000 188.000000 64.000000 224.000000" class="connection" style="fill:none;stroke:#0D32B2;opacity:1.000000;stroke-width:2;" marker-end="url(#mk-3990223579)" /></g><style type="text/css"><![CDATA[
.text-bold {
	font-family: "font-bold";
}
@font-face {
	font-family: font-bold;
	src: url("REMOVED");
}]]></style></svg>
`, url1, url2)

	ms := &xmain.State{
		Name: "test",

		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,

		Env: xos.NewEnv(os.Environ()),
	}
	ms.Log = cmdlog.NewTB(ms.Env, t)

	count := 0

	httpClient.Transport = roundTripFunc(func(req *http.Request) *http.Response {
		count++
		respRecorder := httptest.NewRecorder()
		respRecorder.WriteString(`<?xml version=\"1.0\" encoding=\"iso-8859-1\"?>\r\n<!-- Generator: Adobe Illustrator 19.0.0, SVG Export Plug-In . SVG Version: 6.00 Build 0)  -->\r\n<svg version=\"1.1\" id=\"Capa_1\" xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" x=\"0px\" y=\"0px\"\r\n\t viewBox=\"0 0 58 58\" style=\"enable-background:new 0 0 58 58;\" xml:space=\"preserve\">\r\n<rect x=\"1\" y=\"7\" style=\"fill:#C3E1ED;stroke:#E7ECED;stroke-width:2;stroke-miterlimit:10;\" width=\"56\" height=\"44\"/>\r\n<circle style=\"fill:#ED8A19;\" cx=\"16\" cy=\"17.569\" r=\"6.569\"/>\r\n<polygon style=\"fill:#1A9172;\" points=\"56,36.111 55,35 43,24 32.5,35.5 37.983,40.983 42,45 56,45 \"/>\r\n<polygon style=\"fill:#1A9172;\" points=\"2,49 26,49 21.983,44.983 11.017,34.017 2,41.956 \"/>\r\n<rect x=\"2\" y=\"45\" style=\"fill:#6B5B4B;\" width=\"54\" height=\"5\"/>\r\n<polygon style=\"fill:#25AE88;\" points=\"37.983,40.983 27.017,30.017 10,45 42,45 \"/>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n<g>\r\n</g>\r\n</svg>`)
		respRecorder.WriteHeader(200)
		return respRecorder.Result()
	})

	// Using a cache, imgs are not refetched on multiple runs
	_, err := BundleRemote(ctx, []byte(sampleSVG), true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = BundleRemote(ctx, []byte(sampleSVG), true)
	if err != nil {
		t.Fatal(err)
	}
	tassert.Equal(t, 1, count)

	// With cache disabled, it refetches
	count = 0
	_, err = BundleRemote(ctx, []byte(sampleSVG), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = BundleRemote(ctx, []byte(sampleSVG), false)
	if err != nil {
		t.Fatal(err)
	}
	tassert.Equal(t, 2, count)
}
