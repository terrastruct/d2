package d2chaos_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2chaos"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// usage: D2_CHAOS_MAXI=100 D2_CHAOS_N=100 ./ci/test.sh ./d2chaos
//
// D2_CHAOS_MAXI controls the number of iterations that the dsl generator
// should go through to generate each input D2. It's roughly equivalent to
// the complexity level of each input D2.
//
// D2_CHAOS_N controls the number of D2 texts to generate and run the full
// D2 flow on.
//
// All generated texts are stored in ./out/<n>.d2 and also ./out/<n>.d2.goenc
// The goenc version is the text encoded as a Go string. It lets you replay
// a test by adding it to testPinned below as you can just copy paste the go
// string in.
//
// If D2Chaos fails on CI and you need to investigate the input text that caused the
// failure, all generated texts will be available in the d2chaos-test and d2chaos-race
// github actions artifacts.
func TestD2Chaos(t *testing.T) {
	t.Parallel()

	const outDir = "out"
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("writing generated files to %s", outDir)

	t.Run("pinned", func(t *testing.T) {
		testPinned(t, outDir)
	})

	n := 1
	if os.Getenv("D2_CHAOS_N") != "" {
		envn, err := strconv.Atoi(os.Getenv("D2_CHAOS_N"))
		if err != nil {
			t.Errorf("failed to atoi $D2_CHAOS_N: %v", err)
		} else {
			n = envn
		}
	}

	maxi := 10
	if os.Getenv("D2_CHAOS_MAXI") != "" {
		envMaxi, err := strconv.Atoi(os.Getenv("D2_CHAOS_MAXI"))
		if err != nil {
			t.Errorf("failed to atoi $D2_CHAOS_MAXI: %v", err)
		} else {
			maxi = envMaxi
		}
	}

	for i := 0; i < n; i++ {
		i := i
		t.Run("", func(t *testing.T) {
			t.Parallel()

			text, err := d2chaos.GenDSL(maxi)
			if err != nil {
				t.Fatal(err)
			}

			textPath := filepath.Join(outDir, fmt.Sprintf("%d.d2", i))
			test(t, textPath, text)
		})
	}
}

func test(t *testing.T, textPath, text string) {
	t.Logf("writing d2 to %v (%d bytes)", textPath, len(text))
	err := ioutil.WriteFile(textPath, []byte(text), 0644)
	if err != nil {
		t.Fatal(err)
	}

	goencText := fmt.Sprintf("%#v", text)
	t.Logf("writing d2.goenc to %v (%d bytes)", textPath+".goenc", len(goencText))
	err = ioutil.WriteFile(textPath+".goenc", []byte(goencText), 0644)
	if err != nil {
		t.Fatal(err)
	}

	g, err := d2compiler.Compile("", strings.NewReader(text), nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("layout", func(t *testing.T) {
		defer func() {
			r := recover()
			if r != nil {
				t.Errorf("recovered layout engine panic: %#v\n%s", r, debug.Stack())
			}
		}()

		ctx := log.WithTB(context.Background(), t, nil)

		ruler, err := textmeasure.NewRuler()
		assert.Nil(t, err)

		err = g.SetDimensions(nil, ruler)
		assert.Nil(t, err)

		err = d2dagrelayout.Layout(ctx, g)
		if err != nil {
			t.Fatal(err)
		}

		_, err = d2exporter.Export(ctx, g, 0)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func testPinned(t *testing.T, outDir string) {
	t.Parallel()

	outDir = filepath.Join(outDir, t.Name())
	err := os.MkdirAll(outDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("writing generated files to %v", outDir)

	testCases := []struct {
		name string
		text string
	}{
		{
			name: "internal class edge",
			text: "a: {\n shape: class\n  b -> c\n }",
		},
		{
			name: "table edge order",
			text: "B: {shape: sql_table}\n B.C -- D\n B.C -- D\n D -- B.C\n B -- D\n A -- D",
		},
		{
			name: "child to container edge",
			text: "a.b -> a",
		},
		{
			name: "sequence",
			text: "a: {shape: step}\nb: {shape: step}\na -> b\n",
		},
		{
			name: "orientation",
			text: "a: {\n  b\n  c\n }\n  a <- a.c\n  a.b -> a\n",
		},
		{
			name: "panic",
			text: "\"y\x10e5k%|\": {\n  \"\x11Do\b:\": '\"6d7\\n+u8B\u0080\x18M\x15kb\x12ZPn~\x13\x16/*[y\x03\ffR\u007fsN[w\vb\x1c>\x04\x1dj\\\"&bD\x1eK(D@t,:=L\\\"7\x1f\\n\rcXsnlc\x1c\\n)7RJ\\n\\$t5Mer\u007fX&a\x16A\x0f q4!\u0080\x0e|ym9l\\n\x1aP\ag\x19\x02S,[EU0\x19^\\$(\beB\x18l\x11;A\a>@0\x1d0N\u007f\fV\x1aHSA^\\\"Yn)NYb[Wh7Zhbl\x11\x06vb,^\f\x1e\x144oePF\x15\vt eE.q\x1fG''=~o\x16Q|7oM\x17\b\v(\rX\\\"\x17\x0e@V*E3e\f\x11\x1a-\x1ecccW+:\x1a\x03G\x133Lna:\x06\x1b.\x18M(ScF,!l.HE|E\x0e6-4\x15bi\x0f3!o\x1bt\"'\n  \"#2>rM\x18I\\$\": {\n    shape: document\n    \"\x161%~;H\bfiBG\": {shape: rectangle}\n  }\n  \"1U5^b/6*\x1b2QCc97\\\"\\n>0\x1emc\\n(n87\x03+\t\": '\"KoJ-R\x02xhcG\x17hhb?l)\x15V\x19\v#\x19o)E\x15\a\x12#,\x13?(,h=@?L\x1b\x11\x18,Eu4. eL]b\\\"W(,.A+p&[Z&\\n!)\x16\x0eS7\f\vw0\x02\"' {shape: code}\n  \"1U5^b/6*\x1b2QCc97\\\"\\n>0\x1emc\\n(n87\x03+\t\" <-> \"\x11Do\b:\"\n}\n\"h#\x06z\x0e5c\u0080g~C([\b:\x12H%D\x1c\x18s\x1fog.^oA>\": '\"v\\nm]\x1c\u00809umD\x17YDQ\x1d/)\\nt[i!6<)r?P\x19<F\x10vxB\\n,B]x\f\u0080''\r\v\x0f\x17dc](\t\\nH4^0>\bJ\f\x1c\x12j\x1dTb-]XFuC5KR|q4IwR[@7\x17\x18\x1b\x10 y\x14\aTf\x01!id\bY\x1bosZ8G;~.\u007fKj=Ne2Lum\b\x18]\\n\x1dj|[CvZ#n=kA=0=\x11)\"' {shape: text}\n\"h#\x06z\x0e5c\u0080g~C([\b:\x12H%D\x1c\x18s\x1fog.^oA>\" <-> \"y\x10e5k%|\": '\"\t]2J\x14-\x185\aYVUN\\n7bJ\aC^\x14R9<>\x0eK\x04fD7*7\x06U~\x114\\\"\u00806\"'\n",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			textPath := filepath.Join(outDir, fmt.Sprintf("%s.d2", tc.name))
			test(t, textPath, tc.text)
		})
	}
}
