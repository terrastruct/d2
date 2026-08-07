//go:build !nodagre

package main

import (
	"github.com/d2lang/util-go/xmain"

	"oss.terrastruct.com/d2/d2plugin"
)

func main() {
	xmain.Main(d2plugin.Serve(&d2plugin.DagrePlugin))
}
