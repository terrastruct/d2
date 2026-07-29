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

	"oss.terrastruct.com/d2/lib/compression"
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
	// Optimizations for a very tightly scoped Playwright instance
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Args: []string{
			"--no-sandbox",                             // Removes security overhead
			"--disable-dev-shm-usage",                  // Prevents /dev/shm issues
			"--disable-background-timer-throttling",    // Prevents CPU throttling
			"--disable-backgrounding-occluded-windows", // Keeps rendering active
			"--disable-features=TranslateUI",           // Reduces feature overhead
			"--disable-ipc-flooding-protection",        // Removes IPC limits
		},
	})
	if err != nil {
		return Playwright{}, fmt.Errorf("failed to launch Chromium: %w", err)
	}
	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		DeviceScaleFactor: playwright.Float(2.0),
	})
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
	if os.Getenv("CI") != "" {
		return InitPlaywright()
	}

	// Just try running first. This only works if drivers and browsers are already installed
	pw, err := playwright.Run()
	if err == nil {
		return startPlaywright(pw)
	}

	fmt.Print("D2 needs to install Chromium v130.0.6723.19 to render non-SVG images. Continue? (y/N): ")
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

const pngPrefix = "data:image/png;base64,"

func MountSVG(page playwright.Page, svgMarkup string) error {
	decompressed := compression.UnzipEmbeddedSVGImages([]byte(svgMarkup))
	html := `<!doctype html><meta charset="utf-8">
<style>
  html,body{margin:0;background:#fff}
  #stage{display:inline-block}
</style>
<div id="stage">` + string(decompressed) + `</div>
<script>
  const s = document.querySelector('svg');
  if (s && s.pauseAnimations) s.pauseAnimations();
</script>`
	_, err := page.Goto("data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html)))
	if err != nil {
		return err
	}
	return page.Locator("svg").First().WaitFor()
}

func SetAnimationTime(page playwright.Page, t float64) error {
	_, err := page.Evaluate(`(t) => {
	  const s = document.querySelector('svg');
	  if (!s) return;
	  if (s.pauseAnimations) s.pauseAnimations();
	  if (s.setCurrentTime) s.setCurrentTime(t);
	  // Pause & scrub CSS/Web Animations too:
	  for (const a of document.getAnimations()) { a.pause(); a.currentTime = t * 1000; }
	}`, t)
	return err
}

func ScreenshotSVG(page playwright.Page) ([]byte, error) {
	return page.Locator("svg").First().Screenshot()
}

func ConvertSVG(browser playwright.Browser, svg []byte) ([]byte, error) {
	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		DeviceScaleFactor: playwright.Float(2.0),
	})
	if err != nil {
		return nil, err
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		return nil, err
	}
	defer page.Close()

	if err := MountSVG(page, string(svg)); err != nil {
		return nil, err
	}

	if err := SetAnimationTime(page, 0); err != nil {
		return nil, err
	}
	_, _ = page.Evaluate(`() => new Promise(r => requestAnimationFrame(() => r())))`)

	png, err := ScreenshotSVG(page)
	if err != nil {
		return nil, err
	}
	return png, nil
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
