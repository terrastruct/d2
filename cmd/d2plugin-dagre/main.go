//go:build !plugins_embed

package main

import (
	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/internal/embeddedplugin/dagre"
)

func main() {
	xmain.Main(d2plugin.Serve(&dagre.DagrePlugin{}))
}
