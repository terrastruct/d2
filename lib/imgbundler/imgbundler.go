package imgbundler

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
	"oss.terrastruct.com/xdefer"

	"oss.terrastruct.com/d2/lib/xmain"
)

const maxImageSize int64 = 1 << 25 // 33_554_432

var imageRegex = regexp.MustCompile(`<image href="([^"]+)"`)

func BundleLocal(ctx context.Context, ms *xmain.State, in []byte) ([]byte, error) {
	return bundle(ctx, ms, in, false)
}

func BundleRemote(ctx context.Context, ms *xmain.State, in []byte) ([]byte, error) {
	return bundle(ctx, ms, in, true)
}

type repl struct {
	from []byte
	to   []byte
}

func bundle(ctx context.Context, ms *xmain.State, svg []byte, isRemote bool) (_ []byte, err error) {
	defer xdefer.Errorf(&err, "failed to bundle images")
	imgs := imageRegex.FindAllSubmatch(svg, -1)
	imgs = filterImageElements(imgs, isRemote)

	var wg sync.WaitGroup
	replc := make(chan repl)
	// Limits the number of workers to 16.
	sema := make(chan struct{}, 16)

	var errhrefsMu sync.Mutex
	var errhrefs []string

	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	wg.Add(len(imgs))
	// Start workers as the sema allows.
	go func() {
		for _, img := range imgs {
			sema <- struct{}{}
			go func(imgel, href []byte) {
				defer func() {
					wg.Done()
					<-sema
				}()

				var buf []byte
				var err error
				if isRemote {
					buf, err = httpGet(ctx, string(href))
				} else {
					buf, err = os.ReadFile(string(href))
				}
				if err != nil {
					ms.Log.Error.Printf("failed to bundle %s: %v", imgel, err)
					errhrefsMu.Lock()
					errhrefs = append(errhrefs, string(href))
					errhrefsMu.Unlock()
					return
				}

				mimeType := http.DetectContentType(buf)
				mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)
				b64 := base64.StdEncoding.EncodeToString(buf)

				select {
				case <-ctx.Done():
				case replc <- repl{
					from: imgel,
					to:   []byte(fmt.Sprintf(`<image href="data:%s;base64,%s"`, mimeType, b64)),
				}:
				}
			}(img[0], img[1])
		}
	}()

	go func() {
		wg.Wait()
		close(replc)
	}()

	for {
		select {
		case <-ctx.Done():
			return svg, xerrors.Errorf("failed to wait for workers: %w", ctx.Err())
		case <-time.After(time.Second * 5):
			ms.Log.Info.Printf("fetching images...")
		case repl, ok := <-replc:
			if !ok {
				if len(errhrefs) > 0 {
					return svg, xerrors.Errorf("failed to bundle the following images: %v", errhrefs)
				}
				return svg, nil
			}
			svg = bytes.Replace(svg, repl.from, repl.to, 1)
		}
	}
}

// filterImageElements finds all image elements in imgs that are eligible
// for bundling in the current context.
func filterImageElements(imgs [][][]byte, isRemote bool) [][][]byte {
	imgs2 := imgs[:0]
	for _, img := range imgs {
		href := string(img[1])

		// Skip already bundled images.
		if strings.HasPrefix(href, "data:") {
			continue
		}

		u, err := url.Parse(href)
		isRemoteImg := err == nil && strings.HasPrefix(u.Scheme, "http")

		if isRemoteImg == isRemote {
			imgs2 = append(imgs2, img)
		}
	}
	return imgs2
}

var httpClient = &http.Client{}

func httpGet(ctx context.Context, href string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", href, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("expected status 200 but got %d %s", resp.StatusCode, resp.Status)
	}
	r := http.MaxBytesReader(nil, resp.Body, maxImageSize)
	return ioutil.ReadAll(r)
}
