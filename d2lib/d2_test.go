package d2lib

import (
	"context"
	"testing"

	"github.com/d2lang/d2/lib/textmeasure"
)

func TestGetLayoutDoesNotUseEnvironmentFallback(t *testing.T) {
	t.Setenv("D2_LAYOUT", "dagre")

	_, err := getLayout(&CompileOptions{})
	if err == nil || err.Error() != "no available layout" {
		t.Fatalf("getLayout() error = %v, want no available layout", err)
	}
}

func TestCompileRequiresLayoutResolver(t *testing.T) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = Compile(context.Background(), "x", &CompileOptions{Ruler: ruler}, nil)
	const want = `no layout resolver configured for layout engine "dagre"`
	if err == nil || err.Error() != want {
		t.Fatalf("Compile() error = %v, want %q", err, want)
	}
}
