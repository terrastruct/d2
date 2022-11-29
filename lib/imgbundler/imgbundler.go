package imgbundler

import (
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

	"oss.terrastruct.com/d2/lib/xmain"
)

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

func inline(ms *xmain.State, svg []byte, isRemote bool) ([]byte, error) {
	imgs := imageRe.FindAllSubmatch(svg, -1)

	var filtered [][]string
	for _, img := range imgs {
		u, err := url.Parse(string(img[1]))
		isRemoteImg := err == nil && strings.HasPrefix(u.Scheme, "http")
		if isRemoteImg == isRemote {
			filtered = append(filtered, []string{string(img[0]), string(img[0])})
		}
	}

	var wg sync.WaitGroup
	respChan := make(chan resp)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wg.Add(len(filtered))
	for _, img := range filtered {
		go func(src, href string) {
			var data string
			var err error
			if isRemote {
				data, err = fetch(ctx, href)
			} else {
				data, err = read(ctx, href)
			}
			respChan <- resp{
				srctxt: src,
				data:   data,
				err:    err,
			}
		}(img[0], img[1])
	}

	out := string(svg)
	go func() {
		for {
			select {
			case resp, ok := <-respChan:
				if !ok {
					return
				}
				if resp.err != nil {
					ms.Log.Error.Printf("image failed to fetch: %v", resp.err)
				} else {
					out = strings.Replace(out, resp.srctxt, fmt.Sprintf(`<image href="%s"`, resp.data), 1)
				}
				wg.Done()
			}
		}
	}()

	wg.Wait()
	close(respChan)

	return []byte(out), nil
}

var transport = http.DefaultTransport

func fetch(ctx context.Context, href string) (string, error) {
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
	data, err := ioutil.ReadAll(imgResp.Body)
	if err != nil {
		return "", err
	}

	mimeType := http.DetectContentType(data)
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)

	enc := base64.StdEncoding.EncodeToString(data)

	return fmt.Sprintf("data:%s;base64,%s", mimeType, enc), nil
}

func read(ctx context.Context, href string) (string, error) {
	data, err := os.ReadFile(href)
	if err != nil {
		return "", err
	}

	mimeType := http.DetectContentType(data)
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)

	enc := base64.StdEncoding.EncodeToString(data)

	return fmt.Sprintf("data:%s;base64,%s", mimeType, enc), nil
}
