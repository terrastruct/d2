package d2plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/xmain"
)

// Serve returns a xmain.RunFunc that will invoke the plugin p as necessary to service the
// calling d2 CLI.
//
// See implementation of d2plugin-dagre in the ./cmd directory.
//
// Also see execPlugin in exec.go for the d2 binary plugin protocol.
func Serve(p Plugin) func(context.Context, *xmain.State) error {
	return func(ctx context.Context, ms *xmain.State) (err error) {
		if len(ms.Args) < 1 {
			return errors.New("expected first argument to plugin binary to be function name")
		}
		reqFunc := ms.Args[0]

		switch ms.Args[0] {
		case "info":
			return info(ctx, p, ms)
		case "layout":
			return layout(ctx, p, ms)
		case "postprocess":
			return postProcess(ctx, p, ms)
		case "options":
			return options(ctx, p, ms)
		default:
			return fmt.Errorf("unrecognized command: %s", reqFunc)
		}
	}
}

func info(ctx context.Context, p Plugin, ms *xmain.State) error {
	info, err := p.Info(ctx)
	if err != nil {
		return err
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	_, err = ms.Stdout.Write(b)
	if err != nil {
		return err
	}
	return nil
}

func layout(ctx context.Context, p Plugin, ms *xmain.State) error {
	in, err := io.ReadAll(ms.Stdin)
	if err != nil {
		return err
	}
	var g d2graph.Graph
	if err := d2graph.DeserializeGraph(in, &g); err != nil {
		return fmt.Errorf("failed to unmarshal input to graph: %s", in)
	}
	err = p.Layout(ctx, &g)
	if err != nil {
		return err
	}
	b, err := d2graph.SerializeGraph(&g)
	if err != nil {
		return err
	}
	_, err = ms.Stdout.Write(b)
	if err != nil {
		return err
	}
	return nil
}

func postProcess(ctx context.Context, p Plugin, ms *xmain.State) error {
	in, err := io.ReadAll(ms.Stdin)
	if err != nil {
		return err
	}

	out, err := p.PostProcess(ctx, in)
	if err != nil {
		return err
	}

	_, err = ms.Stdout.Write(out)
	if err != nil {
		return err
	}
	return nil
}

func options(ctx context.Context, p Plugin, ms *xmain.State) error {
	options, err := p.Options(ctx)
	if err != nil {
		return err
	}

	out, err := json.Marshal(options)
	if err != nil {
		return err
	}

	_, err = ms.Stdout.Write(out)
	if err != nil {
		return err
	}
	return nil
}
