package main

import (
	"bytes"
	"context"

	"oss.terrastruct.com/util-go/xdefer"

	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
)

func fmtCmd(ctx context.Context, ms *xmain.State) (err error) {
	defer xdefer.Errorf(&err, "failed to fmt")

	ms.Opts = xmain.NewOpts(ms.Env, ms.Log, ms.Opts.Flags.Args()[1:])
	if len(ms.Opts.Args) == 0 {
		return xmain.UsageErrorf("fmt must be passed the file to be formatted")
	} else if len(ms.Opts.Args) > 1 {
		return xmain.UsageErrorf("fmt accepts only one argument for the file to be formatted")
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
