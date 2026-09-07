package placement

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestContainerOrientationAreaBudgetRestoresCluster(t *testing.T) {
	f, cluster := newClusterContainerOrientationFixture(t, 1000)
	beforeWidth, beforeHeight := orientationFootprintSize(f.graph, f.root)
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	oldCluster := *cluster
	oldArrangement, oldDesired, oldPadding := cluster.Arrangement, cluster.DesiredArrangement, cluster.Padding
	oldCell := f.graph.CellSize
	ctx := &observeContainerClusterTurn{Context: context.Background(), cluster: cluster}
	require.NoError(t, orientSourceInterior(ctx, f.graph, f.root, nil))
	require.True(t, ctx.observed, "the candidate must actually rotate before being rejected")
	assertContainerOrientationNodesRestored(t, states)
	require.Equal(t, oldArrangement, cluster.Arrangement)
	require.Equal(t, oldDesired, cluster.DesiredArrangement)
	require.Equal(t, oldPadding, cluster.Padding)
	require.Equal(t, oldCell, f.graph.CellSize)
	require.Same(t, cluster, f.graph.Clusters[cluster.Vessel])
	require.Equal(t, oldCluster, *cluster)
	afterWidth, afterHeight := orientationFootprintSize(f.graph, f.root)
	require.Equal(t, beforeWidth, afterWidth)
	require.Equal(t, beforeHeight, afterHeight)
}

func TestContainerOrientationAreaBudgetAcceptsSmallerCluster(t *testing.T) {
	f, cluster := newClusterContainerOrientationFixture(t, 500)
	beforeWidth, beforeHeight := orientationFootprintSize(f.graph, f.root)
	oldCell := f.graph.CellSize
	rootWidth, rootHeight := f.root.Width, f.root.Height
	require.NoError(t, orientSourceInterior(context.Background(), f.graph, f.root, nil))
	require.Equal(t, layoutgraph.Row, cluster.Arrangement)
	require.Equal(t, layoutgraph.Row, cluster.DesiredArrangement)
	require.Equal(t, oldCell, f.graph.CellSize)
	require.Equal(t, rootWidth, f.root.Width)
	require.Equal(t, rootHeight, f.root.Height)
	afterWidth, afterHeight := orientationFootprintSize(f.graph, f.root)
	require.Greater(t, afterWidth*afterHeight, beforeWidth*beforeHeight, "modest area growth remains allowed")
	require.LessOrEqual(t, afterWidth*afterHeight, 2*beforeWidth*beforeHeight)
	for _, n := range cluster.Nodes {
		require.Equal(t, 500.0, n.Width)
		require.Equal(t, 40.0, n.Height)
	}
}
