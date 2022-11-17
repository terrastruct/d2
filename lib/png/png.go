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
)

func InitPlaywright() (*playwright.Playwright, playwright.Browser, error) {
	// check if playwright driver/browsers are installed and up to date
	// https://github.com/playwright-community/playwright-go/blob/8e8f670b5fa7ba5365ae4bfc123fea4aac359763/run.go#L64.
	driver, err := playwright.NewDriver(&playwright.RunOptions{})
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(driver.DriverBinaryLocation); errors.Is(err, os.ErrNotExist) {
		err = playwright.Install()
		if err != nil {
			return nil, nil, err
		}
	} else if err == nil {
		cmd := exec.Command(driver.DriverBinaryLocation, "--version")
		output, err := cmd.Output()
		if err != nil || !bytes.Contains(output, []byte(driver.Version)) {
			err = playwright.Install()
			if err != nil {
				return nil, nil, err
			}
		}
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, nil, err
	}
	browser, err := pw.Chromium.Launch()
	if err != nil {
		return nil, nil, err
	}
	return pw, browser, nil
}

//go:embed generate_png.js
var genPNGScript string

func ExportPNG(browser playwright.Browser, svg []byte) (outputImage []byte, err error) {
	var page playwright.Page
	defer func() error {
		err = page.Close()
		if err != nil {
			return err
		}
		return nil
	}()

	if browser == nil {
		return nil, fmt.Errorf("browser is not initialized for png export")
	}
	page, err = browser.NewPage()
	if err != nil {
		return nil, err
	}
	encodedSVG := base64.StdEncoding.EncodeToString(svg)
	pngInterface, err := page.Evaluate(genPNGScript, "data:image/svg+xml;charset=utf-8;base64,"+encodedSVG)
	if err != nil {
		return nil, err
	}

	pngString := fmt.Sprintf("%v", pngInterface)
	pngPrefix := "data:image/png;base64,"
	if !strings.HasPrefix(pngString, pngPrefix) {
		return nil, fmt.Errorf("playwright export generated invalid png")
	}
	splicedPNGString := pngString[len(pngPrefix):]
	outputImage, err = base64.StdEncoding.DecodeString(splicedPNGString)
	if err != nil {
		return nil, err
	}

	return outputImage, nil
}

func Cleanup(pw *playwright.Playwright, browser playwright.Browser) (err error) {
	if browser != nil {
		if err = browser.Close(); err != nil {
			return err
		}
	}
	if pw != nil {
		if err = pw.Stop(); err != nil {
			return err
		}
	}
	return nil
}
