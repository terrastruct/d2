package main

import (
	"context"
	"embed"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/lib/png"
	"oss.terrastruct.com/d2/lib/xbrowser"
	"oss.terrastruct.com/d2/lib/xhttp"
	"oss.terrastruct.com/d2/lib/xmain"
)

// Enabled with the build tag "dev".
// See watch_dev.go
// Controls whether the embedded staticFS is used or if files are served directly from the
// file system. Useful for quick iteration in development.
var devMode = false

//go:embed static
var staticFS embed.FS

type watcher struct {
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	devMode bool

	ms           *xmain.State
	layoutPlugin d2plugin.Plugin
	inputPath    string
	outputPath   string

	compileCh chan struct{}

	fw               *fsnotify.Watcher
	l                net.Listener
	staticFileServer http.Handler

	wsclientsMu sync.Mutex
	closing     bool
	wsclientsWG sync.WaitGroup
	wsclients   map[*wsclient]struct{}

	errMu sync.Mutex
	err   error

	resMu sync.Mutex
	res   *compileResult

	pw png.Playwright
}

type compileResult struct {
	Err string `json:"err"`
	SVG string `json:"svg"`
}

func newWatcher(ctx context.Context, ms *xmain.State, layoutPlugin d2plugin.Plugin, inputPath, outputPath string, pw png.Playwright) (*watcher, error) {
	ctx, cancel := context.WithCancel(ctx)

	w := &watcher{
		ctx:     ctx,
		cancel:  cancel,
		devMode: devMode,

		ms:           ms,
		layoutPlugin: layoutPlugin,
		inputPath:    inputPath,
		outputPath:   outputPath,

		compileCh: make(chan struct{}, 1),
		wsclients: make(map[*wsclient]struct{}),
		pw:        pw,
	}
	err := w.init()
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (w *watcher) init() error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.fw = fw
	err = w.initStaticFileServer()
	if err != nil {
		return err
	}
	return w.listen()
}

func (w *watcher) initStaticFileServer() error {
	// Serve files directly in dev mode for fast iteration.
	if w.devMode {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			return errors.New("d2: runtime failed to provide path of watch.go")
		}

		staticFilesDir := filepath.Join(filepath.Dir(file), "./static")
		w.staticFileServer = http.FileServer(http.Dir(staticFilesDir))
		return nil
	}

	sfs, err := fs.Sub(staticFS, "static")
	if err != nil {
		return err
	}
	w.staticFileServer = http.FileServer(http.FS(sfs))
	return nil
}

func (w *watcher) run() error {
	defer w.close()

	w.goFunc(w.watchLoop)
	w.goFunc(w.compileLoop)

	err := w.goServe()
	if err != nil {
		return err
	}

	w.wg.Wait()
	w.close()
	return w.err
}

func (w *watcher) close() {
	w.wsclientsMu.Lock()
	if w.closing {
		w.wsclientsMu.Unlock()
		return
	}
	w.closing = true
	w.wsclientsMu.Unlock()

	w.cancel()
	if w.fw != nil {
		err := w.fw.Close()
		w.setErr(err)
	}
	if w.l != nil {
		err := w.l.Close()
		w.setErr(err)
	}

	w.wsclientsWG.Wait()
}

func (w *watcher) setErr(err error) {
	w.errMu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.errMu.Unlock()
}

func (w *watcher) goFunc(fn func(context.Context) error) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer w.cancel()

		err := fn(w.ctx)
		w.setErr(err)
	}()
}

/*
 * IMPORTANT
 *
 * Do not touch watchLoop or ensureAddWatch without consulting @nhooyr
 * fsnotify and file system watching APIs in general are notoriously hard
 * to use correctly.
 *
 * This issue is a good summary though it too contains confusion and misunderstandings:
 *   https://github.com/fsnotify/fsnotify/issues/372
 *
 * The code was thoroughly considered and experimentally vetted.
 *
 * TODO: Abstract out file system and fsnotify to test this with 100% coverage. See comment in main_test.go
 */
