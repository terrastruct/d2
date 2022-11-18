package png

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
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

func (pw *Playwright) RestartBrowser() (newPW Playwright, err error) {
	if err = pw.Browser.Close(); err != nil {
		return Playwright{}, err
	}
	return startPlaywright(pw.PW)
}

func (pw *Playwright) Cleanup() error {
	if err := pw.Browser.Close(); err != nil {
		return err
	}
	if err := pw.PW.Stop(); err != nil {
		return err
	}
	return nil
}

func startPlaywright(pw *playwright.Playwright) (Playwright, error) {
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
		PW:      pw,
		Browser: browser,
		Page:    page,
	}, nil
}

func InitPlaywright() (Playwright, error) {
	// check if playwright driver/browsers are installed and up to date
	// https://github.com/playwright-community/playwright-go/blob/8e8f670b5fa7ba5365ae4bfc123fea4aac359763/run.go#L64.
	driver, err := playwright.NewDriver(&playwright.RunOptions{})
	if err != nil {
		return Playwright{}, err
	}
	_, err = os.Stat(driver.DriverBinaryLocation)
	if err != nil {
		if os.IsNotExist(err) {
			err = playwright.Install()
			if err != nil {
				return Playwright{}, err
			}
		} else {
			return Playwright{}, fmt.Errorf("could not access Playwright binary location: %v\nerror: %w\nplease report this issue here: https://github.com/terrastruct/d2/issues/new", driver.DriverBinaryLocation, err)
		}
	}

	cmd := exec.Command(driver.DriverBinaryLocation, "--version")
	output, err := cmd.Output()
	if err != nil {
		return Playwright{}, fmt.Errorf("error getting Playwright version: %w\nplease report this issue here: https://github.com/terrastruct/d2/issues/new", err)
	}
	if !bytes.Contains(output, []byte(driver.Version)) {
		err = playwright.Install()
		if err != nil {
			return Playwright{}, err
		}
	}

	pw, err := playwright.Run()
	if err != nil {
		return Playwright{}, err
	}
	return startPlaywright(pw)
}

//go:embed generate_png.js
var genPNGScript string

const pngPrefix = "data:image/png;base64,"

func ConvertSVG(ms *xmain.State, page playwright.Page, svg []byte) (outputImage []byte, err error) {
	encodedSVG := base64.StdEncoding.EncodeToString(svg)
	pngInterface, err := page.Evaluate(genPNGScript, "data:image/svg+xml;charset=utf-8;base64,"+encodedSVG)
	if err != nil {
		return nil, fmt.Errorf("failed to generate png: %w\nplease report this issue here: https://github.com/terrastruct/d2/issues/new", err)
	}

	pngString := fmt.Sprintf("%v", pngInterface)
	if !strings.HasPrefix(pngString, pngPrefix) {
		if len(pngString) > 50 {
			pngString = pngString[0:50] + "..."
		}
		return nil, fmt.Errorf("invalid PNG: %v\nplease report this issue here: https://github.com/terrastruct/d2/issues/new", pngString)
	}
	splicedPNGString := pngString[len(pngPrefix):]
	return base64.StdEncoding.DecodeString(splicedPNGString)
}
