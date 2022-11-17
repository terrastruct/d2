package xmain

import (
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

	registeredEnvs []string
}

func NewOpts(env *xos.Env, log *cmdlog.Logger, args []string) *Opts {
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)
	flags.SortFlags = false
	flags.Usage = func() {}
	flags.SetOutput(io.Discard)
	return &Opts{
		Args:  args,
		Flags: flags,
		env:   env,
		log:   log,
	}
}

func (o *Opts) Help() string {
	b := &strings.Builder{}
	o.Flags.SetOutput(b)
	o.Flags.PrintDefaults()

	if len(o.registeredEnvs) > 0 {
		b.WriteString("\nYou may persistently set the following as environment variables (flags take precedent):\n")
		for i, e := range o.registeredEnvs {
			s := fmt.Sprintf("- $%s", e)
			if i != len(o.registeredEnvs)-1 {
				s += "\n"
			}
			b.WriteString(s)
		}
	}

	return b.String()
}

func (o *Opts) getEnv(k string) string {
	if k != "" {
		o.registeredEnvs = append(o.registeredEnvs, k)
		return o.env.Getenv(k)
	}
	return ""
}

func (o *Opts) Int64(envKey, flag, shortFlag string, defaultVal int64, usage string) (*int64, error) {
	if env := o.getEnv(envKey); env != "" {
		envVal, err := strconv.ParseInt(env, 10, 64)
		if err != nil {
			return nil, fmt.Errorf(`invalid environment variable %s. Expected int64. Found "%v".`, envKey, envVal)
		}
		defaultVal = envVal
	}

	return o.Flags.Int64P(flag, shortFlag, defaultVal, usage), nil
}

func (o *Opts) String(envKey, flag, shortFlag string, defaultVal, usage string) *string {
	if env := o.getEnv(envKey); env != "" {
		defaultVal = env
	}

	return o.Flags.StringP(flag, shortFlag, defaultVal, usage)
}

func (o *Opts) Bool(envKey, flag, shortFlag string, defaultVal bool, usage string) (*bool, error) {
	if env := o.getEnv(envKey); env != "" {
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
