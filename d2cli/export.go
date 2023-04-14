package d2cli

import (
	"fmt"
	"path/filepath"
)

type exportExtension string

const GIF exportExtension = ".gif"
const PNG exportExtension = ".png"
const PPTX exportExtension = ".pptx"
const PDF exportExtension = ".pdf"
const SVG exportExtension = ".svg"

var SUPPORTED_EXTENSIONS = []exportExtension{SVG, PNG, PDF, PPTX, GIF}

func getExportExtension(outputPath string) (exportExtension, error) {
	ext := filepath.Ext(outputPath)
	if ext == ".ppt" {
		return "", fmt.Errorf("D2 does not support ppt exports, did you mean \"pptx\"?")
	}
	for _, kext := range SUPPORTED_EXTENSIONS {
		if kext == exportExtension(ext) {
			return exportExtension(ext), nil
		}
	}
	// default is svg
	return exportExtension(SVG), nil
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
