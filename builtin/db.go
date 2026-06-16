package builtin

import (
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"

	"github.com/aoiflux/graphene"
	"github.com/aoiflux/graphene/store"

	"mutant/object"
)

var (
	dbHandleCounter int64
	dbHandles       sync.Map // int64 → *graphene.Graph
)

func dbLabelToNodeType(label string) store.NodeType {
	h := fnv.New32a()
	h.Write([]byte(label))
	return store.CustomNodeType(uint8(h.Sum32() % 200))
}

func dbLabelToEdgeType(label string) store.EdgeType {
	h := fnv.New32a()
	h.Write([]byte("e:" + label))
	return store.CustomEdgeType(uint8(h.Sum32() % 200))
}

func dbGet(handle int64) (*graphene.Graph, bool) {
	v, ok := dbHandles.Load(handle)
	if !ok {
		return nil, false
	}
	g, ok := v.(*graphene.Graph)
	return g, ok
}

func DbOpen(args ...object.Object) object.Object {
	if len(args) != 0 {
		return newError("wrong number of arguments. got=%d, want=0", len(args))
	}
	g := graphene.NewInMemory()
	handle := atomic.AddInt64(&dbHandleCounter, 1)
	dbHandles.Store(handle, g)
	return intObj(handle)
}

func DbOpenDisk(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	path, ok := args[0].(*object.String)
	if !ok {
		return newError("argument to `db_open_disk` must be STRING, got %s", args[0].Type())
	}
	g, err := graphene.Open(path.Value)
	if err != nil {
		return newError("db_open_disk: %s", err.Error())
	}
	handle := atomic.AddInt64(&dbHandleCounter, 1)
	dbHandles.Store(handle, g)
	return intObj(handle)
}

func DbClose(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument to `db_close` must be INTEGER, got %s", args[0].Type())
	}
	g, found := dbGet(h.Value)
	if !found {
		return fsOkOrError(fmt.Errorf("db_close: invalid handle %d", h.Value))
	}
	dbHandles.Delete(h.Value)
	return fsOkOrError(g.Close())
}

func DbAddNode(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument 1 to `db_add_node` must be INTEGER, got %s", args[0].Type())
	}
	label, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `db_add_node` must be STRING, got %s", args[1].Type())
	}
	g, found := dbGet(h.Value)
	if !found {
		return newError("db_add_node: invalid handle %d", h.Value)
	}
	nodeID, err := g.AddNode(&store.Node{
		Labels: []store.NodeType{dbLabelToNodeType(label.Value)},
	})
	if err != nil {
		return newError("db_add_node: %s", err.Error())
	}
	return intObj(int64(nodeID))
}

