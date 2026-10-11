package d2cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/fsnotify/fsnotify"

	"github.com/d2lang/util-go/xbrowser"

	"github.com/d2lang/util-go/xhttp"

	"github.com/d2lang/util-go/xmain"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/lib/localfile"
)

// Enabled with the build tag "dev".
// See watch_dev.go
// Controls whether the embedded staticFS is used or if files are served directly from the
// file system. Useful for quick iteration in development.
var devMode = false

//go:embed static
var staticFS embed.FS

type watcherOpts struct {
	layout          *string
	renderOpts      d2svg.RenderOpts
	animateInterval int64
	host            string
	port            string
	inputPath       string
	outputPath      string
	boardPath       string
	bundle          bool
	forceAppendix   bool
	fontFamily      *d2fonts.FontFamily
	monoFontFamily  *d2fonts.FontFamily
	outputFormat    exportExtension
	asciiMode       string
}

type watcher struct {
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	devMode bool

	ms *xmain.State
	watcherOpts

	compileCh chan struct{}

	fw               *fsnotify.Watcher
	l                net.Listener
	staticFileServer http.Handler

	boardpathMu sync.Mutex
	wsclientsMu sync.Mutex
	closing     bool
	wsclientsWG sync.WaitGroup
	wsclients   map[*wsclient]struct{}

	errMu sync.Mutex
	err   error

	resMu sync.Mutex
	res   *compileResult

	accessToken      string
	accessCookieName string
}

const watchAccessTokenBytes = 32

type compileResult struct {
	SVG   string   `json:"svg"`
	Scale *float64 `json:"scale,omitEmpty"`
	Err   string   `json:"err"`
}

func newWatcher(ctx context.Context, ms *xmain.State, opts watcherOpts) (*watcher, error) {
	ctx, cancel := context.WithCancel(ctx)
	accessToken, err := newWatchAccessToken()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate watch access token: %w", err)
	}

	w := &watcher{
		ctx:     ctx,
		cancel:  cancel,
		devMode: devMode,

		ms:          ms,
		watcherOpts: opts,

		compileCh: make(chan struct{}, 1),
		wsclients: make(map[*wsclient]struct{}),

		accessToken: accessToken,
	}
	err = w.init()
	if err != nil {
		return nil, err
	}
	return w, nil
}

