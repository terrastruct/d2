package d2cli

import (
	"context"
	"fmt"

	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/util-go/xdefer"
	"oss.terrastruct.com/util-go/xmain"
)

func validateCmd(ctx context.Context, ms *xmain.State) (err error) {
	defer xdefer.Errorf(&err, "")

	ms.Opts = xmain.NewOpts(ms.Env, ms.Opts.Flags.Args()[1:])
	if len(ms.Opts.Args) == 0 {
		return xmain.UsageErrorf("validate must be passed an input file")
	}

	inputPath := ms.Opts.Args[0]
	if inputPath != "-" {
		inputPath = ms.AbsPath(inputPath)
	}

	input, err := ms.ReadPath(inputPath)
	if err != nil {
		return err
	}

	_, err = d2lib.Parse(ctx, string(input), nil)
	if err != nil {
		return err
	}

	if inputPath == "-" {
		inputPath = "Input"
	}

	fmt.Printf("Success! [%s] is valid D2.\n", inputPath)
	return nil
}
