package xhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"oss.terrastruct.com/cmdlog"
)

// Error represents an HTTP error.
// It's exported only for comparison in tests.
type Error struct {
	Code int
	Resp interface{}
	Err  error
}

var _ interface {
	Is(error) bool
	Unwrap() error
} = Error{}

// Errorf creates a new error with code, resp, msg and v.
//
// When returned from an xhttp.HandlerFunc, it will be correctly logged
// and written to the connection. See xhttp.WrapHandlerFunc
func Errorf(code int, resp interface{}, msg string, v ...interface{}) error {
	return errorWrap(code, resp, fmt.Errorf(msg, v...))
}

// ErrorWrap wraps err with the code and resp for xhttp.HandlerFunc.
//
// When returned from an xhttp.HandlerFunc, it will be correctly logged
// and written to the connection. See xhttp.WrapHandlerFunc
func ErrorWrap(code int, resp interface{}, err error) error {
	return errorWrap(code, resp, err)
}

func errorWrap(code int, resp interface{}, err error) error {
	if resp == nil {
		resp = http.StatusText(code)
	}
	return Error{code, resp, err}
}

func (e Error) Unwrap() error {
	return e.Err
}

func (e Error) Is(err error) bool {
	e2, ok := err.(Error)
	if !ok {
		return false
	}
	return e.Code == e2.Code && e.Resp == e2.Resp && errors.Is(e.Err, e2.Err)
}

func (e Error) Error() string {
	return fmt.Sprintf("http error with code %v and resp %#v: %v", e.Code, e.Resp, e.Err)
}

// HandlerFunc is like http.HandlerFunc but returns an error.
// See Errorf and ErrorWrap.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

type HandlerFuncAdapter struct {
	Log  *cmdlog.Logger
	Func HandlerFunc
}

// ServeHTTP adapts xhttp.HandlerFunc into http.Handler for usage with standard
// HTTP routers like chi.
//
// It logs and writes any error from xhttp.HandlerFunc to the connection.
//
// If err was created with xhttp.Errorf or wrapped with xhttp.WrapError, then the error
// will be logged at the correct level for the status code and xhttp.JSON will be called
// with the code and resp.
//
// 400s are logged as warns and 500s as errors.
//
// If the error was not created with the xhttp helpers then a 500 will be written.
//
// If resp is nil, then resp is set to http.StatusText(code)
//
// If the code is not a 400 or a 500, then an error about about the unexpected error code
// will be logged and a 500 will be written. The original error will also be logged.
func (a HandlerFuncAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := a.Func(w, r)
		if err != nil {
			handleError(a.Log, w, err)
		}
	})

	h.ServeHTTP(w, r)
}

func handleError(clog *cmdlog.Logger, w http.ResponseWriter, err error) {
	var herr Error
	ok := errors.As(err, &herr)
	if !ok {
		herr = ErrorWrap(http.StatusInternalServerError, nil, err).(Error)
	}

	var logger *log.Logger
	switch {
	case 400 <= herr.Code && herr.Code < 500:
		logger = clog.Warn
	case 500 <= herr.Code && herr.Code < 600:
		logger = clog.Error
	default:
		logger = clog.Error

		clog.Error.Printf("unexpected non error http status code %d with resp: %#v", herr.Code, herr.Resp)

		herr.Code = http.StatusInternalServerError
		herr.Resp = nil
	}

	if herr.Resp == nil {
		herr.Resp = http.StatusText(herr.Code)
	}

	logger.Printf("error handling http request: %v", err)

	ww, ok := w.(writtenResponseWriter)
	if !ok {
		clog.Warn.Printf("response writer does not implement Written, double write logs possible: %#v", w)
	} else if ww.Written() {
		// Avoid double writes if an error occurred while the response was
		// being written.
		return
	}

	JSON(clog, w, herr.Code, map[string]interface{}{
		"error": herr.Resp,
	})
}

type writtenResponseWriter interface {
	Written() bool
}

func JSON(clog *cmdlog.Logger, w http.ResponseWriter, code int, v interface{}) {
	if v == nil {
		v = map[string]interface{}{
			"status": http.StatusText(code),
		}
	}

	b, err := json.Marshal(v)
	if err != nil {
		clog.Error.Printf("json marshal error: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}
