package d2graph

import (
	"encoding/json"

	"oss.terrastruct.com/util-go/go2"
)

type SerializedGraph struct {
	Root    SerializedObject   `json:"root"`
	Edges   []SerializedEdge   `json:"edges"`
	Objects []SerializedObject `json:"objects"`
}

type SerializedObject map[string]interface{}

type SerializedEdge map[string]interface{}

func DeserializeGraph(bytes []byte, g *Graph) error {
	var sg *SerializedGraph
	err := json.Unmarshal(bytes, &sg)
	if err != nil {
		return err
	}

	g.Root = &Object{
		Graph:    g,
		Children: make(map[string]*Object),
	}

	idToObj := make(map[string]*Object)
	idToObj[""] = g.Root
	var objects []*Object
	for _, so := range sg.Objects {
		var o Object
		if err := convert(so, &o); err != nil {
			return err
		}
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
				children[o.IDVal] = o

				o.Parent = idToObj[so["AbsID"].(string)]
			}

			idToObj[so["AbsID"].(string)].Children = children
			idToObj[so["AbsID"].(string)].ChildrenArray = childrenArray
		}
	}

	var edges []*Edge
	for _, se := range sg.Edges {
		var e Edge
		if err := convert(se, &e); err != nil {
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
		se, err := toSerializedEdge(e)
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
	if err := convert(o, &so); err != nil {
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

func toSerializedEdge(e *Edge) (SerializedEdge, error) {
	var se SerializedEdge
	if err := convert(e, &se); err != nil {
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

func convert[T, Q any](from T, to *Q) error {
	b, err := json.Marshal(from)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, to); err != nil {
		return err
	}
	return nil
}
