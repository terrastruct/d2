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
	args  []string
	flags *pflag.FlagSet
	env   *xos.Env
	log   *cmdlog.Logger

	registeredEnvs []string
}

func NewOpts(env *xos.Env, args []string, log *cmdlog.Logger) *Opts {
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)
	flags.SortFlags = false
	flags.Usage = func() {}
	flags.SetOutput(io.Discard)
	return &Opts{
		args:  args,
		flags: flags,
		env:   env,
		log:   log,
	}
}

func (o *Opts) Help() string {
	b := &strings.Builder{}
	o.flags.SetOutput(b)
	o.flags.PrintDefaults()

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

func (o *Opts) Int64(envKey, flag, shortFlag string, defaultVal int64, usage string) *int64 {
	if envKey != "" {
		if o.env.Getenv(envKey) != "" {
			envVal, err := strconv.ParseInt(o.env.Getenv(envKey), 10, 64)
			if err != nil {
				o.log.Error.Printf(`ignoring invalid environment variable %s. Expected int64. Found "%v".`, envKey, envVal)
			} else if envVal != defaultVal {
				defaultVal = envVal
			}
		}
		o.registeredEnvs = append(o.registeredEnvs, envKey)
	}

	return o.flags.Int64P(flag, shortFlag, defaultVal, usage)
}

func (o *Opts) String(envKey, flag, shortFlag string, defaultVal, usage string) *string {
	if envKey != "" {
		if o.env.Getenv(envKey) != "" {
			envVal := o.env.Getenv(envKey)
			if envVal != defaultVal {
				defaultVal = envVal
			}
		}
		o.registeredEnvs = append(o.registeredEnvs, envKey)
	}

	return o.flags.StringP(flag, shortFlag, defaultVal, usage)
}

func (o *Opts) Bool(envKey, flag, shortFlag string, defaultVal bool, usage string) *bool {
	if envKey != "" {
		if o.env.Getenv(envKey) != "" {
			envVal := o.env.Getenv(envKey)
			if !boolyEnv(envVal) {
				o.log.Error.Printf(`ignoring invalid environment variable %s. Expected bool. Found "%s".`, envKey, envVal)
			} else if (defaultVal && falseyEnv(envVal)) ||
				(!defaultVal && truthyEnv(envVal)) {
				defaultVal = !defaultVal
			}
		}
		o.registeredEnvs = append(o.registeredEnvs, envKey)
	}

	return o.flags.BoolP(flag, shortFlag, defaultVal, usage)
}

func boolyEnv(s string) bool {
	return falseyEnv(s) || truthyEnv(s)
}

func falseyEnv(s string) bool {
	return s == "0" || s == "false" || s == "f"
}

func truthyEnv(s string) bool {
	return s == "1" || s == "true" || s == "t"
}

func (o *Opts) Parse() error {
	err := o.flags.Parse(o.args)
	if err != nil {
		return err
	}
	return nil
}

func (o *Opts) SetArgs(args []string) {
	o.args = args
}

func (o *Opts) Args() []string {
	return o.flags.Args()
}

func (o *Opts) Arg(i int) string {
	return o.flags.Arg(i)
}
