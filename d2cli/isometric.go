package d2cli

import (
	"fmt"

	"github.com/d2lang/util-go/xmain"
)

// Keep an unset mode distinct from an explicit false so embedded D2 config has
// exactly the same precedence as sketch: flags, then environment, then source.
func resolveIsometricFlag(ms *xmain.State) (*bool, error) {
	isometric, err := ms.Opts.Flags.GetBool("isometric")
	if err != nil {
		return nil, err
	}
	if ms.Opts.Flags.Changed("isometric") || ms.Env.Getenv("D2_ISOMETRIC") != "" {
		return &isometric, nil
	}
	return nil, nil
}

func isometricExportExtension(ext exportExtension) (exportExtension, error) {
	switch ext {
	case SVG, isometricSVG:
		return isometricSVG, nil
	case PNG, isometricPNG:
		return isometricPNG, nil
	case GIF, isometricGIF:
		return isometricGIF, nil
	case PDF, isometricPDF:
		return isometricPDF, nil
	case PPTX, isometricPPTX:
		return isometricPPTX, nil
	default:
		return "", fmt.Errorf("--isometric exports SVG, PNG, GIF, PDF or PPTX; use a matching filename or --stdout-format")
	}
}
