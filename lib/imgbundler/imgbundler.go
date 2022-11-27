package imgbundler

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"oss.terrastruct.com/d2/lib/xmain"
)

var uriImgRe = regexp.MustCompile(`<image href="(http[^"]+)"`)
var localImgRe = regexp.MustCompile(`<image href="([^http][^"]+)"`)

type resp struct {
	srctxt string
	data   string
	err    error
}

func InlineLocal(ms *xmain.State, in []byte) ([]byte, error) {
	return inline(ms, localImgRe, in)
}

func InlineRemote(ms *xmain.State, in []byte) ([]byte, error) {
	return inline(ms, uriImgRe, in)
}

func inline(ms *xmain.State, re *regexp.Regexp, in []byte) ([]byte, error) {
	svg := string(in)

	imgs := re.FindAllStringSubmatch(svg, -1)

	var wg sync.WaitGroup
	respChan := make(chan resp)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wg.Add(len(imgs))
	for _, img := range imgs {
		if re == uriImgRe {
			go fetch(ctx, img[0], img[1], respChan)
		} else {
			go read(ctx, img[0], img[1], respChan)
		}
	}

	go func() {
		for {
			select {
			case resp, ok := <-respChan:
				if !ok {
					return
				}
				if resp.err != nil {
					ms.Log.Error.Printf("image failed to fetch: %s", resp.err.Error())
				} else {
					svg = strings.Replace(svg, resp.srctxt, fmt.Sprintf(`<image href="%s"`, resp.data), 1)
				}
				wg.Done()
			}
		}
	}()

	wg.Wait()
	close(respChan)

	return []byte(svg), nil
}

var transport = http.DefaultTransport

func fetch(ctx context.Context, srctxt, href string, respChan chan resp) {
	req, err := http.NewRequestWithContext(ctx, "GET", href, nil)
	if err != nil {

		respChan <- resp{err: err}
		return
	}

	client := &http.Client{Transport: transport}
	imgResp, err := client.Do(req)
	if err != nil {
		respChan <- resp{err: err}
		return
	}
	defer imgResp.Body.Close()
	data, err := ioutil.ReadAll(imgResp.Body)
	if err != nil {
		respChan <- resp{err: err}
		return
	}

	mimeType := http.DetectContentType(data)
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)

	enc := base64.StdEncoding.EncodeToString(data)

	respChan <- resp{
		srctxt: srctxt,
		data:   fmt.Sprintf("data:%s;base64,%s", mimeType, enc),
	}
}

func read(ctx context.Context, srctxt, href string, respChan chan resp) {
	data, err := os.ReadFile(href)
	if err != nil {
		respChan <- resp{err: err}
		return
	}

	mimeType := http.DetectContentType(data)
	mimeType = strings.Replace(mimeType, "text/xml", "image/svg+xml", 1)

	enc := base64.StdEncoding.EncodeToString(data)

	respChan <- resp{
		srctxt: srctxt,
		data:   fmt.Sprintf("data:%s;base64,%s", mimeType, enc),
	}
}
