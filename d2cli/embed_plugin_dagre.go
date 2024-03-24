//go:build plugins_embed && plugins_embed_dagre

package d2cli

import (
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/internal/embeddedplugin/dagre"
)

func init() {
	d2plugin.RegisterEmbeddedPlugin(&dagre.DagrePlugin{})
}
