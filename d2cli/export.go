package d2cli

import (
	"path/filepath"
)

type exportExtension string

const GIF = ".gif"
const PNG = ".png"
const PPTX = ".pptx"
const PDF = ".pdf"
const SVG = ".svg"

var KNOWN_EXTENSIONS = []string{SVG, PNG, PDF, PPTX, GIF}

func getExportExtension(outputPath string) exportExtension {
	ext := filepath.Ext(outputPath)
	for _, kext := range KNOWN_EXTENSIONS {
		if kext == ext {
			return exportExtension(ext)
		}
	}
	// default is svg
	return exportExtension(SVG)
}

func (ex exportExtension) supportsAnimation() bool {
	return ex == SVG || ex == GIF
}

func (ex exportExtension) requiresPNGRenderer() bool {
	return ex == PNG || ex == PDF || ex == PPTX || ex == GIF
}

func (ex exportExtension) supportsDarkTheme() bool {
	return ex == SVG
}
