package xbrowser

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/pkg/browser"

	"oss.terrastruct.com/xos"
)

func OpenURL(ctx context.Context, env *xos.Env, url string) error {
	browserEnv := env.Getenv("BROWSER")
	if browserEnv != "" {
		browserSh := fmt.Sprintf("%s '$1'", browserEnv)
		cmd := exec.CommandContext(ctx, "sh", "-c", browserSh, "--", url)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to run %v (out: %q): %w", cmd.Args, out, err)
		}
		return nil
	}
	return browser.OpenURL(url)
}
