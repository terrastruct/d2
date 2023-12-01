//go:build plugins_embed

package d2plugin

func RegisterEmbeddedPlugin(p Plugin) {
	plugins = append(plugins, p)
}
