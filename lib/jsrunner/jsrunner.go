package jsrunner

import "context"

type Engine int

const (
	Goja Engine = iota
	Native
)

type JSRunner interface {
	RunString(code string) (JSValue, error)
	NewObject() JSObject
	Set(name string, value interface{}) error
	WaitPromise(ctx context.Context, val JSValue) (interface{}, error)
	Engine() Engine
	MustGet(string) (JSValue, error)
}

type JSValue interface {
	String() string
	Export() interface{}
}

type JSObject interface {
	JSValue
}
