package d2cli

import (
	"fmt"
	"os"

	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2renderers"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/ast"
	"oss.terrastruct.com/d2/lib/fs"
	"oss.terrastruct.com/d2/lib/shell"
)

func Run(args []string) error {
	if len(args)!= 2 {
		return fmt.Errorf("usage: d2 <input.d2> <output.svg>")
	}

	inputPath := args[0]
	outputPath := args[1]

	src, err := os.ReadFile(inputPath)
	if err!= nil {
		return fmt.Errorf("failed to read input file: %v", err)
	}

	board, err := d2parser.Parse(string(src))
	if err!= nil {
		return err
	}

	themeID := 300
	theme, err := d2themes.GetTheme(themeID)
	if err!= nil {
		return err
	}
	board.Theme = theme

	svg, err := d2renderers.RenderSVG(board)
	if err!= nil {
		return err
	}

	err = os.WriteFile(outputPath, []byte(svg), 0o644)
	if err!= nil {
		return fmt.Errorf("failed to write output file: %v", err)
	}

	fmt.Printf("Wrote %s\n", outputPath)

	if fs.WatchingEnabled {
		err := shell.OpenInBrowser(outputPath)
		if err!= nil {
			return fmt.Errorf("failed to open in browser: %v", err)
		}
		err = fs.Watch(inputPath, outputPath, func() error {
			newSrc, err := os.ReadFile(inputPath)
			if err!= nil {
				return fmt.Errorf("failed to read input file: %v", err)
			}

			newBoard, err := d2parser.Parse(string(newSrc))
			if err!= nil {
				return err
			}
			newBoard.Theme = theme

			newSVG, err := d2renderers.RenderSVG(newBoard)
			if err!= nil {
				return err
			}

			return os.WriteFile(outputPath, []byte(newSVG), 0o644)
		})
		if err!= nil {
			return fmt.Errorf("failed to watch: %v", err)
		}
	}

	return nil
}
