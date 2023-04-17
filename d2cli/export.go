package d2cli

import (
	"path/filepath"
)

type exportExtension string

const GIF exportExtension = ".gif"
const PNG exportExtension = ".png"
const PPTX exportExtension = ".pptx"
const PPT exportExtension = ".ppt"
const PDF exportExtension = ".pdf"
const SVG exportExtension = ".svg"

var SUPPORTED_EXTENSIONS = []exportExtension{SVG, PNG, PDF, PPTX, GIF, PPT}

func getExportExtension(outputPath string) exportExtension {
	ext := filepath.Ext(outputPath)
	for _, kext := range SUPPORTED_EXTENSIONS {
		if kext == exportExtension(ext) {
			return exportExtension(ext)
		}
	}
	// default is svg
	return exportExtension(SVG)
}

func (ex exportExtension) supportsAnimation() bool {
	return ex == SVG || ex == GIF
}

func (ex exportExtension) requiresAnimationInterval() bool {
	return ex == GIF
}

func (ex exportExtension) requiresPNGRenderer() bool {
	return ex == PNG || ex == PDF || ex == PPTX || ex == GIF
}

func (ex exportExtension) supportsDarkTheme() bool {
	return ex == SVG
}
