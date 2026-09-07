package layoutgraph

// EntityID identifies a node or edge independently of the host architecture.
//
// TALA IDs are serialized and participate in deterministic ordering, so their
// width must not depend on whether the engine is running natively or in WASM.
type EntityID = int64
