package d2plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"oss.terrastruct.com/util-go/xdefer"

	"oss.terrastruct.com/d2/d2graph"
)

// execPlugin uses the binary at pathname with the plugin protocol to implement
// the Plugin interface.
//
// The layout plugin protocol works as follows.
//
// Info
// 	1. The binary is invoked with info as the first argument.
// 	2. The stdout of the binary is unmarshalled into PluginInfo.
//
// Layout
// 	1. The binary is invoked with layout as the first argument and the json marshalled
// 	   d2graph.Graph on stdin.
// 	2. The stdout of the binary is unmarshalled into a d2graph.Graph
//
// PostProcess
// 	1. The binary is invoked with postprocess as the first argument and the
// 	bytes of the SVG render on stdin.
// 	2. The stdout of the binary is bytes of SVG with any post-processing.
//
// If any errors occur the binary will exit with a non zero status code and write
// the error to stderr.
type execPlugin struct {
	path string
}

func (p execPlugin) Info(ctx context.Context) (_ *PluginInfo, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.path, "info")
	defer xdefer.Errorf(&err, "failed to run %v", cmd.Args)

	stdout, err := cmd.Output()
	if err != nil {
		ee := &exec.ExitError{}
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%v\nstderr:\n%s", ee, ee.Stderr)
		}
		return nil, err
	}

	var info PluginInfo

	err = json.Unmarshal(stdout, &info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %w", err)
	}

	return &info, nil
}

func (p execPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	graphBytes, err := d2graph.SerializeGraph(g)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, p.path, "layout")

	buffer := bytes.Buffer{}
	buffer.Write(graphBytes)
	cmd.Stdin = &buffer

	stdout, err := cmd.Output()
	if err != nil {
		ee := &exec.ExitError{}
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return fmt.Errorf("%v\nstderr:\n%s", ee, ee.Stderr)
		}
		return err
	}
	err = d2graph.DeserializeGraph(stdout, g)
	if err != nil {
		return fmt.Errorf("failed to unmarshal json: %w", err)
	}

	return nil
}

func (p execPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.path, "postprocess")

	cmd.Stdin = bytes.NewBuffer(in)

	stdout, err := cmd.Output()
	if err != nil {
		ee := &exec.ExitError{}
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%v\nstderr:\n%s", ee, ee.Stderr)
		}
		return nil, err
	}

	return stdout, nil
}
