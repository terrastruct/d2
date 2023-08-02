//go:build ignore

// utf16_gen.go is used to create test UTF-16 input for the UTF-16 input test in parse_test.go
// Confirm `file utf16.txt` returns
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func main() {
	// Pretend we're on Windows.
	s := "x -> y\r\n"

	b := &bytes.Buffer{}
	t := transform.NewWriter(b, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder())
	_, err := io.WriteString(t, s)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%q\n", b.String())

	err = os.WriteFile("./utf16.d2", b.Bytes(), 0644)
	if err != nil {
		log.Fatal(err)
	}
}
