package trees

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/log"
)

func withTestLogger(ctx context.Context, tb testlog.TB) context.Context {
	tb.Helper()
	return log.With(ctx, testlog.New(tb))
}

type exactTestSlice[T comparable] struct {
	header  []T
	backing []T
}

func captureExactTestSlice[T comparable](values []T) exactTestSlice[T] {
	return exactTestSlice[T]{header: values, backing: slices.Clone(values[:cap(values)])}
}

func (snapshot exactTestSlice[T]) assertRestored(t *testing.T, got []T, name string) {
	t.Helper()
	if len(got) != len(snapshot.header) || cap(got) != cap(snapshot.header) {
		t.Fatalf("%s header = len %d cap %d; want len %d cap %d", name, len(got), cap(got), len(snapshot.header), cap(snapshot.header))
	}
	if cap(got) > 0 && &got[:cap(got)][0] != &snapshot.header[:cap(snapshot.header)][0] {
		t.Fatalf("%s backing array identity changed", name)
	}
	if !slices.Equal(got[:cap(got)], snapshot.backing) {
		t.Fatalf("%s backing array contents changed", name)
	}
}

type pointerSnapshot[T any] struct {
	pointer *T
	value   T
}

func snapshotPointer[T any](pointer *T) pointerSnapshot[T] {
	if pointer == nil {
		return pointerSnapshot[T]{}
	}
	return pointerSnapshot[T]{pointer: pointer, value: *pointer}
}

func requireCanceledAt(t *testing.T, err error, location string) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
	if !strings.Contains(err.Error(), location) {
		t.Fatalf("cancellation error = %v, want operation %q", err, location)
	}
}

type treeRuntimeState struct {
	nodes map[*layoutgraph.Node]struct{}
	edges map[*layoutgraph.Edge]struct{}
	trees map[*layoutgraph.Tree]struct{}
}

func collectTreeRuntimeState(g *layoutgraph.Graph) treeRuntimeState {
	state := treeRuntimeState{
		nodes: make(map[*layoutgraph.Node]struct{}),
		edges: make(map[*layoutgraph.Edge]struct{}),
		trees: make(map[*layoutgraph.Tree]struct{}),
	}
	addNode := func(node *layoutgraph.Node) {
		if node == nil {
			return
		}
		state.nodes[node] = struct{}{}
		for _, edge := range node.Edges {
			if edge != nil {
				state.edges[edge] = struct{}{}
			}
		}
	}
	for _, node := range g.Nodes {
		addNode(node)
	}
	for container, children := range g.Containers {
		addNode(container)
		for _, child := range children {
			addNode(child)
		}
	}
	for _, edge := range g.Edges {
		if edge != nil {
			state.edges[edge] = struct{}{}
		}
	}
	for sentinel, roots := range g.Trees {
		addNode(sentinel)
		stack := slices.Clone(roots)
		for len(stack) > 0 {
			tree := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if tree == nil {
				continue
			}
			if _, seen := state.trees[tree]; seen {
				continue
			}
			state.trees[tree] = struct{}{}
			addNode(tree.Node)
			if tree.SentinelEdge != nil {
				state.edges[tree.SentinelEdge] = struct{}{}
			}
			stack = append(stack, tree.Children...)
		}
	}
	return state
}

func mustBuildPlacementTrees(t *testing.T, g *layoutgraph.Graph) []*layoutgraph.Tree {
	t.Helper()
	guard, err := newWorkGuard(context.Background(), "GetPlacementTrees")
	if err != nil {
		t.Fatal(err)
	}
	trees, err := buildPlacementTrees(g, guard)
	if err != nil {
		t.Fatal(err)
	}
	return trees
}

func mustTreeSize(t *testing.T, tree *layoutgraph.Tree) int {
	t.Helper()
	guard, err := newWorkGuard(context.Background(), "TreeSize")
	if err != nil {
		t.Fatal(err)
	}
	size, err := treeSize(tree, guard)
	if err != nil {
		t.Fatal(err)
	}
	return size
}
