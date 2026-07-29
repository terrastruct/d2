package d2cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/version"
)

func help(ms *xmain.State) {
	fmt.Fprintf(ms.Stdout, `%[1]s %[2]s
Usage:
  %[1]s [--watch=false] [--theme=0] file.d2 [file.svg | file.png | file.pdf | file.pptx | file.gif | file.txt]
  %[1]s layout [name]
  %[1]s fmt file.d2 ...
  %[1]s play [--theme=0] [--sketch] file.d2
  %[1]s validate file.d2

%[1]s compiles and renders file.d2 to file.svg | file.png | file.pdf | file.pptx | file.gif | file.txt
It defaults to file.svg if an output path is not provided.

Use - to have d2 read from stdin or write to stdout.

See man d2 for more detailed docs.

Flags:
%[3]s

Subcommands:
  %[1]s layout - Lists available layout engine options with short help
  %[1]s layout [name] - Display long help for a particular layout engine, including its configuration options
  %[1]s themes - Lists available themes
  %[1]s fmt file.d2 ... - Format passed files
	%[1]s play file.d2 - Opens the file in playground, an online web viewer (https://play.d2lang.com)
  %[1]s validate file.d2  - Validates file.d2

See more docs and the source code at https://oss.terrastruct.com/d2.
Hosted icons at https://icons.terrastruct.com.
Playground runner at https://play.d2lang.com.
`, filepath.Base(ms.Name), version.Version, ms.Opts.Defaults())
}

func layoutCmd(ctx context.Context, ms *xmain.State, ps []d2plugin.Plugin) error {
	if len(ms.Opts.Flags.Args()) == 1 {
		return shortLayoutHelp(ctx, ms, ps)
	} else if len(ms.Opts.Flags.Args()) == 2 {
		return longLayoutHelp(ctx, ms, ps)
	} else {
		return pluginSubcommand(ctx, ms, ps)
	}
}

func themesCmd(_ context.Context, ms *xmain.State) {
	fmt.Fprintf(ms.Stdout, "Available themes:\n%s", d2themescatalog.CLIString())
}

func shortLayoutHelp(ctx context.Context, ms *xmain.State, ps []d2plugin.Plugin) error {
	var pluginLines []string
	pinfos, err := d2plugin.ListPluginInfos(ctx, ps)
	if err != nil {
		return err
	}
	for _, p := range pinfos {
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
  To use a particular layout engine, set the environment variable D2_LAYOUT=[name] or flag --layout=[name].

Example:
  D2_LAYOUT=dagre d2 in.d2 out.svg

Subcommands:
  %s layout [layout name] - Display long help for a particular layout engine, including its configuration options

See more docs at https://d2lang.com/tour/layouts
`, strings.Join(pluginLines, "\n"), ms.Name)
	return nil
}

func longLayoutHelp(ctx context.Context, ms *xmain.State, ps []d2plugin.Plugin) error {
	layout := ms.Opts.Flags.Arg(1)
	plugin, err := d2plugin.FindPlugin(ctx, ps, layout)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return layoutNotFound(ctx, ps, layout)
		}
		return err
	}

	pinfo, err := plugin.Info(ctx)
	if err != nil {
		return err
	}

	plocation := pinfo.Type
	if pinfo.Type == "binary" {
		plocation = fmt.Sprintf("executable plugin at %s", humanPath(pinfo.Path))
	}

	if !strings.HasSuffix(pinfo.LongHelp, "\n") {
		pinfo.LongHelp += "\n"
	}
	fmt.Fprintf(ms.Stdout, `%s (%s):

%s`, pinfo.Name, plocation, pinfo.LongHelp)

	return nil
}

func layoutNotFound(ctx context.Context, ps []d2plugin.Plugin, layout string) error {
	pinfos, err := d2plugin.ListPluginInfos(ctx, ps)
	if err != nil {
		return err
	}
	var names []string
	for _, p := range pinfos {
		names = append(names, p.Name)
	}

	return xmain.UsageErrorf(`D2_LAYOUT "%s" is not bundled and could not be found in your $PATH.
The available options are: %s. For details on each option, run "d2 layout".

For more information on setup, please visit https://github.com/terrastruct/d2.`,
		layout, strings.Join(names, ", "))
}

func pluginSubcommand(ctx context.Context, ms *xmain.State, ps []d2plugin.Plugin) error {
	layout := ms.Opts.Flags.Arg(1)
	plugin, err := d2plugin.FindPlugin(ctx, ps, layout)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return layoutNotFound(ctx, ps, layout)
		}
		return err
	}

	ms.Opts.Args = ms.Opts.Flags.Args()[2:]
	return d2plugin.Serve(plugin)(ctx, ms)
}

func humanPath(fp string) string {
	if strings.HasPrefix(fp, os.Getenv("HOME")) {
		return filepath.Join("~", strings.TrimPrefix(fp, os.Getenv("HOME")))
	}
	return fp
}
