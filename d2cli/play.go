package d2cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"oss.terrastruct.com/d2/lib/urlenc"
	"oss.terrastruct.com/util-go/xbrowser"
	"oss.terrastruct.com/util-go/xmain"
)

func playCmd(ctx context.Context, ms *xmain.State) error {
	if len(ms.Opts.Flags.Args()) != 2 {
		return xmain.UsageErrorf("play must be passed one argument: either a filepath or '-' for stdin")
	}
	filepath := ms.Opts.Flags.Args()[1]

	theme, err := ms.Opts.Flags.GetInt64("theme")
	if err != nil {
		return err
	}

	sketch, err := ms.Opts.Flags.GetBool("sketch")
	if err != nil {
		return err
	}

	var sketchNumber int
	if sketch {
		sketchNumber = 1
	} else {
		sketchNumber = 0
	}

	fileRaw, err := readInput(filepath)
	if err != nil {
		return err
	}

	encoded, err := urlenc.Encode(fileRaw)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://play.d2lang.com/?script=%s&sketch=%d&theme=%d&", encoded, sketchNumber, theme)
	openBrowser(ctx, ms, url)
	return nil
}

func readInput(filepath string) (string, error) {
	if filepath == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("error reading from stdin: %w", err)
		}
		return string(data), nil
	}

	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", xmain.UsageErrorf("%s", err.Error())
	}
	return string(data), nil
}

func openBrowser(ctx context.Context, ms *xmain.State, url string) {
	ms.Log.Info.Printf("opening playground: %s", url)

	err := xbrowser.Open(ctx, ms.Env, url)
	if err != nil {
		ms.Log.Warn.Printf("failed to open browser to %v: %v", url, err)
	}
}
