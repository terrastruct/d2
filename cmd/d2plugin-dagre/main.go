//go:build !nodagre

package main

import (
	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2plugin"
)

func main() {
	xmain.Main(d2plugin.Serve(&d2plugin.DagrePlugin))
}
