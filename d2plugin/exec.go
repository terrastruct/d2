package d2plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"oss.terrastruct.com/util-go/xdefer"
	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2graph"
	timelib "oss.terrastruct.com/d2/lib/time"
)

// execPlugin uses the binary at pathname with the plugin protocol to implement
// the Plugin interface.
//
// The layout plugin protocol works as follows.
//
// Info
//  1. The binary is invoked with info as the first argument.
//  2. The stdout of the binary is unmarshalled into PluginInfo.
//
// Layout
//  1. The binary is invoked with layout as the first argument and the json marshalled
//     d2graph.Graph on stdin.
//  2. The stdout of the binary is unmarshalled into a d2graph.Graph
//
// PostProcess
//  1. The binary is invoked with postprocess as the first argument and the
//     bytes of the SVG render on stdin.
//  2. The stdout of the binary is bytes of SVG with any post-processing.
//
// If any errors occur the binary will exit with a non zero status code and write
// the error to stderr.
type execPlugin struct {
	path string
	opts map[string]string
	info *PluginInfo
}

func (p *execPlugin) Flags(ctx context.Context) (_ []PluginSpecificFlag, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.path, "flags")
	defer xdefer.Errorf(&err, "failed to run %v", cmd.Args)

	stdout, err := cmd.Output()
	if err != nil {
		ee := &exec.ExitError{}
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%v\nstderr:\n%s", ee, ee.Stderr)
		}
		return nil, err
	}

	var flags []PluginSpecificFlag

	err = json.Unmarshal(stdout, &flags)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %w", err)
	}

	return flags, nil
}

func (p *execPlugin) HydrateOpts(opts []byte) error {
	if opts != nil {
		var execOpts map[string]interface{}
		err := json.Unmarshal(opts, &execOpts)
		if err != nil {
			return xmain.UsageErrorf("non-exec layout options given for exec")
		}

		allString := make(map[string]string)
		for k, v := range execOpts {
			switch vt := v.(type) {
			case string:
				allString[k] = vt
			case int64:
				allString[k] = strconv.Itoa(int(vt))
			case []interface{}:
				str := make([]string, len(vt))
				for i, v := range vt {
					switch vt := v.(type) {
					case string:
						str[i] = vt
					case int64:
						str[i] = strconv.Itoa(int(vt))
					case float64:
						str[i] = strconv.Itoa(int(vt))
					}
				}
				allString[k] = strings.Join(str, ",")
			case []int64:
				str := make([]string, len(vt))
				for i, v := range vt {
					str[i] = strconv.Itoa(int(v))
				}
				allString[k] = strings.Join(str, ",")
			case float64:
				allString[k] = strconv.Itoa(int(vt))
			}
		}

		p.opts = allString
	}
	return nil
}

func (p *execPlugin) Info(ctx context.Context) (_ *PluginInfo, err error) {
	if p.info != nil {
		return p.info, nil
	}

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

	info.Type = "binary"
	info.Path = p.path

	p.info = &info
	return &info, nil
}

func (p *execPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	ctx, cancel := timelib.WithTimeout(ctx, time.Minute)
	defer cancel()

	graphBytes, err := d2graph.SerializeGraph(g)
	if err != nil {
		return err
	}

	args := []string{"layout"}
	for k, v := range p.opts {
		args = append(args, fmt.Sprintf("--%s", k), v)
	}
	cmd := exec.CommandContext(ctx, p.path, args...)

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

func (p *execPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
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
