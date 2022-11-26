package imgbundler

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"oss.terrastruct.com/d2/lib/xmain"
)

var imgRe = regexp.MustCompile(`<image href="([^"]+)"`)

type resp struct {
	srctxt string
	data   string
	err    error
}

func Inline(ms *xmain.State, in []byte) ([]byte, error) {
	svg := string(in)

	imgs := imgRe.FindAllStringSubmatch(svg, -1)

	var wg sync.WaitGroup
	respChan := make(chan resp)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wg.Add(len(imgs))
	for _, img := range imgs {
		go fetch(ctx, img[0], img[1], respChan)
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
