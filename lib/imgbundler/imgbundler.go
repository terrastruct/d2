package imgbundler

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"oss.terrastruct.com/d2/lib/simplelog"
	"oss.terrastruct.com/util-go/xdefer"
)

var imgCache sync.Map

const maxImageSize int64 = 1 << 25 // 33_554_432

var imageRegex = regexp.MustCompile(`<image href="([^"]+)"`)

func BundleLocal(ctx context.Context, l simplelog.Logger, inputPath string, in []byte, cacheImages bool) ([]byte, error) {
	return bundle(ctx, l, inputPath, in, false, cacheImages)
}

func BundleRemote(ctx context.Context, l simplelog.Logger, in []byte, cacheImages bool) ([]byte, error) {
	return bundle(ctx, l, "", in, true, cacheImages)
}

type repl struct {
	from []byte
	to   []byte
}

func bundle(ctx context.Context, l simplelog.Logger, inputPath string, svg []byte, isRemote, cacheImages bool) (_ []byte, err error) {
	if isRemote {
		defer xdefer.Errorf(&err, "failed to bundle remote images")
	} else {
		defer xdefer.Errorf(&err, "failed to bundle local images")
	}
	imgs := imageRegex.FindAllSubmatch(svg, -1)
	imgs = filterImageElements(imgs, isRemote)

	if len(imgs) == 0 {
		return svg, nil
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	return runWorkers(ctx, l, inputPath, svg, imgs, isRemote, cacheImages)
}

// filterImageElements finds all unique image elements in imgs that are
// eligible for bundling in the current context.
func filterImageElements(imgs [][][]byte, isRemote bool) [][][]byte {
	unq := make(map[string]struct{})
	imgs2 := imgs[:0]
	for _, img := range imgs {
		href := string(img[1])
		if _, ok := unq[href]; ok {
			continue
		}
		unq[href] = struct{}{}

		// Skip already bundled images.
		if strings.HasPrefix(href, "data:") {
			continue
		}

		u, err := url.Parse(html.UnescapeString(href))
		isRemoteImg := err == nil && strings.HasPrefix(u.Scheme, "http")

		if isRemoteImg == isRemote {
			imgs2 = append(imgs2, img)
		}
	}
	return imgs2
}

func runWorkers(ctx context.Context, l simplelog.Logger, inputPath string, svg []byte, imgs [][][]byte, isRemote, cacheImages bool) (_ []byte, err error) {
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

				bundledImage, err := worker(ctx, l, inputPath, img[1], isRemote, cacheImages)
				if err != nil {
					l.Error(fmt.Sprintf("failed to bundle %s: %v", img[1], err))
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
			l.Info("fetching images...")
		case repl, ok := <-replc:
			if !ok {
				if len(errhrefs) > 0 {
					return svg, xerrors.Errorf("%v", errhrefs)
				}
				return svg, nil
			}
			svg = bytes.Replace(svg, repl.from, repl.to, -1)
		}
	}
}

func worker(ctx context.Context, l simplelog.Logger, inputPath string, href []byte, isRemote, cacheImages bool) ([]byte, error) {
	if cacheImages {
		if hit, ok := imgCache.Load(string(href)); ok {
			return hit.([]byte), nil
		}
	}
	var buf []byte
	var mimeType string
	var err error
	if isRemote {
		l.Debug(fmt.Sprintf("fetching %s remotely", string(href)))
		buf, mimeType, err = httpGet(ctx, l, html.UnescapeString(string(href)))
	} else {
		l.Debug(fmt.Sprintf("reading %s from disk", string(href)))
		path := html.UnescapeString(string(href))
		if inputPath != "-" && !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(inputPath), path)
		}
		buf, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}

	if mimeType == "" {
		mimeType = sniffMimeType(href, buf, isRemote)
		l.Debug(fmt.Sprintf("no mimetype provided - sniffed MIME type for %s: %s", string(href), mimeType))
	} else {
		l.Debug(fmt.Sprintf("mimetype provided for %s: %s", string(href), mimeType))
	}
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)
	if mimeType == "application/octet-stream" && bytes.Contains(buf, []byte("<svg")) {
		l.Debug(fmt.Sprintf("octet-stream mimetype replaced with svg for %s", string(href)))
		mimeType = "image/svg+xml"
	}
	b64 := base64.StdEncoding.EncodeToString(buf)

	out := []byte(fmt.Sprintf(`<image href="data:%s;base64,%s"`, mimeType, b64))
	if cacheImages {
		imgCache.Store(string(href), out)
	}
	return out, nil
}

var httpClient = &http.Client{}

func httpGet(ctx context.Context, l simplelog.Logger, href string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", href, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	l.Debug(fmt.Sprintf("fetched %s remotely - response code %v", string(href), resp.StatusCode))
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("expected status 200 but got %d %s", resp.StatusCode, resp.Status)
	}
	r := http.MaxBytesReader(nil, resp.Body, maxImageSize)
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	l.Debug(fmt.Sprintf("fetched content type: %s, Content length: %d bytes", contentType, len(buf)))

	return buf, contentType, nil
}

// sniffMimeType sniffs the mime type of href based on its file extension and contents.
func sniffMimeType(href, buf []byte, isRemote bool) string {
	p := string(href)
	if isRemote {
		u, err := url.Parse(html.UnescapeString(p))
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
