package d2format_test

import (
	"strings"
	"testing"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2format"
)

func TestFormatKeyPath(t *testing.T) {
	values := []string{"", "a", "a_b", "A0", "_", "null", "NULL", "nUlL", "suspend", "unsuspend", "true", "a b", " a", "a ", "a.b", "a-b", "a--b", "a-", "-a", "a\\b", "a\nb", "a\rb", "a\tb", "a\"b", "a'b", "'a", "\"a", "|a", "&a", "$a", "${a}", "*", "**", "***", "a: b", "世界", "é", "\u00a0a", "a\u2000", string([]byte{0xff}), "x\x00y"}
	for reserved := range d2ast.ReservedKeywords {
		values = append(values, reserved, strings.ToUpper(reserved))
	}
	check := func(path []d2ast.String) {
		t.Helper()
		want := d2format.Format(d2ast.MakeKeyPathString(path))
		if got := d2format.FormatKeyPath(path); got != want {
			t.Fatalf("path %q: got %q, want %q", path, got, want)
		}
	}
	check(nil)
	for _, a := range values {
		for _, b := range values {
			// Input quoting and string node type must not affect canonical IDs.
			check([]d2ast.String{d2ast.FlatUnquotedString(a), d2ast.FlatDoubleQuotedString(b), &d2ast.SingleQuotedString{Value: a}})
		}
	}
}

func FuzzFormatKeyPath(f *testing.F) {
	for _, seed := range []string{"", "a.b", "NULL", "style.fill", "${a}", "世界", " a\n\"b", "x\xffy"} {
		f.Add(seed, "STYLE", "a_b")
	}
	f.Fuzz(func(t *testing.T, a, b, c string) {
		path := []d2ast.String{d2ast.FlatUnquotedString(a), d2ast.FlatDoubleQuotedString(b), &d2ast.SingleQuotedString{Value: c}}
		want := d2format.Format(d2ast.MakeKeyPathString(path))
		if got := d2format.FormatKeyPath(path); got != want {
			t.Fatalf("path %q: got %q, want %q", []string{a, b, c}, got, want)
		}
	})
}

func TestFormatKeyPathCustomizedSpecials(t *testing.T) {
	previous := d2ast.UnquotedKeySpecials
	t.Cleanup(func() { d2ast.UnquotedKeySpecials = previous })
	for _, specials := range []string{previous + "x", previous + "N", previous + "_", "a012", ""} {
		d2ast.UnquotedKeySpecials = specials
		path := []d2ast.String{d2ast.FlatUnquotedString("x"), d2ast.FlatUnquotedString("NULL"), d2ast.FlatUnquotedString("a_b"), d2ast.FlatUnquotedString("A0")}
		want := d2format.Format(d2ast.MakeKeyPathString(path))
		if got := d2format.FormatKeyPath(path); got != want {
			t.Fatalf("specials %q: got %q, want %q", specials, got, want)
		}
	}
}
