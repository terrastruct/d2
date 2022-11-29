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

	"go.uber.org/multierr"
	"oss.terrastruct.com/xdefer"

	"oss.terrastruct.com/d2/lib/xmain"
)

// 32 MB
var max_img_size int64 = 33_554_432

var imageRe = regexp.MustCompile(`<image href="([^"]+)"`)

type resp struct {
	srctxt string
	data   string
	err    error
}

func InlineLocal(ms *xmain.State, in []byte) ([]byte, error) {
	return inline(ms, in, false)
}

func InlineRemote(ms *xmain.State, in []byte) ([]byte, error) {
	return inline(ms, in, true)
}

func inline(ms *xmain.State, svg []byte, isRemote bool) (_ []byte, err error) {
	defer xdefer.Errorf(&err, "failed to bundle images")
	imgs := imageRe.FindAllSubmatch(svg, -1)

	var filtered [][][]byte
	for _, img := range imgs {
		u, err := url.Parse(string(img[1]))
		isRemoteImg := err == nil && strings.HasPrefix(u.Scheme, "http")
		if isRemoteImg == isRemote {
			filtered = append(filtered, img)
		}
	}

	var wg sync.WaitGroup
	respChan := make(chan resp)
	// Limits the number of workers to 16.
	sema := make(chan struct{}, 16)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	wg.Add(len(filtered))
	// Start workers as the sema allows.
	go func() {
		for _, img := range filtered {
			sema <- struct{}{}
			go func(src, href string) {
				defer func() {
					wg.Done()
					<-sema
				}()

				var data string
				var err error
				if isRemote {
					data, err = fetch(ctx, href)
				} else {
					data, err = read(href)
				}
				select {
				case <-ctx.Done():
				case respChan <- resp{
					srctxt: src,
					data:   data,
					err:    err,
				}:
				}
			}(string(img[0]), string(img[1]))
		}
	}()

	go func() {
		wg.Wait()
		close(respChan)
	}()

	for {
		select {
		case <-ctx.Done():
			ms.Log.Debug.Printf("there")
			return nil, fmt.Errorf("failed waiting for imgbundler workers: %w", ctx.Err())
		case <-time.After(time.Second * 5):
			ms.Log.Info.Printf("fetching images...")
		case resp, ok := <-respChan:
			if !ok {
				return svg, err
			}
			if resp.err != nil {
				err = multierr.Combine(err, resp.err)
				continue
			}
			svg = bytes.Replace(svg, []byte(resp.srctxt), []byte(fmt.Sprintf(`<image href="%s"`, resp.data)), 1)
		}
	}
}

var transport = http.DefaultTransport

func fetch(ctx context.Context, href string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", href, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Transport: transport}
	imgResp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer imgResp.Body.Close()
	if imgResp.StatusCode != 200 {
		return "", fmt.Errorf("img %s returned status code %d", href, imgResp.StatusCode)
	}
	r := http.MaxBytesReader(nil, imgResp.Body, max_img_size)
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}

	mimeType := http.DetectContentType(data)
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)

	enc := base64.StdEncoding.EncodeToString(data)

	return fmt.Sprintf("data:%s;base64,%s", mimeType, enc), nil
}

func read(href string) (string, error) {
	data, err := os.ReadFile(href)
	if err != nil {
		return "", err
	}

	mimeType := http.DetectContentType(data)
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)

	enc := base64.StdEncoding.EncodeToString(data)

	return fmt.Sprintf("data:%s;base64,%s", mimeType, enc), nil
}