func DbAddEdge(args ...object.Object) object.Object {
	if len(args) != 4 {
		return newError("wrong number of arguments. got=%d, want=4", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument 1 to `db_add_edge` must be INTEGER, got %s", args[0].Type())
	}
	src, ok := args[1].(*object.Integer)
	if !ok {
		return newError("argument 2 to `db_add_edge` must be INTEGER, got %s", args[1].Type())
	}
	dst, ok := args[2].(*object.Integer)
	if !ok {
		return newError("argument 3 to `db_add_edge` must be INTEGER, got %s", args[2].Type())
	}
	label, ok := args[3].(*object.String)
	if !ok {
		return newError("argument 4 to `db_add_edge` must be STRING, got %s", args[3].Type())
	}
	g, found := dbGet(h.Value)
	if !found {
		return newError("db_add_edge: invalid handle %d", h.Value)
	}
	edgeID, err := g.AddEdge(&store.Edge{
		Src:    store.NodeID(src.Value),
		Dst:    store.NodeID(dst.Value),
		Labels: []store.EdgeType{dbLabelToEdgeType(label.Value)},
	})
	if err != nil {
		return newError("db_add_edge: %s", err.Error())
	}
	return intObj(int64(edgeID))
}

func DbIndexProp(args ...object.Object) object.Object {
	if len(args) != 4 {
		return newError("wrong number of arguments. got=%d, want=4", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument 1 to `db_index_prop` must be INTEGER, got %s", args[0].Type())
	}
	nodeID, ok := args[1].(*object.Integer)
	if !ok {
		return newError("argument 2 to `db_index_prop` must be INTEGER, got %s", args[1].Type())
	}
	key, ok := args[2].(*object.String)
	if !ok {
		return newError("argument 3 to `db_index_prop` must be STRING, got %s", args[2].Type())
	}
	val, ok := args[3].(*object.String)
	if !ok {
		return newError("argument 4 to `db_index_prop` must be STRING, got %s", args[3].Type())
	}
	g, found := dbGet(h.Value)
	if !found {
		return fsOkOrError(fmt.Errorf("db_index_prop: invalid handle %d", h.Value))
	}
	err := g.IndexNodeProperty(store.NodeID(nodeID.Value), key.Value, []byte(val.Value))
	return fsOkOrError(err)
}

func DbQueryNodes(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("wrong number of arguments. got=%d, want=2", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument 1 to `db_query_nodes` must be INTEGER, got %s", args[0].Type())
	}
	label, ok := args[1].(*object.String)
	if !ok {
		return newError("argument 2 to `db_query_nodes` must be STRING, got %s", args[1].Type())
	}
	g, found := dbGet(h.Value)
	if !found {
		return newError("db_query_nodes: invalid handle %d", h.Value)
	}
	nodeType := dbLabelToNodeType(label.Value)
	ids, err := g.QueryNodeIDs(store.NodeQuery{
		Types: []store.NodeType{nodeType},
	})
	if err != nil {
		return newError("db_query_nodes: %s", err.Error())
	}
	elements := make([]object.Object, 0, len(ids))
	for _, id := range ids {
		elements = append(elements, intObj(int64(id)))
	}
	return &object.Array{Elements: elements}
}

func DbBFS(args ...object.Object) object.Object {
	if len(args) != 4 {
		return newError("wrong number of arguments. got=%d, want=4", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument 1 to `db_bfs` must be INTEGER, got %s", args[0].Type())
	}
	originID, ok := args[1].(*object.Integer)
	if !ok {
		return newError("argument 2 to `db_bfs` must be INTEGER, got %s", args[1].Type())
	}
	depth, ok := args[2].(*object.Integer)
	if !ok {
		return newError("argument 3 to `db_bfs` must be INTEGER, got %s", args[2].Type())
	}
	dirStr, ok := args[3].(*object.String)
	if !ok {
		return newError("argument 4 to `db_bfs` must be STRING, got %s", args[3].Type())
	}

	g, found := dbGet(h.Value)
	if !found {
		return newError("db_bfs: invalid handle %d", h.Value)
	}

	dir := dbParseDirection(dirStr.Value)
	result, err := g.BFS(store.NodeID(originID.Value), int(depth.Value), dir, nil)
	if err != nil {
		return newError("db_bfs: %s", err.Error())
	}

	nodeElems := make([]object.Object, 0, len(result.Nodes))
	for _, n := range result.Nodes {
		nodeElems = append(nodeElems, intObj(int64(n.ID)))
	}
	edgeElems := make([]object.Object, 0, len(result.Edges))
	for _, e := range result.Edges {
		edgeElems = append(edgeElems, intObj(int64(e.ID)))
	}

	return makeHashObject(map[string]object.Object{
		"nodes": &object.Array{Elements: nodeElems},
		"edges": &object.Array{Elements: edgeElems},
	})
}

func DbShortestPath(args ...object.Object) object.Object {
	if len(args) != 3 {
		return newError("wrong number of arguments. got=%d, want=3", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument 1 to `db_shortest_path` must be INTEGER, got %s", args[0].Type())
	}
	srcID, ok := args[1].(*object.Integer)
	if !ok {
		return newError("argument 2 to `db_shortest_path` must be INTEGER, got %s", args[1].Type())
	}
	dstID, ok := args[2].(*object.Integer)
	if !ok {
		return newError("argument 3 to `db_shortest_path` must be INTEGER, got %s", args[2].Type())
	}

	g, found := dbGet(h.Value)
	if !found {
		return newError("db_shortest_path: invalid handle %d", h.Value)
	}

	path, err := g.ShortestPath(store.NodeID(srcID.Value), store.NodeID(dstID.Value), nil)
	if err != nil {
		return newError("db_shortest_path: %s", err.Error())
	}

	elements := make([]object.Object, 0, len(path.Nodes))
	for _, n := range path.Nodes {
		elements = append(elements, intObj(int64(n.ID)))
	}
	return &object.Array{Elements: elements}
}

func DbStats(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	h, ok := args[0].(*object.Integer)
	if !ok {
		return newError("argument to `db_stats` must be INTEGER, got %s", args[0].Type())
	}
	g, found := dbGet(h.Value)
	if !found {
		return newError("db_stats: invalid handle %d", h.Value)
	}
	stats, err := g.Stats()
	if err != nil {
		return newError("db_stats: %s", err.Error())
	}
	return makeHashObject(map[string]object.Object{
		"nodes": intObj(int64(stats.NodeCount)),
		"edges": intObj(int64(stats.EdgeCount)),
	})
}

func dbParseDirection(s string) store.Direction {
	switch s {
	case "in":
		return store.DirectionInbound
	case "out":
		return store.DirectionOutbound
	default:
		return store.DirectionBoth
	}
}
