package d2graph

import (
	"encoding/json"
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/util-go/go2"
)

type SerializedGraph struct {
	Root      SerializedObject       `json:"root"`
	Edges     []SerializedEdge       `json:"edges"`
	Objects   []SerializedObject     `json:"objects"`
	RootLevel int                    `json:"rootLevel"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type SerializedObject map[string]interface{}

type SerializedEdge map[string]interface{}

func DeserializeGraph(bytes []byte, g *Graph) error {
	var sg *SerializedGraph
	err := json.Unmarshal(bytes, &sg)
	if err != nil {
		return err
	}

	g.Data = sg.Data
	var root Object
	Convert(sg.Root, &root)
	g.Root = &root
	root.Graph = g
	g.RootLevel = sg.RootLevel

	idToObj := make(map[string]*Object)
	idToObj[""] = g.Root
	var objects []*Object
	for _, so := range sg.Objects {
		var o Object
		if err := Convert(so, &o); err != nil {
			return err
		}
		o.Graph = g
		objects = append(objects, &o)
		idToObj[so["AbsID"].(string)] = &o
	}

	for _, so := range append(sg.Objects, sg.Root) {
		if so["ChildrenArray"] != nil {
			children := make(map[string]*Object)
			var childrenArray []*Object

			for _, id := range so["ChildrenArray"].([]interface{}) {
				o := idToObj[id.(string)]
				childrenArray = append(childrenArray, o)
				children[strings.ToLower(o.ID)] = o

				o.Parent = idToObj[so["AbsID"].(string)]
			}

			idToObj[so["AbsID"].(string)].Children = children
			idToObj[so["AbsID"].(string)].ChildrenArray = childrenArray
		}
	}

	var edges []*Edge
	for _, se := range sg.Edges {
		var e Edge
		if err := Convert(se, &e); err != nil {
			return err
		}

		if se["Src"] != nil {
			e.Src = idToObj[se["Src"].(string)]
		}
		if se["Dst"] != nil {
			e.Dst = idToObj[se["Dst"].(string)]
		}
		edges = append(edges, &e)
	}

	g.Objects = objects
	g.Edges = edges

	return nil
}

func SerializeGraph(g *Graph) ([]byte, error) {
	sg := SerializedGraph{}

	root, err := toSerializedObject(g.Root)
	if err != nil {
		return nil, err
	}
	sg.Root = root
	sg.RootLevel = g.RootLevel
	sg.Data = g.Data

	var sobjects []SerializedObject
	for _, o := range g.Objects {
		so, err := toSerializedObject(o)
		if err != nil {
			return nil, err
		}
		sobjects = append(sobjects, so)
	}
	sg.Objects = sobjects

	var sedges []SerializedEdge
	for _, e := range g.Edges {
		se, err := ToSerializedEdge(e)
		if err != nil {
			return nil, err
		}
		sedges = append(sedges, se)
	}
	sg.Edges = sedges

	return json.Marshal(sg)
}

func toSerializedObject(o *Object) (SerializedObject, error) {
	var so SerializedObject
	if err := Convert(o, &so); err != nil {
		return nil, err
	}

	so["AbsID"] = o.AbsID()

	if len(o.ChildrenArray) > 0 {
		var children []string
		for _, c := range o.ChildrenArray {
			children = append(children, c.AbsID())
		}
		so["ChildrenArray"] = children
	}

	return so, nil
}

func ToSerializedEdge(e *Edge) (SerializedEdge, error) {
	var se SerializedEdge
	if err := Convert(e, &se); err != nil {
		return nil, err
	}

	if e.Src != nil {
		se["Src"] = go2.Pointer(e.Src.AbsID())
	}
	if e.Dst != nil {
		se["Dst"] = go2.Pointer(e.Dst.AbsID())
	}

	return se, nil
}

func Convert[T, Q any](from T, to *Q) error {
	b, err := json.Marshal(from)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, to); err != nil {
		return err
	}
	return nil
}

func CompareSerializedGraph(g, other *Graph) error {
	if len(g.Objects) != len(other.Objects) {
		return fmt.Errorf("object count differs: g=%d, other=%d", len(g.Objects), len(other.Objects))
	}

	if len(g.Edges) != len(other.Edges) {
		return fmt.Errorf("edge count differs: g=%d, other=%d", len(g.Edges), len(other.Edges))
	}

	if err := CompareSerializedObject(g.Root, other.Root); err != nil {
		return fmt.Errorf("root differs: %v", err)
	}

	for i := 0; i < len(g.Objects); i++ {
		if err := CompareSerializedObject(g.Objects[i], other.Objects[i]); err != nil {
			return fmt.Errorf(
				"objects differ at %d [g=%s, other=%s]: %v",
				i,
				g.Objects[i].ID,
				other.Objects[i].ID,
				err,
			)
		}
	}

	for i := 0; i < len(g.Edges); i++ {
		if err := CompareSerializedEdge(g.Edges[i], other.Edges[i]); err != nil {
			return fmt.Errorf(
				"edges differ at %d [g=%s, other=%s]: %v",
				i,
				g.Edges[i].AbsID(),
				other.Edges[i].AbsID(),
				err,
			)
		}
	}

	return nil
}

func CompareSerializedObject(obj, other *Object) error {
	if obj != nil && other == nil {
		return fmt.Errorf("other is nil")
	} else if obj == nil && other != nil {
		return fmt.Errorf("obj is nil")
	} else if obj == nil {
		// both are nil
		return nil
	}

	if obj.ID != other.ID {
		return fmt.Errorf("ids differ: obj=%s, other=%s", obj.ID, other.ID)
	}

	if obj.AbsID() != other.AbsID() {
		return fmt.Errorf("absolute ids differ: obj=%s, other=%s", obj.AbsID(), other.AbsID())
	}

	if obj.Box != nil && other.Box == nil {
		return fmt.Errorf("other should have a box")
	} else if obj.Box == nil && other.Box != nil {
		return fmt.Errorf("other should not have a box")
	} else if obj.Box != nil {
		if obj.Width != other.Width {
			return fmt.Errorf("widths differ: obj=%f, other=%f", obj.Width, other.Width)
		}

		if obj.Height != other.Height {
			return fmt.Errorf("heights differ: obj=%f, other=%f", obj.Height, other.Height)
		}
	}

	if obj.Parent != nil && other.Parent == nil {
		return fmt.Errorf("other should have a parent")
	} else if obj.Parent == nil && other.Parent != nil {
		return fmt.Errorf("other should not have a parent")
	} else if obj.Parent != nil && obj.Parent.ID != other.Parent.ID {
		return fmt.Errorf("parent differs: obj=%s, other=%s", obj.Parent.ID, other.Parent.ID)
	}

	if len(obj.Children) != len(other.Children) {
		return fmt.Errorf("children count differs: obj=%d, other=%d", len(obj.Children), len(other.Children))
	}

	for childID, objChild := range obj.Children {
		if otherChild, exists := other.Children[childID]; exists {
			if err := CompareSerializedObject(objChild, otherChild); err != nil {
				return fmt.Errorf("children differ at key %s: %v", childID, err)
			}
		} else {
			return fmt.Errorf("child %s does not exist in other", childID)
		}
	}

	if len(obj.ChildrenArray) != len(other.ChildrenArray) {
		return fmt.Errorf("childrenArray count differs: obj=%d, other=%d", len(obj.ChildrenArray), len(other.ChildrenArray))
	}

	for i := 0; i < len(obj.ChildrenArray); i++ {
		if err := CompareSerializedObject(obj.ChildrenArray[i], other.ChildrenArray[i]); err != nil {
			return fmt.Errorf("childrenArray differs at %d: %v", i, err)
		}
	}

	if d2target.IsShape(obj.Shape.Value) != d2target.IsShape(other.Shape.Value) {
		return fmt.Errorf(
			"shapes differ: obj=%s, other=%s",
			obj.Shape.Value,
			other.Shape.Value,
		)
	}

	if obj.Icon == nil && other.Icon != nil {
		return fmt.Errorf("other does not have an icon")
	} else if obj.Icon != nil && other.Icon == nil {
		return fmt.Errorf("obj does not have an icon")
	}

	if obj.Direction.Value != other.Direction.Value {
		return fmt.Errorf(
			"directions differ: obj=%s, other=%s",
			obj.Direction.Value,
			other.Direction.Value,
		)
	}

	if obj.Label.Value != other.Label.Value {
		return fmt.Errorf(
			"labels differ: obj=%s, other=%s",
			obj.Label.Value,
			other.Label.Value,
		)
	}

	if obj.NearKey != nil {
		if other.NearKey == nil {
			return fmt.Errorf("other does not have near")
		}
		objKey := strings.Join(Key(obj.NearKey), ".")
		deserKey := strings.Join(Key(other.NearKey), ".")
		if objKey != deserKey {
			return fmt.Errorf(
				"near differs: obj=%s, other=%s",
				objKey,
				deserKey,
			)
		}
	} else if other.NearKey != nil {
		return fmt.Errorf("other should not have near")
	}

	if obj.LabelDimensions.Width != other.LabelDimensions.Width {
		return fmt.Errorf(
			"label width differs: obj=%d, other=%d",
			obj.LabelDimensions.Width,
			other.LabelDimensions.Width,
		)
	}

	if obj.LabelDimensions.Height != other.LabelDimensions.Height {
		return fmt.Errorf(
			"label height differs: obj=%d, other=%d",
			obj.LabelDimensions.Height,
			other.LabelDimensions.Height,
		)
	}

	if obj.SQLTable == nil && other.SQLTable != nil {
		return fmt.Errorf("other is not a sql table")
	} else if obj.SQLTable != nil && other.SQLTable == nil {
		return fmt.Errorf("obj is not a sql table")
	}

	if obj.SQLTable != nil {
		if len(obj.SQLTable.Columns) != len(other.SQLTable.Columns) {
			return fmt.Errorf(
				"table columns count differ: obj=%d, other=%d",
				len(obj.SQLTable.Columns),
				len(other.SQLTable.Columns),
			)
		}
	}

	return nil
}

func CompareSerializedEdge(edge, other *Edge) error {
	if edge.AbsID() != other.AbsID() {
		return fmt.Errorf(
			"absolute ids differ: edge=%s, other=%s",
			edge.AbsID(),
			other.AbsID(),
		)
	}

	if edge.Src.AbsID() != other.Src.AbsID() {
		return fmt.Errorf(
			"sources differ: edge=%s, other=%s",
			edge.Src.AbsID(),
			other.Src.AbsID(),
		)
	}

	if edge.Dst.AbsID() != other.Dst.AbsID() {
		return fmt.Errorf(
			"targets differ: edge=%s, other=%s",
			edge.Dst.AbsID(),
			other.Dst.AbsID(),
		)
	}

	if edge.SrcArrow != other.SrcArrow {
		return fmt.Errorf(
			"source arrows differ: edge=%t, other=%t",
			edge.SrcArrow,
			other.SrcArrow,
		)
	}

	if edge.DstArrow != other.DstArrow {
		return fmt.Errorf(
			"target arrows differ: edge=%t, other=%t",
			edge.DstArrow,
			other.DstArrow,
		)
	}

	if edge.Label.Value != other.Label.Value {
		return fmt.Errorf(
			"labels differ: edge=%s, other=%s",
			edge.Label.Value,
			other.Label.Value,
		)
	}

	if edge.LabelDimensions.Width != other.LabelDimensions.Width {
		return fmt.Errorf(
			"label width differs: edge=%d, other=%d",
			edge.LabelDimensions.Width,
			other.LabelDimensions.Width,
		)
	}

	if edge.LabelDimensions.Height != other.LabelDimensions.Height {
		return fmt.Errorf(
			"label height differs: edge=%d, other=%d",
			edge.LabelDimensions.Height,
			other.LabelDimensions.Height,
		)
	}

	if edge.SrcTableColumnIndex != nil && other.SrcTableColumnIndex == nil {
		return fmt.Errorf("other should have src column index")
	} else if other.SrcTableColumnIndex != nil && edge.SrcTableColumnIndex == nil {
		return fmt.Errorf("other should not have src column index")
	} else if other.SrcTableColumnIndex != nil {
		edgeColumn := *edge.SrcTableColumnIndex
		otherColumn := *other.SrcTableColumnIndex
		if edgeColumn != otherColumn {
			return fmt.Errorf("src column differs: edge=%d, other=%d", edgeColumn, otherColumn)
		}
	}

	if edge.DstTableColumnIndex != nil && other.DstTableColumnIndex == nil {
		return fmt.Errorf("other should have dst column index")
	} else if other.DstTableColumnIndex != nil && edge.DstTableColumnIndex == nil {
		return fmt.Errorf("other should not have dst column index")
	} else if other.DstTableColumnIndex != nil {
		edgeColumn := *edge.DstTableColumnIndex
		otherColumn := *other.DstTableColumnIndex
		if edgeColumn != otherColumn {
			return fmt.Errorf("dst column differs: edge=%d, other=%d", edgeColumn, otherColumn)
		}
	}
	return nil
}
