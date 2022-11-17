package png

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	_ "embed"

	"github.com/playwright-community/playwright-go"
	"oss.terrastruct.com/d2/lib/xmain"
)

type Playwright struct {
	PW             *playwright.Playwright
	Browser        playwright.Browser
	BrowserContext playwright.BrowserContext
	Page           playwright.Page
}

func (pw *Playwright) RestartBrowser() (newPW Playwright, err error) {
	if err = pw.Browser.Close(); err != nil {
		return Playwright{}, err
	}
	browser, err := pw.PW.Chromium.Launch()
	if err != nil {
		return Playwright{}, err
	}
	context, err := browser.NewContext()
	if err != nil {
		return Playwright{}, err
	}
	page, err := context.NewPage()
	if err != nil {
		return Playwright{}, err
	}
	return Playwright{
		PW:             pw.PW,
		Browser:        browser,
		BrowserContext: context,
		Page:           page,
	}, nil
}

func (pw *Playwright) Cleanup() (err error) {
	if err = pw.Browser.Close(); err != nil {
		return err
	}
	if err = pw.PW.Stop(); err != nil {
		return err
	}
	return nil
}

func InitPlaywright() (Playwright, error) {
	// check if playwright driver/browsers are installed and up to date
	// https://github.com/playwright-community/playwright-go/blob/8e8f670b5fa7ba5365ae4bfc123fea4aac359763/run.go#L64.
	driver, err := playwright.NewDriver(&playwright.RunOptions{})
	if err != nil {
		return Playwright{}, err
	}
	if _, err := os.Stat(driver.DriverBinaryLocation); errors.Is(err, os.ErrNotExist) {
		err = playwright.Install()
		if err != nil {
			return Playwright{}, err
		}
	} else if err == nil {
		cmd := exec.Command(driver.DriverBinaryLocation, "--version")
		output, err := cmd.Output()
		if err != nil || !bytes.Contains(output, []byte(driver.Version)) {
			err = playwright.Install()
			if err != nil {
				return Playwright{}, err
			}
		}
	}

	pw, err := playwright.Run()
	if err != nil {
		return Playwright{}, err
	}
	browser, err := pw.Chromium.Launch()
	if err != nil {
		return Playwright{}, err
	}
	context, err := browser.NewContext()
	if err != nil {
		return Playwright{}, err
	}
	page, err := context.NewPage()
	if err != nil {
		return Playwright{}, err
	}
	return Playwright{
		PW:             pw,
		Browser:        browser,
		BrowserContext: context,
		Page:           page,
	}, nil
}

//go:embed generate_png.js
var genPNGScript string

func ExportPNG(ms *xmain.State, page playwright.Page, svg []byte) (outputImage []byte, err error) {
	if page == nil {
		return nil, fmt.Errorf("Playwright was not initialized properly for PNG export")
	}

	encodedSVG := base64.StdEncoding.EncodeToString(svg)
	pngInterface, err := page.Evaluate(genPNGScript, "data:image/svg+xml;charset=utf-8;base64,"+encodedSVG)
	if err != nil {
		return nil, err
	}

	pngString := fmt.Sprintf("%v", pngInterface)
	pngPrefix := "data:image/png;base64,"
	if !strings.HasPrefix(pngString, pngPrefix) {
		if len(pngString) > 50 {
			pngString = pngString[0:50] + "..."
		}
		return nil, fmt.Errorf("invalid PNG: %v\nplease report this issue here: https://github.com/terrastruct/d2/issues/new", pngString)
	}
	splicedPNGString := pngString[len(pngPrefix):]
	return base64.StdEncoding.DecodeString(splicedPNGString)
}
