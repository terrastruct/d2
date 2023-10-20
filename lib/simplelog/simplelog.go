// Package simplelog contains a very simple interface for logging strings at either Debug, Info, or Error levels
package simplelog

import (
	"context"

	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/util-go/cmdlog"
)

type Logger interface {
	Debug(string)
	Info(string)
	Error(string)
}

type logger struct {
	logDebug *func(string)
	logInfo  *func(string)
	logError *func(string)
}

func (l logger) Debug(s string) {
	if l.logDebug != nil {
		(*l.logDebug)(s)
	}
}
func (l logger) Info(s string) {
	if l.logInfo != nil {
		(*l.logInfo)(s)
	}
}
func (l logger) Error(s string) {
	if l.logError != nil {
		(*l.logError)(s)
	}
}

func Make(logDebug, logInfo, logError *func(string)) Logger {
	return logger{
		logDebug: logDebug,
		logInfo:  logInfo,
		logError: logError,
	}
}

func FromLibLog(ctx context.Context) Logger {
	lDebug := func(s string) {
		log.Debug(ctx, s)
	}
	lInfo := func(s string) {
		log.Info(ctx, s)
	}
	lError := func(s string) {
		log.Error(ctx, s)
	}
	return Make(&lDebug, &lInfo, &lError)
}

func FromCmdLog(cl *cmdlog.Logger) Logger {
	lDebug := func(s string) {
		cl.Debug.Print(s)
	}
	lInfo := func(s string) {
		cl.Info.Print(s)
	}
	lError := func(s string) {
		cl.Error.Print(s)
	}
	return Make(&lDebug, &lInfo, &lError)
}
