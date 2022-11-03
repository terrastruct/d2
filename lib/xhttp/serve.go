// Package xhttp implements http helpers.
package xhttp

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"oss.terrastruct.com/xcontext"
)

func NewServer(log *log.Logger, h http.Handler) *http.Server {
	return &http.Server{
		MaxHeaderBytes: 1 << 18, // 262,144B
		ReadTimeout:    time.Minute,
		WriteTimeout:   time.Minute,
		IdleTimeout:    time.Hour,
		ErrorLog:       log,
		Handler:        http.MaxBytesHandler(h, 1<<20), // 1,048,576B
	}
}

func Serve(ctx context.Context, shutdownTimeout time.Duration, s *http.Server, l net.Listener) error {
	s.BaseContext = func(net.Listener) context.Context {
		return ctx
	}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(l)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		ctx = xcontext.WithoutCancel(ctx)
		ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()
		return s.Shutdown(ctx)
	}
}
