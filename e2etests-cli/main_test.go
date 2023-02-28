package e2etests_cli

import (
	"context"
	"testing"
	"time"
)

func TestCLI_E2E(t *testing.T) {
	t.Parallel()

	tca := []struct {
		name string
		run  func(t *testing.T, ctx context.Context)
	}{
		{
			name: "hello_world",
			run:  func(t *testing.T, ctx context.Context) {},
		},
	}

	ctx := context.Background()
	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
			defer cancel()

			tc.run(t, ctx)
		})
	}
}
