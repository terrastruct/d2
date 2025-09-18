package png

import (
	"bytes"
	"encoding/base64"
	"fmt"
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
	return startPlaywrightWithPath(pw, "")
}

func createPlaywrightInstance(pw *playwright.Playwright, browser playwright.Browser) (Playwright, error) {
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

func launchBrowser(pw *playwright.Playwright, opts playwright.BrowserTypeLaunchOptions) (playwright.Browser, error) {
	browser, err := pw.Chromium.Launch(opts)
	if err != nil {
		return nil, err
	}
	return browser, nil
}

func startPlaywrightWithPath(pw *playwright.Playwright, chromiumPath string) (Playwright, error) {
	args := []string{
		"--no-sandbox",                    // Removes security overhead
		"--disable-dev-shm-usage",         // Prevents /dev/shm issues
		"--disable-background-timer-throttling", // Prevents CPU throttling
		"--disable-backgrounding-occluded-windows", // Keeps rendering active
		"--disable-features=TranslateUI",  // Reduces feature overhead
		"--disable-ipc-flooding-protection", // Removes IPC limits
	}

	launchOptions := playwright.BrowserTypeLaunchOptions{
		Args: args,
	}

	var browser playwright.Browser
	var err error

	// If custom chromium path is specified, use it directly
	if chromiumPath != "" {
		launchOptions.ExecutablePath = playwright.String(chromiumPath)
		browser, err = launchBrowser(pw, launchOptions)
		if err != nil {
			return Playwright{}, fmt.Errorf("failed to launch Chromium at %s: %w", chromiumPath, err)
		}
		return createPlaywrightInstance(pw, browser)
	}

	// Try system Chrome first
	launchOptions.Channel = playwright.String("chrome")
	browser, err = launchBrowser(pw, launchOptions)
	if err != nil {
		// Fall back to system Chromium
		launchOptions.Channel = playwright.String("chromium")
		browser, err = launchBrowser(pw, launchOptions)
		if err != nil {
			// Fall back to bundled Chromium
			launchOptions.Channel = nil
			browser, err = launchBrowser(pw, launchOptions)
			if err != nil {
				return Playwright{}, fmt.Errorf("failed to launch Chromium: %w", err)
			}
		}
	}

	return createPlaywrightInstance(pw, browser)
}

func InitPlaywright() (Playwright, error) {
	return InitPlaywrightWithPath("")
}

func InitPlaywrightWithPath(chromiumPath string) (Playwright, error) {
	// Try to skip browser installation first
	err := playwright.Install(&playwright.RunOptions{
		SkipInstallBrowsers: true,
	})
	if err != nil {
		// Fall back to installing browsers if needed
		err = playwright.Install(&playwright.RunOptions{
			Verbose:  false,
			Browsers: []string{"chromium"},
		})
		if err != nil {
			return Playwright{}, fmt.Errorf("failed to install Playwright: %w", err)
		}
	}

	pw, err := playwright.Run()
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to run Playwright: %w", err)
	}
	return startPlaywrightWithPath(pw, chromiumPath)
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
