package d2cli

import "path/filepath"

type exportExtension string

var KNOWN_EXTENSIONS = []string{".svg", ".png", ".pptx", ".pdf"}

func getExportExtension(outputPath string) exportExtension {
	ext := filepath.Ext(outputPath)
	for _, kext := range KNOWN_EXTENSIONS {
		if kext == ext {
			return exportExtension(ext)
		}
	}
	// default is svg
	return exportExtension(".svg")
}

func (ex exportExtension) supportsAnimation() bool {
	return ex == ".svg"
}

func (ex exportExtension) requiresPngRenderer() bool {
	return ex == ".png" || ex == ".pdf" || ex == ".pptx"
}

func (ex exportExtension) supportsDarkTheme() bool {
	return ex == ".svg"
}
