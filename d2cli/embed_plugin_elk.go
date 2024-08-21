//go:build plugins_embed && plugins_embed_elk

package d2cli

import (
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/internal/embeddedplugin/elk"
)

func init() {
	d2plugin.RegisterEmbeddedPlugin(&elk.ELKPlugin{})
}
