package png

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	_ "embed"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	pngstruct "github.com/dsoprea/go-png-image-structure/v2"
	"github.com/playwright-community/playwright-go"

	"oss.terrastruct.com/d2/lib/version"
)

// ConvertSVG scales the image by 2x
const SCALE = 2.

type Playwright struct {
	PW      *playwright.Playwright
	Browser playwright.Browser
	Page    playwright.Page
}

func (pw *Playwright) RestartBrowser() (Playwright, error) {
	if err := pw.Browser.Close(); err != nil {
		return Playwright{}, fmt.Errorf("failed to close Playwright browser: %w", err)
	}
	return startPlaywright(pw.PW)
}

func (pw *Playwright) Cleanup() error {
	if err := pw.Browser.Close(); err != nil {
		return fmt.Errorf("failed to close Playwright browser: %w", err)
	}
	if err := pw.PW.Stop(); err != nil {
		return fmt.Errorf("failed to stop Playwright: %w", err)
	}
	return nil
}

func startPlaywright(pw *playwright.Playwright) (Playwright, error) {
	browser, err := pw.Chromium.Launch()
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to launch Chromium: %w", err)
	}
	context, err := browser.NewContext()
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to start new Playwright browser context: %w", err)
	}
	page, err := context.NewPage()
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to start new Playwright page: %w", err)
	}
	return Playwright{
		PW:      pw,
		Browser: browser,
		Page:    page,
	}, nil
}

func InitPlaywright() (Playwright, error) {
	err := playwright.Install(&playwright.RunOptions{
		Verbose:  false,
		Browsers: []string{"chromium"},
	})
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to install Playwright: %w", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to run Playwright: %w", err)
	}
	return startPlaywright(pw)
}

func InitPlaywrightWithPrompt() (Playwright, error) {
	fmt.Print("D2 needs to install Chromium v130.0.6723.19 to render images. Continue? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to read user input: %w", err)
	}
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		return Playwright{}, fmt.Errorf("chromium installation cancelled by user")
	}

	return InitPlaywright()
}

//go:embed generate_png.js
var genPNGScript string

const pngPrefix = "data:image/png;base64,"

// ConvertSVG converts the given SVG into a PNG.
// Note that the resulting PNG has 2x the size (width and height) of the original SVG (see generate_png.js)
func ConvertSVG(page playwright.Page, svg []byte) ([]byte, error) {
	encodedSVG := base64.StdEncoding.EncodeToString(svg)
	pngInterface, err := page.Evaluate(genPNGScript, map[string]interface{}{
		"imgString": "data:image/svg+xml;charset=utf-8;base64," + encodedSVG,
		"scale":     int(SCALE),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate png: %w", err)
	}

	pngString := pngInterface.(string)
	if !strings.HasPrefix(pngString, pngPrefix) {
		if len(pngString) > 50 {
			pngString = pngString[0:50] + "..."
		}
		return nil, fmt.Errorf("invalid PNG: %q", pngString)
	}
	splicedPNGString := pngString[len(pngPrefix):]
	return base64.StdEncoding.DecodeString(splicedPNGString)
}

func AddExif(png []byte) ([]byte, error) {
	// https://pkg.go.dev/github.com/dsoprea/go-png-image-structure/v2?utm_source=godoc#example-ChunkSlice.SetExif
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, err
	}

	ti := exif.NewTagIndex()

	ib := exif.NewIfdBuilder(im, ti, exifcommon.IfdStandardIfdIdentity, exifcommon.TestDefaultByteOrder)

	err = ib.AddStandardWithName("Make", "D2")
	if err != nil {
		return nil, err
	}

	err = ib.AddStandardWithName("Model", version.Version)
	if err != nil {
		return nil, err
	}

	pmp := pngstruct.NewPngMediaParser()
	intfc, err := pmp.ParseBytes(png)
	if err != nil {
		return nil, err
	}
	cs := intfc.(*pngstruct.ChunkSlice)
	err = cs.SetExif(ib)
	if err != nil {
		return nil, err
	}
	b := new(bytes.Buffer)
	err = cs.WriteTo(b)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
