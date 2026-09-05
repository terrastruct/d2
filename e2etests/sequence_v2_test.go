package e2etests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2target"
)

func testSequenceV2(t *testing.T) {
	runa(t, []testCase{
		{
			name: "groups_create_actors",
			script: `shape: sequence-diagram
request: {
  shape: edge-group
  client -> server: Request
  processing: {
    shape: edge-group
    server.work -> database.query: Read
    database.query -> server.work: Result
  }
  server -> client: Response
}
`,
			assertions: func(t *testing.T, d *d2target.Diagram) {
				ids := make(map[string]bool)
				for _, s := range d.Shapes {
					ids[s.ID] = true
				}
				for _, id := range []string{"client", "server", "database", "server.work", "database.query", "request", "request.processing"} {
					require.True(t, ids[id], id)
				}
				require.False(t, ids["request.client"])
			},
		},
		{
			name: "labels_events_and_repeated_actors",
			script: `shape: sequence-diagram
vars: {mirror: true; numbered: true}
client: Client {shape: person}
server: Service
client -> server.work: Start
server.work: Processing {label.near: top-right}
server.work.inner: Transaction
server.work.inner -> server.work.inner: Validate
server.status: Waiting {shape: page}
server.checkpoint: Checkpoint {shape: diamond}
server.again: {shape: actor}
server.work.inner -> client: Finished
`,
			assertions: func(t *testing.T, d *d2target.Diagram) {
				labels := make(map[string]int)
				for _, s := range d.Shapes {
					labels[s.Label]++
				}
				require.GreaterOrEqual(t, labels["Service"], 3)
				require.GreaterOrEqual(t, labels["Client"], 2)
				require.Equal(t, 1, labels["Processing"])
				require.Equal(t, 1, labels["Transaction"])
				require.Equal(t, 1, labels["Waiting"])
				require.Equal(t, 1, labels["Checkpoint"])
				found := false
				for _, c := range d.Connections {
					if strings.Contains(c.Label, "Start") {
						found = true
						require.True(t, strings.HasPrefix(c.Label, "1"))
					}
				}
				require.True(t, found)
			},
		},
		{
			name: "actor_groups_and_gaps",
			script: `shape: sequence-diagram
horizontal-gap: 75
vertical-gap: 45
app: Application {
  shape: actor-group
  producer: Producer
  consumer: Consumer
}
kafka: Kafka {
  shape: actor-group
  topics: Topics {
    shape: actor-group
    source: Source
    destination: Destination
  }
}
app.producer -> kafka.topics.source: Send {vertical-gap: 0}
kafka.topics.source -> kafka.topics.destination: Forward
kafka.topics.destination -> app.consumer: Receive {vertical-gap: 120}
app.consumer -> app.producer: Acknowledge
`,
		},
		{
			name: "nested_and_legacy",
			script: `direction: right
legacy: {shape: sequence_diagram; a -> b: Legacy}
modern: {
  shape: sequence-diagram
  vars: {mirror: true}
  group: {shape: edge-group; x -> y: V2}
}
legacy -> modern
`,
		},
		{
			name: "nested_grid_and_near",
			script: `grid: {
  grid-columns: 2
  first: {shape: sequence-diagram; vars: {mirror: true}; a -> b: First}
  second: {shape: sequence-diagram; x -> y: Second}
}
legend: {shape: sequence-diagram; near: bottom-center; p -> q: Legend}
`,
		},
		{
			name: "structured_actor_repetition",
			script: `shape: sequence-diagram
vars: {mirror: true}
table: Accounts {shape: sql_table; id: int}
service: Service {shape: class; +send(): bool}
table -> service: Query
table.again: {shape: actor}
service.again: {shape: actor}
service -> table: Result
`,
			assertions: func(t *testing.T, d *d2target.Diagram) {
				labels := make(map[string]int)
				for _, s := range d.Shapes {
					labels[s.Label]++
					if s.Type == d2target.ShapeSQLTable {
						require.Len(t, s.Columns, 1)
					}
					if s.Type == d2target.ShapeClass {
						require.Len(t, s.Methods, 1)
					}
				}
				require.Equal(t, 3, labels["Accounts"])
				require.Equal(t, 3, labels["Service"])
			},
		},
		{
			name: "boards",
			script: `shape: sequence-diagram
vars: {mirror: true}
a -> b: Request
scenarios: {response: {b -> a: Reply}}
`,
		},
		{
			name: "empty_groups_and_no_messages",
			script: `shape: sequence-diagram
actors: {shape: actor-group; a; b}
empty: {shape: edge-group}
actors.a.note: Ready {shape: page}
`,
		},
	})
}
