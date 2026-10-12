package d2cli

import (
	"fmt"
	"path/filepath"
	"strings"
)

type exportExtension string

const GIF exportExtension = ".gif"
const PNG exportExtension = ".png"
const PPTX exportExtension = ".pptx"
const PDF exportExtension = ".pdf"
const SVG exportExtension = ".svg"
const TXT exportExtension = ".txt"

// Internal selectors keep the opt-in isometric renderer separate from 2D exports.
const isometricSVG exportExtension = "isometric-svg"
const isometricPNG exportExtension = "isometric-png"
const isometricGIF exportExtension = "isometric-gif"
const isometricPDF exportExtension = "isometric-pdf"
const isometricPPTX exportExtension = "isometric-pptx"

func (ex exportExtension) isIsometric() bool {
	return ex == isometricSVG || ex == isometricPNG || ex == isometricGIF || ex == isometricPDF || ex == isometricPPTX
}

var SUPPORTED_EXTENSIONS = []exportExtension{SVG, PNG, PDF, PPTX, GIF, TXT}

var STDOUT_FORMAT_MAP = map[string]exportExtension{
	"png":   PNG,
	"svg":   SVG,
	"ascii": TXT,
	"txt":   TXT,
	"pdf":   PDF,
	"pptx":  PPTX,
	"gif":   GIF,
}

var SUPPORTED_STDOUT_FORMATS = []string{"png", "svg", "ascii", "txt", "pdf", "pptx", "gif"}

func getOutputFormat(stdoutFormatFlag *string, outputPath string) (exportExtension, error) {
	if stdoutFormatFlag != nil && *stdoutFormatFlag != "" {
		format := strings.ToLower(*stdoutFormatFlag)
		if ext, ok := STDOUT_FORMAT_MAP[format]; ok {
			return ext, nil
		}
		return "", fmt.Errorf("%s is not a supported format. Supported formats are: %s", *stdoutFormatFlag, SUPPORTED_STDOUT_FORMATS)
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
	return ex == SVG || ex == GIF || ex == isometricGIF
}

func (ex exportExtension) supportsDarkTheme() bool {
	// Preserve a requested isometric dark theme until its renderer can issue a
	// clear unsupported-feature error, rather than silently stripping the flag.
	return ex == SVG || ex == isometricSVG
}
