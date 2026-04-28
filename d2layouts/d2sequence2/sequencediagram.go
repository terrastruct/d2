package sequencediagram

import (
	"context"

	"oss.terrastruct.com/d2/d2graph"
)

/*
Concepts:

- Actor
  - A top-level entity.
  - Every other entity must be associated with an actor.
- Actor Group
  - A top level entity.
  - If an actor is defined in an actor group, it must also be referenced as such, e.g. group.actor -> actor2
- Message
  - A connection between two actors.
  - vertical-gap can be defined to specify the NEXT spacing
- Span
  - A child of an actor whose shape == nil.
  - Messages may be attached directly onto a span.
  - Spans can nest, e.g. actor.span1.span2
  - Labels are default not shown. Must be explicitly defined to show.
- Note
  - A child of an actor whose shape == page.
  - Messages are not allowed on Notes.
- Event
  - A child of an actor whose shape != nil && shape != page.
  - Messages are not allowed on Events.
- Edge Group
  - A child of an actor whose shape == edge-group.
  - Message inside an edge group are visually grouped together under one label.


- configs:
  - vars.mirror: true
  - vars.numbered: true
*/

type SequenceDiagram struct {
	*builder
}

// Traverse top-down vertically.
// At each vertical step, traverse left-right horizontally
type builder struct {
	y int
	x int
}

func Layout(ctx context.Context, g *d2graph.Graph) (*SequenceDiagram, error) {
	return newSequenceDiagram(g), nil
}

func newSequenceDiagram(g *d2graph.Graph) *SequenceDiagram {
	return &SequenceDiagram{builder: newBuilder()}
}

func newBuilder() *builder {
	return &builder{}
}
