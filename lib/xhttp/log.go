package xhttp

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"golang.org/x/text/message"

	"oss.terrastruct.com/cmdlog"
)

type ResponseWriter interface {
	http.ResponseWriter
	http.Hijacker
	http.Flusher
	writtenResponseWriter
}

var _ ResponseWriter = &responseWriter{}

type responseWriter struct {
	rw http.ResponseWriter

	written bool
	status  int
	length  int
}

func (rw *responseWriter) Header() http.Header {
	return rw.rw.Header()
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	if !rw.written {
		rw.written = true
		rw.status = statusCode
	}
	rw.rw.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	if !rw.written && len(p) > 0 {
		rw.written = true
		if rw.status == 0 {
			rw.status = http.StatusOK
		}
	}
	rw.length += len(p)
	return rw.rw.Write(p)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := rw.rw.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying response writer does not implement http.Hijacker: %T", rw.rw)
	}
	return hj.Hijack()
}

func (rw *responseWriter) Flush() {
	f, ok := rw.rw.(http.Flusher)
	if !ok {
		return
	}
	f.Flush()
}

func (rw *responseWriter) Written() bool {
	return rw.written
}

func Log(clog *cmdlog.Logger, next http.Handler) http.Handler {
	englishPrinter := message.NewPrinter(message.MatchLanguage("en"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec != nil {
				clog.Error.Printf("caught panic: %#v\n%s", rec, debug.Stack())
				JSON(clog, w, http.StatusInternalServerError, map[string]interface{}{
					"error": http.StatusText(http.StatusInternalServerError),
				})
			}
		}()

		rw := &responseWriter{
			rw: w,
		}

		start := time.Now()
		next.ServeHTTP(rw, r)
		dur := time.Since(start)

		if !rw.Written() {
			_, err := rw.Write(nil)
			if errors.Is(err, http.ErrHijacked) {
				clog.Success.Printf("%s %s %v: hijacked", r.Method, r.URL, dur)
				return
			}

			clog.Warn.Printf("%s %s %v: no response written", r.Method, r.URL, dur)
			return
		}

		var statusLogger *log.Logger
		switch {
		case 100 <= rw.status && rw.status <= 299:
			statusLogger = clog.Success
		case 300 <= rw.status && rw.status <= 399:
			statusLogger = clog.Info
		case 400 <= rw.status && rw.status <= 499:
			statusLogger = clog.Warn
		case 500 <= rw.status && rw.status <= 599:
			statusLogger = clog.Error
		}
		lengthStr := englishPrinter.Sprint(rw.length)
		// TODO: make work with watch.go on hijack, not after
		statusLogger.Printf("%s %s %d %sB %v", r.Method, r.URL, rw.status, lengthStr, dur)
	})
}
