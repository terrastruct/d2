package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/lib/xmain"
)

func help(ms *xmain.State) {
	fmt.Fprintf(ms.Stdout, `Usage:
  %s [--watch=false] [--theme=0] file.d2 [file.svg]

%[1]s compiles and renders file.d2 to file.svg
Use - to have d2 read from stdin or write to stdout.

Flags:
%s

You may persistently set the following as environment variables:
- $D2_WATCH
- $D2_BUNDLE
- $D2_LAYOUT
- $D2_THEME
- $DEBUG

Subcommands:
  %[1]s layout - Lists available layout engine options with short help
  %[1]s layout [layout name] - Display long help for a particular layout engine

See more docs at https://oss.terrastruct.com/d2
`, ms.Name, ms.FlagHelp())
}

func layoutHelp(ctx context.Context, ms *xmain.State) error {
	if len(ms.FlagSet.Args()) == 1 {
		return shortLayoutHelp(ctx, ms)
	} else if len(ms.FlagSet.Args()) == 2 {
		return longLayoutHelp(ctx, ms)
	} else {
		return pluginSubcommand(ctx, ms)
	}
}

func shortLayoutHelp(ctx context.Context, ms *xmain.State) error {
	var pluginLines []string
	plugins, err := d2plugin.ListPlugins(ctx)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		var l string
		if p.Type == "bundled" {
			l = fmt.Sprintf("%s (bundled) - %s", p.Name, p.ShortHelp)
		} else {
			l = fmt.Sprintf("%s (%s) - %s", p.Name, humanPath(p.Path), p.ShortHelp)
		}
		pluginLines = append(pluginLines, l)
	}
	fmt.Fprintf(ms.Stdout, `Available layout engines found:

%s

Usage:
  To use a particular layout engine, set the environment variable D2_LAYOUT=[layout name].

Example:
  D2_LAYOUT=dagre d2 in.d2 out.svg

Subcommands:
  %s layout [layout name] - Display long help for a particular layout engine

See more docs at https://oss.terrastruct.com/d2
`, strings.Join(pluginLines, "\n"), ms.Name)
	return nil
}

func longLayoutHelp(ctx context.Context, ms *xmain.State) error {
	layout := ms.FlagSet.Arg(1)
	plugin, path, err := d2plugin.FindPlugin(ctx, layout)
	if errors.Is(err, exec.ErrNotFound) {
		return layoutNotFound(ctx, layout)
	}

	pluginLocation := "bundled"
	if path != "" {
		pluginLocation = fmt.Sprintf("executable plugin at %s", humanPath(path))
	}

	pluginInfo, err := plugin.Info(ctx)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(pluginInfo.LongHelp, "\n") {
		pluginInfo.LongHelp += "\n"
	}
	fmt.Fprintf(ms.Stdout, `%s (%s):

%s`, pluginInfo.Name, pluginLocation, pluginInfo.LongHelp)

	return nil
}

func layoutNotFound(ctx context.Context, layout string) error {
	var names []string
	plugins, err := d2plugin.ListPlugins(ctx)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		names = append(names, p.Name)
	}

	return xmain.UsageErrorf(`D2_LAYOUT "%s" is not bundled and could not be found in your $PATH.
The available options are: %s. For details on each option, run "d2 layout".

For more information on setup, please visit https://github.com/terrastruct/d2.`,
		layout, strings.Join(names, ", "))
}

func pluginSubcommand(ctx context.Context, ms *xmain.State) error {
	layout := ms.FlagSet.Arg(1)
	plugin, _, err := d2plugin.FindPlugin(ctx, layout)
	if errors.Is(err, exec.ErrNotFound) {
		return layoutNotFound(ctx, layout)
	}

	ms.Args = ms.FlagSet.Args()[2:]
	return d2plugin.Serve(plugin)(ctx, ms)
}

func humanPath(fp string) string {
	if strings.HasPrefix(fp, os.Getenv("HOME")) {
		return filepath.Join("~", strings.TrimPrefix(fp, os.Getenv("HOME")))
	}
	return fp
}
