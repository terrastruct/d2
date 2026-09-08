package layoutgraph

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/lib/geo"
)

func mustNewTransaction(t *testing.T, graph *Graph, options TransactionOptions) *Transaction {
	t.Helper()
	txn, err := graph.newTransactionWithOptionsContext(context.Background(), options, nil)
	require.NoError(t, err)
	return txn
}

func TestIsCandidateRejection(t *testing.T) {
	arbitrary := errors.New("arbitrary failure")
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "invalid candidate", err: ErrInvalidCandidate, want: true},
		{name: "wrapped invalid candidate", err: fmt.Errorf("candidate: %w", ErrInvalidCandidate), want: true},
		{name: "non-improving candidate", err: ErrNonImprovingCandidate, want: true},
		{name: "wrapped non-improving candidate", err: fmt.Errorf("candidate: %w", ErrNonImprovingCandidate), want: true},
		{name: "canceled", err: context.Canceled, want: false},
		{name: "deadline", err: context.DeadlineExceeded, want: false},
		{name: "arbitrary", err: arbitrary, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsCandidateRejection(tt.err))
		})
	}
}

func TestTransactionCommitObservesCanceledContext(t *testing.T) {
	g := NewGraph()
	n := NewNode(1, 10, 10)
	n.TopLeft = geo.NewPoint(1, 2)
	g.AddNewNodeToContainer(nil, n)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	txn := mustNewTransaction(t, g, TransactionOptions{})
	txn.AddOp(func() error {
		called = true
		n.TopLeft.X = 100
		return nil
	})

	err := txn.Commit(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, called)
	require.Equal(t, 1.0, n.TopLeft.X)
}

func TestTransactionCommitRollsBackWhenContextCanceledByOperation(t *testing.T) {
	g := NewGraph()
	n := NewNode(1, 10, 10)
	n.TopLeft = geo.NewPoint(1, 2)
	g.AddNewNodeToContainer(nil, n)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	secondCalled := false
	txn := mustNewTransaction(t, g, TransactionOptions{})
	txn.AddOp(func() error {
		n.TopLeft.X = 100
		cancel()
		return nil
	})
	txn.AddOp(func() error {
		secondCalled = true
		return nil
	})

	err := txn.Commit(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, secondCalled)
	require.Equal(t, 1.0, n.TopLeft.X)
}

func TestTransactionUpdateStateAdvancesRollbackPoint(t *testing.T) {
	g := NewGraph()
	n := NewNode(1, 10, 10)
	n.TopLeft = geo.NewPoint(1, 2)
	g.AddNewNodeToContainer(nil, n)

	txn := mustNewTransaction(t, g, TransactionOptions{})
	txn.AddOp(func() error {
		n.TopLeft.X = 10
		return nil
	})
	require.NoError(t, txn.Commit(context.Background()))
	txn.Clear()
	require.NoError(t, txn.UpdateState())
	txn.AddOp(func() error {
		n.TopLeft.X = 100
		return errors.New("reject")
	})

	require.Error(t, txn.Commit(context.Background()))
	require.Equal(t, 10.0, n.TopLeft.X)
}

func TestTransactionRollbackRestoresTopLeftPointerIdentity(t *testing.T) {
	g := NewGraph()
	withPosition := NewNode(1, 10, 10)
	originalTopLeft := geo.NewPoint(1, 2)
	withPosition.TopLeft = originalTopLeft
	withoutPosition := NewNode(2, 10, 10)
	g.AddNewNodeToContainer(nil, withPosition)
	g.AddNewNodeToContainer(nil, withoutPosition)

	txn := mustNewTransaction(t, g, TransactionOptions{})
	txn.AddOp(func() error {
		withPosition.TopLeft = geo.NewPoint(100, 200)
		withoutPosition.TopLeft = geo.NewPoint(300, 400)
		return errors.New("reject")
	})

	require.Error(t, txn.Commit(context.Background()))
	require.Same(t, originalTopLeft, withPosition.TopLeft)
	require.Equal(t, geo.Point{X: 1, Y: 2}, *withPosition.TopLeft)
	require.Nil(t, withoutPosition.TopLeft)
}

func TestTransactionRollbackRestoresClusterPolicy(t *testing.T) {
	g := NewGraph()
	vessel := NewNode(1, 10, 10)
	vessel.TopLeft = geo.NewPoint(0, 0)
	g.AddNewNodeToContainer(nil, vessel)
	cluster := &Cluster{
		Vessel:             vessel,
		Graph:              g,
		Arrangement:        Row,
		DesiredArrangement: Column,
		Padding:            12,
	}
	g.Clusters[vessel] = cluster

	txn := mustNewTransaction(t, g, TransactionOptions{})
	txn.AddOp(func() error {
		cluster.Arrangement = Column
		cluster.DesiredArrangement = Row
		cluster.Padding = 99
		return errors.New("reject")
	})

	require.Error(t, txn.Commit(context.Background()))
	require.Equal(t, Row, cluster.Arrangement)
	require.Equal(t, Column, cluster.DesiredArrangement)
	require.Equal(t, 12.0, cluster.Padding)
}

func TestTransactionRollbackRestoresEdgeGeometryAndTreeOrientation(t *testing.T) {
	g := NewGraph()
	a := NewNode(1, 10, 10)
	a.TopLeft = geo.NewPoint(0, 0)
	b := NewNode(2, 10, 10)
	b.TopLeft = geo.NewPoint(20, 0)
	g.AddNewNodeToContainer(nil, a)
	g.AddNewNodeToContainer(nil, b)
	edge := g.Connect(a, b)
	edge.Points = []*geo.Point{geo.NewPoint(10, 5), geo.NewPoint(20, 5)}
	originalRoute := edge.Points
	originalFirstPoint := edge.Points[0]
	originalFirstElement := &edge.Points[0]
	tree := NewTree(a)
	tree.Orientation = geo.Right
	g.Trees[a] = []*Tree{tree}
	g.NodeToTree = make(map[*Node]*Tree)
	g.NodeToTree[a] = tree

	txn := mustNewTransaction(t, g, TransactionOptions{AffectEdgeRoutes: true})
	txn.AddOp(func() error {
		originalFirstPoint.X = 999
		edge.Points[0] = geo.NewPoint(777, 888)
		edge.Points = append(edge.Points, geo.NewPoint(500, 500))
		tree.Orientation = geo.Left
		return errors.New("reject")
	})

	require.Error(t, txn.Commit(context.Background()))
	require.Len(t, edge.Points, 2)
	require.Same(t, originalFirstElement, &edge.Points[0])
	require.Same(t, originalFirstPoint, edge.Points[0])
	require.Same(t, &originalRoute[0], &edge.Points[0])
	require.Equal(t, 10.0, edge.Points[0].X)
	require.Equal(t, geo.Right, tree.Orientation)
}

func TestTransactionCommitRollsBackBeforeRepanicking(t *testing.T) {
	g := NewGraph()
	n := NewNode(1, 10, 10)
	n.TopLeft = geo.NewPoint(1, 2)
	g.AddNewNodeToContainer(nil, n)

	txn := mustNewTransaction(t, g, TransactionOptions{})
	txn.AddOp(func() error {
		n.TopLeft.X = 100
		panic("trial panic")
	})

	require.PanicsWithValue(t, "trial panic", func() {
		_ = txn.Commit(context.Background())
	})
	require.Equal(t, 1.0, n.TopLeft.X)
}
