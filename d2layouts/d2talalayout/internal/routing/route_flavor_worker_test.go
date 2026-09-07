package routing

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
)

func routeFlavorTestRouters() []*ovgEdgeRouter {
	return []*ovgEdgeRouter{
		{flavor: ShortestToLongest},
		{flavor: LongestToShortest},
		{flavor: Default},
	}
}

type sensitiveRoutePanic struct {
	formatted *atomic.Bool
}

func (p sensitiveRoutePanic) String() string {
	p.formatted.Store(true)
	return "sensitive payload\ngoroutine 1 [running]:"
}

func TestRouteFlavorPanicIsFatalDespiteSuccess(t *testing.T) {
	var completed atomic.Int32
	var panicFormatted atomic.Bool
	worker := func(router *ovgEdgeRouter, _ context.Context, _ bool) GenerateRouteResponse {
		if router.flavor == ShortestToLongest {
			panic(sensitiveRoutePanic{formatted: &panicFormatted})
		}
		completed.Add(1)
		return GenerateRouteResponse{Distance: float64(completed.Load())}
	}

	responses, err := generateRouteFlavorResponsesWith(context.Background(), routeFlavorTestRouters(), false, worker)
	if responses != nil {
		t.Fatalf("responses = %v, want nil", responses)
	}
	if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("error = %v, want invariant.ErrViolation", err)
	}
	if !strings.Contains(err.Error(), "ShortestToLongest") {
		t.Fatalf("error = %q, want flavor", err)
	}
	if strings.Contains(err.Error(), "sensitive payload") || strings.Contains(err.Error(), "goroutine ") {
		t.Fatalf("error exposes the recovered panic payload: %q", err)
	}
	if panicFormatted.Load() {
		t.Fatal("recovered panic payload was formatted")
	}
	if got := completed.Load(); got != 2 {
		t.Fatalf("completed workers = %d, want 2", got)
	}
}

func TestAllRouteFlavorPanicsAreFatal(t *testing.T) {
	for i := 0; i < 25; i++ {
		worker := func(router *ovgEdgeRouter, _ context.Context, _ bool) GenerateRouteResponse {
			panic("panic from " + string(router.flavor))
		}

		responses, err := generateRouteFlavorResponsesWith(context.Background(), routeFlavorTestRouters(), false, worker)
		if responses != nil {
			t.Fatalf("iteration %d: responses = %v, want nil", i, responses)
		}
		if !errors.Is(err, invariant.ErrViolation) {
			t.Fatalf("iteration %d: error = %v, want invariant.ErrViolation", i, err)
		}
		if !strings.Contains(err.Error(), "edge routing flavor ShortestToLongest violated an invariant") {
			t.Fatalf("iteration %d: error = %q, want first declared flavor", i, err)
		}
		if strings.Contains(err.Error(), "panic from") {
			t.Fatalf("iteration %d: error exposes the recovered panic payload: %q", i, err)
		}
	}
}

func TestRouteFlavorOrdinaryFailureAllowsSuccessfulFallback(t *testing.T) {
	wantErr := errors.New("shortest failed")
	worker := func(router *ovgEdgeRouter, _ context.Context, _ bool) GenerateRouteResponse {
		if router.flavor == ShortestToLongest {
			return GenerateRouteResponse{Err: wantErr}
		}
		return GenerateRouteResponse{Distance: 42}
	}

	responses, err := generateRouteFlavorResponsesWith(context.Background(), routeFlavorTestRouters(), false, worker)
	if err != nil {
		t.Fatalf("generateRouteFlavorResponsesWith error: %v", err)
	}
	successful, err := successfulRouteFlavorResponses(responses)
	if err != nil {
		t.Fatalf("successfulRouteFlavorResponses error: %v", err)
	}
	if _, ok := successful[ShortestToLongest]; ok {
		t.Fatal("failed flavor was reported as successful")
	}
	for _, flavor := range []RouteGenerationFlavor{LongestToShortest, Default} {
		response, ok := successful[flavor]
		if !ok {
			t.Fatalf("missing successful fallback flavor %s", flavor)
		}
		if response.Distance != 42 {
			t.Fatalf("flavor %s distance = %v, want 42", flavor, response.Distance)
		}
	}
}

func TestRouteFlavorAggregateFailureRejectsSuccessfulFallback(t *testing.T) {
	aggregateErr := fmt.Errorf("%w: injected aggregate exhaustion", errRouteStageWorkLimit)
	responses := []GenerateRouteResponse{
		{Flavor: ShortestToLongest, Err: aggregateErr},
		{Flavor: LongestToShortest, Distance: 42},
		{Flavor: Default, Distance: 43},
	}
	if _, err := successfulRouteFlavorResponses(responses); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("successfulRouteFlavorResponses error = %v, want aggregate stage limit", err)
	}
}

func TestAllRouteFlavorFailuresUseDeclaredOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		errorsByFlavor := map[RouteGenerationFlavor]error{
			ShortestToLongest: errors.New("shortest failed"),
			LongestToShortest: errors.New("longest failed"),
			Default:           errors.New("default failed"),
		}
		delayByFlavor := map[RouteGenerationFlavor]time.Duration{
			ShortestToLongest: 2 * time.Millisecond,
			LongestToShortest: time.Millisecond,
			Default:           0,
		}
		worker := func(router *ovgEdgeRouter, _ context.Context, _ bool) GenerateRouteResponse {
			time.Sleep(delayByFlavor[router.flavor])
			runtime.Gosched()
			return GenerateRouteResponse{Err: errorsByFlavor[router.flavor]}
		}

		for i := 0; i < 25; i++ {
			responses, err := generateRouteFlavorResponsesWith(context.Background(), routeFlavorTestRouters(), false, worker)
			if err != nil {
				t.Fatalf("iteration %d: generateRouteFlavorResponsesWith error: %v", i, err)
			}
			_, err = successfulRouteFlavorResponses(responses)
			if !errors.Is(err, errorsByFlavor[ShortestToLongest]) {
				t.Fatalf("iteration %d: error = %v, want first declared flavor error", i, err)
			}
		}
	})
}

func TestRouteFlavorPanicCancelsAndDrainsWorkers(t *testing.T) {
	routers := routeFlavorTestRouters()
	ready := make(chan struct{}, len(routers))
	var active atomic.Int32
	var canceled atomic.Int32
	worker := func(router *ovgEdgeRouter, ctx context.Context, _ bool) GenerateRouteResponse {
		active.Add(1)
		defer active.Add(-1)
		ready <- struct{}{}
		if router.flavor == ShortestToLongest {
			for range routers {
				<-ready
			}
			panic("stop siblings")
		}
		<-ctx.Done()
		canceled.Add(1)
		return GenerateRouteResponse{Err: ctx.Err()}
	}

	responses, err := generateRouteFlavorResponsesWith(context.Background(), routers, false, worker)
	if responses != nil {
		t.Fatalf("responses = %v, want nil", responses)
	}
	if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("error = %v, want invariant.ErrViolation", err)
	}
	if got := canceled.Load(); got != 2 {
		t.Fatalf("canceled workers = %d, want 2", got)
	}
	if got := active.Load(); got != 0 {
		t.Fatalf("active workers after return = %d, want 0", got)
	}
}
