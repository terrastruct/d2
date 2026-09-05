package d2dagrelayout

import (
	"strconv"

	"github.com/d2lang/d2/d2graph"
)

type objectMapper struct {
	objToID map[*d2graph.Object]string
	idToObj map[string]*d2graph.Object
}

func NewObjectMapper() *objectMapper {
	return &objectMapper{
		objToID: make(map[*d2graph.Object]string),
		idToObj: make(map[string]*d2graph.Object),
	}
}

func (c *objectMapper) Register(obj *d2graph.Object) {
	id := strconv.Itoa(len(c.idToObj))
	c.idToObj[id] = obj
	c.objToID[obj] = id
}

func (c *objectMapper) ToID(obj *d2graph.Object) string {
	return c.objToID[obj]
}

func (c *objectMapper) ToObj(id string) *d2graph.Object {
	return c.idToObj[id]
}
