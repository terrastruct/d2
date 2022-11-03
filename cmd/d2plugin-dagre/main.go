//go:build cgo && !nodagre

package main

import (
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/lib/xmain"
)

func main() {
	xmain.Main(d2plugin.Serve(d2plugin.DagrePlugin))
}
