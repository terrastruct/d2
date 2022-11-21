package png

import (
	"encoding/base64"
	"fmt"
	"strings"

	_ "embed"

	"github.com/playwright-community/playwright-go"

	"oss.terrastruct.com/d2/lib/xmain"
)

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
	err := playwright.Install(&playwright.RunOptions{Verbose: false})
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to install Playwright: %w", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to run Playwright: %w", err)
	}
	return startPlaywright(pw)
}

//go:embed generate_png.js
var genPNGScript string

const pngPrefix = "data:image/png;base64,"

func ConvertSVG(ms *xmain.State, page playwright.Page, svg []byte) ([]byte, error) {
	encodedSVG := base64.StdEncoding.EncodeToString(svg)
	pngInterface, err := page.Evaluate(genPNGScript, "data:image/svg+xml;charset=utf-8;base64,"+encodedSVG)
	if err != nil {
		return nil, fmt.Errorf("failed to generate png: %w\nplease report this issue here: https://github.com/terrastruct/d2/issues/new", err)
	}

	pngString := pngInterface.(string)
	if !strings.HasPrefix(pngString, pngPrefix) {
		if len(pngString) > 50 {
			pngString = pngString[0:50] + "..."
		}
		return nil, fmt.Errorf("invalid PNG: %q\nplease report this issue here: https://github.com/terrastruct/d2/issues/new", pngString)
	}
	splicedPNGString := pngString[len(pngPrefix):]
	return base64.StdEncoding.DecodeString(splicedPNGString)
}
