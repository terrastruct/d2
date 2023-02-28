package d2plugin

import (
	"fmt"

	"oss.terrastruct.com/d2/d2graph"
)

type PluginFeature string

// When this is true, objects can set ther `near` key to another object
// When this is false, objects can only set `near` to constants
const NEAR_OBJECT PluginFeature = "near_object"

// When this is true, containers can have dimensions set
const CONTAINER_DIMENSIONS PluginFeature = "container_dimensions"

// When this is true, objects can specify their `top` and `left` keywords
const TOP_LEFT PluginFeature = "top_left"

// When this is true, containers can have connections to descendants
const DESCENDANT_EDGES PluginFeature = "descendant_edges"

func FeatureSupportCheck(info *PluginInfo, g *d2graph.Graph) error {
	// Older version of plugin. Skip checking.
	if info.Features == nil {
		return nil
	}

	featureMap := make(map[PluginFeature]struct{}, len(info.Features))
	for _, f := range info.Features {
		featureMap[f] = struct{}{}
	}

	for _, obj := range g.Objects {
		if obj.Attributes.Top != nil || obj.Attributes.Left != nil {
			if _, ok := featureMap[TOP_LEFT]; !ok {
				return fmt.Errorf(`Object "%s" has attribute "top" and/or "left" set, but layout engine "%s" does not support locked positions.`, obj.AbsID(), info.Name)
			}
		}
		if (obj.Attributes.Width != nil || obj.Attributes.Height != nil) && len(obj.ChildrenArray) > 0 {
			if _, ok := featureMap[CONTAINER_DIMENSIONS]; !ok {
				return fmt.Errorf(`Object "%s" has attribute "width" and/or "height" set, but layout engine "%s" does not support dimensions set on containers.`, obj.AbsID(), info.Name)
			}
		}

		if obj.Attributes.NearKey != nil {
			_, isKey := g.Root.HasChild(d2graph.Key(obj.Attributes.NearKey))
			if isKey {
				if _, ok := featureMap[NEAR_OBJECT]; !ok {
					return fmt.Errorf(`Object "%s" has "near" set to another object, but layout engine "%s" only supports constant values for "near".`, obj.AbsID(), info.Name)
				}
			}
		}
	}
	if _, ok := featureMap[DESCENDANT_EDGES]; !ok {
		for _, e := range g.Edges {
			// descendant edges are ok in sequence diagrams
			if e.Src.OuterSequenceDiagram() != nil || e.Dst.OuterSequenceDiagram() != nil {
				continue
			}
			if !e.Src.IsContainer() && !e.Dst.IsContainer() {
				continue
			}
			if e.Src == e.Dst {
				return fmt.Errorf(`Connection "%s" is a self loop on a container, but layout engine "%s" does not support this.`, e.AbsID(), info.Name)
			}
			if e.Src.IsDescendantOf(e.Dst) || e.Dst.IsDescendantOf(e.Src) {
				return fmt.Errorf(`Connection "%s" goes from a container to a descendant, but layout engine "%s" does not support this.`, e.AbsID(), info.Name)
			}
		}
	}
	return nil
}
