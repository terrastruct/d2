package d2layoutfeatures

// When this is true, objects can set their `near` key to another object
// When this is false, objects can only set `near` to constants
//
// Deprecated: use d2plugin.NEAR_OBJECT. This compatibility constant will be
// removed with the d2layoutfeatures package.
const NEAR_OBJECT = "near_object"

// When this is true, containers can have dimensions set
//
// Deprecated: use d2plugin.CONTAINER_DIMENSIONS. This compatibility constant
// will be removed with the d2layoutfeatures package.
const CONTAINER_DIMENSIONS = "container_dimensions"

// When this is true, objects can specify their `top` and `left` keywords
//
// Deprecated: use d2plugin.TOP_LEFT. This compatibility constant will be
// removed with the d2layoutfeatures package.
const TOP_LEFT = "top_left"
