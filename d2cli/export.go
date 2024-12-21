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

func getOutputFormat(formatFlag *string, outputPath string) (exportExtension, error) {
	var formatMap = map[string]exportExtension{
		"png":  PNG,
		"svg":  SVG,
		"pdf":  PDF,
		"pptx": PPTX,
		"gif":  GIF,
	}

	if *formatFlag != "" {
		if format, ok := formatMap[*formatFlag]; ok {
			return format, nil
		}
		return "", fmt.Errorf("unsupported format: %s", *formatFlag)
	}
	return getExportExtension(outputPath), nil
}

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
