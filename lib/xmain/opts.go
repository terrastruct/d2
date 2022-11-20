package xmain

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"oss.terrastruct.com/cmdlog"
	"oss.terrastruct.com/xos"
)

type Opts struct {
	Args  []string
	Flags *pflag.FlagSet
	env   *xos.Env
	log   *cmdlog.Logger

	flagEnv map[string]string
}

func NewOpts(env *xos.Env, log *cmdlog.Logger, args []string) *Opts {
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)
	flags.SortFlags = false
	flags.Usage = func() {}
	flags.SetOutput(io.Discard)
	return &Opts{
		Args:    args,
		Flags:   flags,
		env:     env,
		log:     log,
		flagEnv: make(map[string]string),
	}
}

// Mostly copy pasted pasted from pflag.FlagUsagesWrapped
// with modifications for env var
func (o *Opts) Defaults() string {
	buf := new(bytes.Buffer)

	var lines []string

	maxlen := 0
	maxEnvLen := 0
	o.Flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}

		line := ""
		if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
			line = fmt.Sprintf("  -%s, --%s", flag.Shorthand, flag.Name)
		} else {
			line = fmt.Sprintf("      --%s", flag.Name)
		}

		varname, usage := pflag.UnquoteUsage(flag)
		if varname != "" {
			line += " " + varname
		}
		if flag.NoOptDefVal != "" {
			switch flag.Value.Type() {
			case "string":
				line += fmt.Sprintf("[=\"%s\"]", flag.NoOptDefVal)
			case "bool":
				if flag.NoOptDefVal != "true" {
					line += fmt.Sprintf("[=%s]", flag.NoOptDefVal)
				}
			case "count":
				if flag.NoOptDefVal != "+1" {
					line += fmt.Sprintf("[=%s]", flag.NoOptDefVal)
				}
			default:
				line += fmt.Sprintf("[=%s]", flag.NoOptDefVal)
			}
		}

		line += "\x00"

		if len(line) > maxlen {
			maxlen = len(line)
		}

		if e, ok := o.flagEnv[flag.Name]; ok {
			line += fmt.Sprintf("$%s", e)
		}

		line += "\x01"

		if len(line) > maxEnvLen {
			maxEnvLen = len(line)
		}

		line += usage
		if flag.Value.Type() == "string" {
			line += fmt.Sprintf(" (default %q)", flag.DefValue)
		} else {
			line += fmt.Sprintf(" (default %s)", flag.DefValue)
		}
		if len(flag.Deprecated) != 0 {
			line += fmt.Sprintf(" (DEPRECATED: %s)", flag.Deprecated)
		}

		lines = append(lines, line)
	})

	for _, line := range lines {
		sidx1 := strings.Index(line, "\x00")
		sidx2 := strings.Index(line, "\x01")
		spacing1 := strings.Repeat(" ", maxlen-sidx1)
		spacing2 := strings.Repeat(" ", (maxEnvLen-maxlen)-sidx2+sidx1)
		fmt.Fprintln(buf, line[:sidx1], spacing1, line[sidx1+1:sidx2], spacing2, wrap(maxEnvLen+3, 0, line[sidx2+1:]))
	}

	return buf.String()
}

func (o *Opts) getEnv(flag, k string) string {
	if k != "" {
		o.flagEnv[flag] = k
		return o.env.Getenv(k)
	}
	return ""
}

func (o *Opts) Int64(envKey, flag, shortFlag string, defaultVal int64, usage string) (*int64, error) {
	if env := o.getEnv(flag, envKey); env != "" {
		envVal, err := strconv.ParseInt(env, 10, 64)
		if err != nil {
			return nil, fmt.Errorf(`invalid environment variable %s. Expected int64. Found "%v".`, envKey, envVal)
		}
		defaultVal = envVal
	}

	return o.Flags.Int64P(flag, shortFlag, defaultVal, usage), nil
}

func (o *Opts) String(envKey, flag, shortFlag string, defaultVal, usage string) *string {
	if env := o.getEnv(flag, envKey); env != "" {
		defaultVal = env
	}

	return o.Flags.StringP(flag, shortFlag, defaultVal, usage)
}

func (o *Opts) Bool(envKey, flag, shortFlag string, defaultVal bool, usage string) (*bool, error) {
	if env := o.getEnv(flag, envKey); env != "" {
		if !boolyEnv(env) {
			return nil, fmt.Errorf(`invalid environment variable %s. Expected bool. Found "%s".`, envKey, env)
		}
		if truthyEnv(env) {
			defaultVal = true
		} else {
			defaultVal = false
		}
	}

	return o.Flags.BoolP(flag, shortFlag, defaultVal, usage), nil
}

func boolyEnv(s string) bool {
	return falseyEnv(s) || truthyEnv(s)
}

func falseyEnv(s string) bool {
	return s == "0" || s == "false"
}

func truthyEnv(s string) bool {
	return s == "1" || s == "true"
}