func newWatchAccessToken() (string, error) {
	b := make([]byte, watchAccessTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
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
	lastModified := make(map[string]time.Time)

	mt, err := w.ensureAddWatch(ctx, w.inputPath)
	if err != nil {
		return err
	}
	lastModified[w.inputPath] = mt
	w.ms.Log.Info.Printf("compiling %v...", w.ms.HumanPath(w.inputPath))
	w.requestCompile()

	eatBurstTimer := time.NewTimer(0)
	<-eatBurstTimer.C
	pollTicker := time.NewTicker(time.Second * 10)
	defer pollTicker.Stop()

	changed := make(map[string]struct{})

	for {
		select {
		case <-pollTicker.C:
			// In case we missed an event indicating the path is unwatchable and we won't be
			// getting any more events.
			// File notification APIs are notoriously unreliable. I've personally experienced
			// many quirks and so feel this check is justified even if excessive.
			missedChanges := false
			for _, watched := range w.fw.WatchList() {
				mt, err := w.ensureAddWatch(ctx, watched)
				if err != nil {
					return err
				}
				if mt2, ok := lastModified[watched]; !ok || !mt.Equal(mt2) {
					missedChanges = true
					lastModified[watched] = mt
				}
			}
			if missedChanges {
				w.requestCompile()
			}
		case ev, ok := <-w.fw.Events:
			if !ok {
				return errors.New("fsnotify watcher closed")
			}
			w.ms.Log.Debug.Printf("received file system event %v", ev)

			if isTemp, reason := isBackupFile(ev.Name); isTemp {
				w.ms.Log.Debug.Printf("skipping event for %q: detected as %s", w.ms.HumanPath(ev.Name), reason)
				continue
			}

			mt, err := w.ensureAddWatch(ctx, ev.Name)
			if err != nil {
				return err
			}
			if ev.Op == fsnotify.Chmod {
				if mt.Equal(lastModified[ev.Name]) {
					// Benign Chmod.
					// See https://github.com/fsnotify/fsnotify/issues/15
					continue
				}
				// We missed changes.
				lastModified[ev.Name] = mt
			}
			changed[ev.Name] = struct{}{}
			// The purpose of eatBurstTimer is to wait at least 16 milliseconds after a sequence of
			// events to ensure that whomever is editing the file is now done.
			//
			// For example, On macOS editing with neovim, every write I see a chmod immediately
			// followed by a write followed by another chmod. We don't want the three events to
			// be treated as two or three compilations, we want them to be batched into one.
			//
			// Another example would be a very large file where one logical edit becomes write
			// events. We wouldn't want to try to compile an incomplete file and then report a
			// misleading error.
			eatBurstTimer.Reset(time.Millisecond * 16)
		case <-eatBurstTimer.C:
			var changedList []string
			for k := range changed {
				changedList = append(changedList, k)
				delete(changed, k)
			}
			sort.Strings(changedList)
			changedStr := w.ms.HumanPath(changedList[0])
			for i := 1; i < len(changedList); i++ {
				changedStr += fmt.Sprintf(", %s", w.ms.HumanPath(changedList[i]))
			}
			w.ms.Log.Info.Printf("detected change in %s: recompiling...", changedStr)
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

// boardStillCurrent reports whether compiledFor is still the board being
// viewed. A compile can take long enough that the user navigates to a
// different board before it finishes; its result must be discarded rather
// than published, or it would overwrite the newly-selected board's cached
// result with stale data.
func (w *watcher) boardStillCurrent(compiledFor string) bool {
	w.boardpathMu.Lock()
	defer w.boardpathMu.Unlock()
	return compiledFor == w.boardPath
}

func (w *watcher) ensureAddWatch(ctx context.Context, path string) (time.Time, error) {
	interval := time.Millisecond * 16
	tc := time.NewTimer(0)
	<-tc.C
	for {
		mt, err := w.addWatch(ctx, path)
		if err == nil {
			return mt, nil
		}
		if interval >= time.Second {
			w.ms.Log.Error.Printf("failed to watch %q: %v (retrying in %v)", w.ms.HumanPath(path), err, interval)
		}

		tc.Reset(interval)
		select {
		case <-tc.C:
			if interval < time.Second {
				interval = time.Second
			}
			if interval < time.Second*16 {
				interval *= 2
			}
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		}
	}
}

func (w *watcher) addWatch(ctx context.Context, path string) (time.Time, error) {
	if isTemp, reason := isBackupFile(path); isTemp {
		w.ms.Log.Debug.Printf("skipping watch for %q: detected as %s", w.ms.HumanPath(path), reason)
		return time.Time{}, nil
	}

	err := w.fw.Add(path)
	if err != nil {
		return time.Time{}, err
	}
	var d os.FileInfo
	d, err = os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return d.ModTime(), nil
}

func (w *watcher) replaceWatchList(ctx context.Context, paths []string) error {
	// First remove the files no longer being watched
	for _, watched := range w.fw.WatchList() {
		if watched == w.inputPath {
			continue
		}
		found := false
		for _, p := range paths {
			if watched == p {
				found = true
				break
			}
		}
		if !found {
			// Don't mind errors here
			w.fw.Remove(watched)
		}
	}
	// Then add the files newly being watched
	for _, p := range paths {
		found := false
		for _, watched := range w.fw.WatchList() {
			if watched == p {
				found = true
				break
			}
		}
		if !found {
			_, err := w.ensureAddWatch(ctx, p)
			if err != nil {
				return err
			}
		}
	}
	return nil
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

		// Watch mode is a trusted local CLI workflow, so it may follow imports
		// anywhere on the host filesystem. Keep that opt-in explicit while using
		// localfile.Policy to reject directories and other special files.
		fs := trackedFS{localFiles: localfile.Unrestricted()}
		w.boardpathMu.Lock()
		compiledBoardPath := w.boardPath
		var boardPath []string
		if compiledBoardPath != "" {
			boardPath = strings.Split(compiledBoardPath, string(os.PathSeparator))
		}
		svg, _, err := compile(ctx, w.ms, &fs, w.layout, w.renderOpts, w.fontFamily, w.monoFontFamily, w.animateInterval, w.inputPath, w.outputPath, boardPath, false, w.bundle, w.forceAppendix, w.outputFormat, w.asciiMode, true)
		w.boardpathMu.Unlock()
		errs := ""
		if err != nil {
			if len(svg) > 0 {
				err = fmt.Errorf("failed to fully %scompile (rendering partial svg): %w", recompiledPrefix, err)
			} else {
				err = fmt.Errorf("failed to %scompile: %w", recompiledPrefix, err)
			}
			errs = err.Error()
			w.ms.Log.Error.Print(errs)
		}
		err = w.replaceWatchList(ctx, fs.opened)
		if err != nil {
			return err
		}

		// handleRoot may have already switched to a different board and cleared
		// w.res while this compile was in flight. Only publish this result if
		// the board it was compiled for is still the one being viewed, so we
		// never hand a newly-connecting client the wrong board's SVG.
		if w.boardStillCurrent(compiledBoardPath) {
			w.broadcast(&compileResult{
				SVG:   string(svg),
				Scale: w.renderOpts.Scale,
				Err:   errs,
			})
		}

		if firstCompile {
			firstCompile = false
			url := w.accessURL("/")
			err = xbrowser.Open(ctx, w.ms.Env, url)
			if err != nil {
				w.ms.Log.Warn.Printf("failed to open browser to %v: %v", url, err)
			}
		}
	}
}

func (w *watcher) listen() error {
	l, err := net.Listen("tcp", net.JoinHostPort(w.host, w.port))
	if err != nil {
		return err
	}
	w.l = l
	_, port, err := net.SplitHostPort(w.l.Addr().String())
	if err != nil {
		_ = w.l.Close()
		return fmt.Errorf("failed to determine watch listener port: %w", err)
	}
	w.accessCookieName = "d2-watch-" + port
	w.ms.Log.Success.Printf("listening on %s", w.accessURL("/"))
	return nil
}

func (w *watcher) accessURL(path string) string {
	return fmt.Sprintf("http://%s%s?token=%s", w.l.Addr(), path, w.accessToken)
}

func (w *watcher) goServe() error {
	m := http.NewServeMux()
	// TODO: Add cmdlog logging and error reporting middleware
	// TODO: Add standard debug/profiling routes
	m.HandleFunc("/", w.handleRoot)
	m.Handle("/static/", http.StripPrefix("/static", w.staticFileServer))
	m.Handle("/watch", xhttp.HandlerFuncAdapter{Log: w.ms.Log, Func: w.handleWatch})

	logged := xhttp.Log(w.ms.Log, m)
	authenticated := http.HandlerFunc(func(hw http.ResponseWriter, r *http.Request) {
		if w.authorizeRequest(hw, r) {
			logged.ServeHTTP(hw, r)
		}
	})
	s := xhttp.NewServer(w.ms.Log.Warn, authenticated)
	w.goFunc(func(ctx context.Context) error {
		return xhttp.Serve(ctx, time.Second*30, s, w.l)
	})

	return nil
}

func (w *watcher) authorizeRequest(hw http.ResponseWriter, r *http.Request) bool {
	query := r.URL.Query()
	if query.Has("token") {
		if !constantTimeTokenEqual(query.Get("token"), w.accessToken) {
			http.Error(hw, "watch access denied", http.StatusForbidden)
			return false
		}

		hw.Header().Set("Cache-Control", "no-store")
		hw.Header().Set("Referrer-Policy", "no-referrer")
		http.SetCookie(hw, &http.Cookie{
			Name:     w.accessCookieName,
			Value:    w.accessToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		query.Del("token")
		redirect := *r.URL
		redirect.Scheme = ""
		redirect.Host = ""
		redirect.RawPath = ""
		redirect.Path = "/" + strings.TrimLeft(redirect.Path, "/")
		redirect.RawQuery = query.Encode()
		http.Redirect(hw, r, redirect.String(), http.StatusSeeOther)
		return false
	}

	if cookie, err := r.Cookie(w.accessCookieName); err == nil &&
		constantTimeTokenEqual(cookie.Value, w.accessToken) {
		return true
	}
	http.Error(hw, "watch access denied", http.StatusForbidden)
	return false
}

func constantTimeTokenEqual(provided, expected string) bool {
	return len(provided) == len(expected) && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
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
	<script src="/static/watch.js"></script>
	<link rel="stylesheet" href="/static/watch.css">
	<link id="favicon" rel="icon" href="/static/favicon.ico">
</head>
<body data-d2-dev-mode=%t>
	<div id="d2-err" style="display: none"></div>
	<div id="d2-svg-container"></div>
</body>
</html>`, filepath.Base(w.outputPath), w.devMode)

	w.boardpathMu.Lock()
	// if path is "/x.svg", we just want "x"
	boardPath := strings.TrimPrefix(r.URL.Path, "/")
	if idx := strings.LastIndexByte(boardPath, '.'); idx != -1 {
		boardPath = boardPath[:idx]
	}
	// if path is "/index", we just want "/"
	boardPath = strings.TrimSuffix(boardPath, "/index")
	if boardPath == "index" {
		boardPath = ""
	}
	recompile := false
	if boardPath != w.boardPath {
		w.boardPath = boardPath
		recompile = true
	}
	w.boardpathMu.Unlock()
	if recompile {
		// Discard the previous board's cached result so a websocket client
		// connecting for this navigation can't receive a stale SVG (with the
		// wrong dimensions) before the new board finishes compiling. Without
		// this, the client's watch.js locks in a fit-to-window scale computed
		// from the old board and never recomputes it once the correct SVG
		// arrives, leaving the diagram mis-scaled.
		w.resMu.Lock()
		w.res = nil
		w.resMu.Unlock()
		w.requestCompile()
	}
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

// trackedFS is OS's FS with the addition that it tracks which files are opened successfully
type trackedFS struct {
	localFiles localfile.Policy
	opened     []string
}

func (tfs *trackedFS) Open(name string) (fs.File, error) {
	f, err := tfs.localFiles.Open(name)
	if err == nil {
		tfs.opened = append(tfs.opened, name)
	}
	return f, err
}

func isBackupFile(path string) (bool, string) {
	ext := filepath.Ext(path)
	baseName := filepath.Base(path)

	// This list is based off of https://github.com/gohugoio/hugo/blob/master/commands/hugobuilder.go#L795
	switch {
	case strings.HasSuffix(ext, "~"):
		return true, "generic backup file (~)"
	case ext == ".swp":
		return true, "vim swap file"
	case ext == ".swx":
		return true, "vim swap file"
	case ext == ".tmp":
		return true, "generic temp file"
	case ext == ".DS_Store":
		return true, "OSX thumbnail"
	case ext == ".bck":
		return true, "Helix backup"
	case baseName == "4913":
		return true, "vim temp file"
	case strings.HasPrefix(ext, ".goutputstream"):
		return true, "GNOME temp file"
	case strings.HasSuffix(ext, "jb_old___"):
		return true, "IntelliJ old backup"
	case strings.HasSuffix(ext, "jb_tmp___"):
		return true, "IntelliJ temp file"
	case strings.HasSuffix(ext, "jb_bak___"):
		return true, "IntelliJ backup"
	case strings.HasPrefix(ext, ".sb-"):
		return true, "Byword temp file"
	case strings.HasPrefix(baseName, ".#"):
		return true, "Emacs lock file"
	case strings.HasPrefix(baseName, "#"):
		return true, "Emacs temp file"
	}
	return false, ""
}
