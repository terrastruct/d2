package imgbundler

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
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
	if isRemote {
		defer xdefer.Errorf(&err, "failed to bundle remote images")
	} else {
		defer xdefer.Errorf(&err, "failed to bundle local images")
	}
	imgs := imageRegex.FindAllSubmatch(svg, -1)
	imgs = filterImageElements(imgs, isRemote)

	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	return runWorkers(ctx, ms, svg, imgs, isRemote)
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

func runWorkers(ctx context.Context, ms *xmain.State, svg []byte, imgs [][][]byte, isRemote bool) (_ []byte, err error) {
	var wg sync.WaitGroup
	replc := make(chan repl)

	wg.Add(len(imgs))
	go func() {
		wg.Wait()
		close(replc)
	}()

	// Limits the number of workers to 16.
	sema := make(chan struct{}, 16)

	var errhrefsMu sync.Mutex
	var errhrefs []string

	// Start workers as the sema allows.
	go func() {
		for _, img := range imgs {
			img := img
			sema <- struct{}{}
			go func() {
				defer func() {
					wg.Done()
					<-sema
				}()

				bundledImage, err := worker(ctx, img[1], isRemote)
				if err != nil {
					ms.Log.Error.Printf("failed to bundle %s: %v", img[1], err)
					errhrefsMu.Lock()
					errhrefs = append(errhrefs, string(img[1]))
					errhrefsMu.Unlock()
					return
				}
				select {
				case <-ctx.Done():
				case replc <- repl{
					from: img[0],
					to:   bundledImage,
				}:
				}
			}()
		}
	}()

	t := time.NewTicker(time.Second * 5)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return svg, xerrors.Errorf("failed to wait for workers: %w", ctx.Err())
		case <-t.C:
			ms.Log.Info.Printf("fetching images...")
		case repl, ok := <-replc:
			if !ok {
				if len(errhrefs) > 0 {
					return svg, xerrors.Errorf("%v", errhrefs)
				}
				return svg, nil
			}
			svg = bytes.Replace(svg, repl.from, repl.to, 1)
		}
	}
}

func worker(ctx context.Context, href []byte, isRemote bool) ([]byte, error) {
	var buf []byte
	var mimeType string
	var err error
	if isRemote {
		buf, mimeType, err = httpGet(ctx, string(href))
	} else {
		buf, err = os.ReadFile(string(href))
	}
	if err != nil {
		return nil, err
	}

	if mimeType == "" {
		mimeType = sniffMimeType(href, buf, isRemote)
	}
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)
	b64 := base64.StdEncoding.EncodeToString(buf)
	return []byte(fmt.Sprintf(`<image href="data:%s;base64,%s"`, mimeType, b64)), nil
}

var httpClient = &http.Client{}

func httpGet(ctx context.Context, href string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", href, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("expected status 200 but got %d %s", resp.StatusCode, resp.Status)
	}
	r := http.MaxBytesReader(nil, resp.Body, maxImageSize)
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	return buf, resp.Header.Get("Content-Type"), nil
}

// sniffMimeType sniffs the mime type of href based on its file extension and contents.
func sniffMimeType(href, buf []byte, isRemote bool) string {
	p := string(href)
	if isRemote {
		u, err := url.Parse(p)
		if err != nil {
			p = ""
		} else {
			p = u.Path
		}
	}
	mimeType := mime.TypeByExtension(path.Ext(p))
	if mimeType == "" {
		mimeType = http.DetectContentType(buf)
	}
	return mimeType
}
