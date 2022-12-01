package main

import (
	"bytes"
	"context"

	"oss.terrastruct.com/xdefer"

	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/lib/xmain"
)

func autofmt(ctx context.Context, ms *xmain.State) (err error) {
	defer xdefer.Errorf(&err, "autofmt failed")

	ms.Opts = xmain.NewOpts(ms.Env, ms.Log, ms.Opts.Flags.Args()[1:])
	if len(ms.Opts.Args) == 0 {
		return xmain.UsageErrorf("fmt must be passed the file to be formatted")
	} else if len(ms.Opts.Args) > 1 {
		return xmain.UsageErrorf("fmt only accepts one argument for the file to be formatted")
	}

	inputPath := ms.Opts.Args[0]
	input, err := ms.ReadPath(inputPath)
	if err != nil {
		return err
	}

	m, err := d2parser.Parse(inputPath, bytes.NewReader(input), nil)
	if err != nil {
		return err
	}

	return ms.WritePath(inputPath, []byte(d2format.Format(m)))
}