func (w *watcher) watchLoop(ctx context.Context) error {
	lastModified, err := w.ensureAddWatch(ctx)
	if err != nil {
		return err
	}
	w.ms.Log.Info.Printf("compiling %v...", w.inputPath)
	w.requestCompile()

	eatBurstTimer := time.NewTimer(0)
	<-eatBurstTimer.C
	pollTicker := time.NewTicker(time.Second * 10)
	defer pollTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			// In case we missed an event indicating the path is unwatchable and we won't be
			// getting any more events.
			// File notification APIs are notoriously unreliable. I've personally experienced
			// many quirks and so feel this check is justified even if excessive.
			mt, err := w.ensureAddWatch(ctx)
			if err != nil {
				return err
			}
			if !mt.Equal(lastModified) {
				// We missed changes.
				lastModified = mt
				w.requestCompile()
			}
		case ev, ok := <-w.fw.Events:
			if !ok {
				return errors.New("fsnotify watcher closed")
			}
			w.ms.Log.Debug.Printf("received file system event %v", ev)
			mt, err := w.ensureAddWatch(ctx)
			if err != nil {
				return err
			}
			if ev.Op == fsnotify.Chmod {
				if mt.Equal(lastModified) {
					// Benign Chmod.
					// See https://github.com/fsnotify/fsnotify/issues/15
					continue
				}
				// We missed changes.
				lastModified = mt
			}
			// The purpose of eatBurstTimer is to wait at least 32 milliseconds after a sequence of
			// events to ensure that whomever is editing the file is now done.
			//
			// For example, On macOS editing with neovim, every write I see a chmod immediately
			// followed by a write followed by another chmod. We don't want the three events to
			// be treated as two or three compilations, we want them to be batched into one.
			//
			// Another example would be a very large file where one logical edit becomes write
			// events. We wouldn't want to try to compile an incomplete file and then report a
			// misleading error.
			eatBurstTimer.Reset(time.Millisecond * 32)
		case <-eatBurstTimer.C:
			w.ms.Log.Info.Printf("detected change in %v: recompiling...", w.inputPath)
			w.requestCompile()
		case err, ok := <-w.fw.Errors:
			if !ok {
				return errors.New("fsnotify watcher closed")
			}
			w.ms.Log.Error.Printf("fsnotify error: %v", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *watcher) requestCompile() {
	select {
	case w.compileCh <- struct{}{}:
	default:
	}
}

func (w *watcher) ensureAddWatch(ctx context.Context) (time.Time, error) {
	interval := time.Second
	tc := time.NewTimer(0)
	<-tc.C
	for {
		mt, err := w.addWatch(ctx)
		if err == nil {
			return mt, nil
		}
		w.ms.Log.Error.Printf("failed to watch inputPath %q: %v (retrying in %v)", w.inputPath, err, interval)

		tc.Reset(interval)
		select {
		case <-tc.C:
			if interval < time.Second*16 {
				interval *= 2
			}
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		}
	}
}

func (w *watcher) addWatch(ctx context.Context) (time.Time, error) {
	err := w.fw.Add(w.inputPath)
	if err != nil {
		return time.Time{}, err
	}
	var d os.FileInfo
	d, err = os.Stat(w.inputPath)
	if err != nil {
		return time.Time{}, err
	}
	return d.ModTime(), nil
}

func (w *watcher) compileLoop(ctx context.Context) error {
	firstCompile := true
	for {
		select {
		case <-w.compileCh:
		case <-ctx.Done():
			return ctx.Err()
		}

		recompiledPrefix := ""
		if !firstCompile {
			recompiledPrefix = "re"
		}

		if filepath.Ext(w.outputPath) == ".png" && !w.pw.Browser.IsConnected() {
			newPW, err := w.pw.RestartBrowser()
			if err != nil {
				broadcastErr := fmt.Errorf("issue encountered with PNG exporter, stopping")
				w.ms.Log.Error.Print(broadcastErr)
				w.broadcast(&compileResult{
					Err: broadcastErr.Error(),
				})
				continue
			}
			w.pw = newPW
		}

		b, err := compile(ctx, w.ms, w.layoutPlugin, w.inputPath, w.outputPath, w.pw.Page)
		if err != nil {
			err = fmt.Errorf("failed to %scompile: %w", recompiledPrefix, err)
			w.ms.Log.Error.Print(err)
			w.broadcast(&compileResult{
				Err: err.Error(),
			})
		} else {
			w.ms.Log.Success.Printf("successfully %scompiled %v to %v", recompiledPrefix, w.inputPath, w.outputPath)
			w.broadcast(&compileResult{
				SVG: string(b),
			})
		}

		if firstCompile {
			firstCompile = false
			url := fmt.Sprintf("http://%s", w.l.Addr())
			err = xbrowser.OpenURL(ctx, w.ms.Env, url)
			if err != nil {
				w.ms.Log.Warn.Printf("failed to open browser to %v: %v", url, err)
			}
		}
	}
}

func (w *watcher) listen() error {
	host := "localhost"
	port := "0"
	hostEnv := w.ms.Env.Getenv("HOST")
	if hostEnv != "" {
		host = hostEnv
	}
	portEnv := w.ms.Env.Getenv("PORT")
	if portEnv != "" {
		port = portEnv
	}

	l, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	w.l = l
	w.ms.Log.Success.Printf("listening on http://%v", w.l.Addr())
	return nil
}

func (w *watcher) goServe() error {
	m := http.NewServeMux()
	// TODO: Add cmdlog logging and error reporting middleware
	// TODO: Add standard debug/profiling routes
	m.HandleFunc("/", w.handleRoot)
	m.Handle("/static/", http.StripPrefix("/static", w.staticFileServer))
	m.Handle("/watch", xhttp.HandlerFuncAdapter{w.ms.Log, w.handleWatch})

	s := xhttp.NewServer(w.ms.Log.Warn, xhttp.Log(w.ms.Log, m))
	w.goFunc(func(ctx context.Context) error {
		return xhttp.Serve(ctx, time.Second*30, s, w.l)
	})

	return nil
}

func (w *watcher) getRes() *compileResult {
	w.resMu.Lock()
	defer w.resMu.Unlock()
	return w.res
}

func (w *watcher) handleRoot(hw http.ResponseWriter, r *http.Request) {
	hw.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(hw, `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
	<script src="./static/watch.js"></script>
	<link rel="stylesheet" href="./static/watch.css">
</head>
<body data-d2-dev-mode=%t>
	<div id="d2-err" style="display: none"></div>
	<div id="d2-svg"></div>
</body>
</html>`, w.outputPath, w.devMode)
}

func (w *watcher) handleWatch(hw http.ResponseWriter, r *http.Request) error {
	w.wsclientsMu.Lock()
	if w.closing {
		w.wsclientsMu.Unlock()
		return xhttp.Errorf(http.StatusServiceUnavailable, "server shutting down...", "server shutting down...")
	}
	// We must register ourselves before we even upgrade the connection to ensure that
	// w.close() will wait for us. If we instead registered afterwards, then there is a
	// brief period between the hijack and the registration where close may return without
	// waiting for us to finish.
	w.wsclientsWG.Add(1)
	w.wsclientsMu.Unlock()

	c, err := websocket.Accept(hw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		w.wsclientsWG.Done()
		return err
	}

	go func() {
		defer w.wsclientsWG.Done()
		defer c.Close(websocket.StatusInternalError, "the sky is falling")

		ctx, cancel := context.WithTimeout(w.ctx, time.Hour)
		defer cancel()

		cl := &wsclient{
			w:         w,
			resultsCh: make(chan struct{}, 1),
			c:         c,
		}

		w.wsclientsMu.Lock()
		w.wsclients[cl] = struct{}{}
		w.wsclientsMu.Unlock()
		defer func() {
			w.wsclientsMu.Lock()
			delete(w.wsclients, cl)
			w.wsclientsMu.Unlock()
		}()

		ctx = cl.c.CloseRead(ctx)
		go wsHeartbeat(ctx, cl.c)
		_ = cl.writeLoop(ctx)
	}()
	return nil
}

type wsclient struct {
	w         *watcher
	resultsCh chan struct{}
	c         *websocket.Conn
}

func (cl *wsclient) writeLoop(ctx context.Context) error {
	for {
		res := cl.w.getRes()
		if res != nil {
			err := cl.write(ctx, res)
			if err != nil {
				return err
			}
		}

		select {
		case <-cl.resultsCh:
		case <-ctx.Done():
			cl.c.Close(websocket.StatusGoingAway, "server shutting down...")
			return ctx.Err()
		}
	}
}

func (cl *wsclient) write(ctx context.Context, res *compileResult) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	return wsjson.Write(ctx, cl.c, res)
}

func (w *watcher) broadcast(res *compileResult) {
	w.resMu.Lock()
	w.res = res
	w.resMu.Unlock()

	w.wsclientsMu.Lock()
	defer w.wsclientsMu.Unlock()
	clientsSuffix := ""
	if len(w.wsclients) != 1 {
		clientsSuffix = "s"
	}
	w.ms.Log.Info.Printf("broadcasting update to %d client%s", len(w.wsclients), clientsSuffix)
	for cl := range w.wsclients {
		select {
		case cl.resultsCh <- struct{}{}:
		default:
		}
	}
}

func wsHeartbeat(ctx context.Context, c *websocket.Conn) {
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	t := time.NewTimer(0)
	<-t.C
	for {
		err := c.Ping(ctx)
		if err != nil {
			return
		}

		t.Reset(time.Second * 30)
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}
