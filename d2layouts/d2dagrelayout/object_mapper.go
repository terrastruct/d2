package d2dagrelayout

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
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

func (c objectMapper) generateAddNodeLine(obj *d2graph.Object, width, height int) string {
	id := c.ToID(obj)
	return fmt.Sprintf("g.setNode(`%s`, { id: `%s`, width: %d, height: %d });\n", id, id, width, height)
}

func (c objectMapper) generateAddParentLine(child, parent *d2graph.Object) string {
	return fmt.Sprintf("g.setParent(`%s`, `%s`);\n", c.ToID(child), c.ToID(parent))
}

func (c objectMapper) generateAddEdgeLine(from, to *d2graph.Object, edgeID string, width, height int) string {
	return fmt.Sprintf(
		"g.setEdge({v:`%s`, w:`%s`, name:`%s`}, { width:%d, height:%d, labelpos: `c` });\n",
		c.ToID(from), c.ToID(to), escapeID(edgeID), width, height,
	)
}

func escapeID(id string) string {
	// fixes \\
	id = strings.ReplaceAll(id, "\\", `\\`)
	// replaces \n with \\n whenever \n is not preceded by \ (does not replace \\n)
	re := regexp.MustCompile(`[^\\]\n`)
	id = re.ReplaceAllString(id, `\\n`)
	// avoid an unescaped \r becoming a \n in the layout result
	id = strings.ReplaceAll(id, "\r", `\r`)
	return id
}
