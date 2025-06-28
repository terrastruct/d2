package d2ast

import "oss.terrastruct.com/d2/lib/label"

// All reserved keywords. See init below.
var ReservedKeywords map[string]struct{}

// Non Style/Holder keywords.
var SimpleReservedKeywords = map[string]struct{}{
	"label":          {},
	"shape":          {},
	"icon":           {},
	"constraint":     {},
	"tooltip":        {},
	"link":           {},
	"near":           {},
	"width":          {},
	"height":         {},
	"direction":      {},
	"top":            {},
	"left":           {},
	"grid-rows":      {},
	"grid-columns":   {},
	"grid-gap":       {},
	"vertical-gap":   {},
	"horizontal-gap": {},
	"class":          {},
	"vars":           {},
}

// ReservedKeywordHolders are reserved keywords that are meaningless on its own and must hold composites
var ReservedKeywordHolders = map[string]struct{}{
	"style": {},
}

// CompositeReservedKeywords are reserved keywords that can hold composites
var CompositeReservedKeywords = map[string]struct{}{
	"source-arrowhead": {},
	"target-arrowhead": {},
	"classes":          {},
	"constraint":       {},
	"label":            {},
	"icon":             {},
	"multiple":         {},
}

// StyleKeywords are reserved keywords which cannot exist outside of the "style" keyword
var StyleKeywords = map[string]struct{}{
	"opacity":       {},
	"stroke":        {},
	"fill":          {},
	"fill-pattern":  {},
	"stroke-width":  {},
	"stroke-dash":   {},
	"border-radius": {},

	// Only for text
	"font":           {},
	"font-size":      {},
	"font-color":     {},
	"bold":           {},
	"italic":         {},
	"underline":      {},
	"text-transform": {},

	// Only for shapes
	"shadow":        {},
	"multiple":      {},
	"double-border": {},

	// Only for squares
	"3d": {},

	// Only for edges
	"animated": {},
	"filled":   {},
}

// TODO maybe autofmt should allow other values, and transform them to conform
// e.g. left-center becomes center-left
var NearConstantsArray = []string{
	"top-left",
	"top-center",
	"top-right",

	"center-left",
	"center-right",

	"bottom-left",
	"bottom-center",
	"bottom-right",
}
var NearConstants map[string]struct{}

// LabelPositionsArray are the values that labels and icons can set `near` to
var LabelPositionsArray = []string{
	"top-left",
	"top-center",
	"top-right",

	"center-left",
	"center-center",
	"center-right",

	"bottom-left",
	"bottom-center",
	"bottom-right",

	"outside-top-left",
	"outside-top-center",
	"outside-top-right",

	"outside-left-top",
	"outside-left-center",
	"outside-left-bottom",

	"outside-right-top",
	"outside-right-center",
	"outside-right-bottom",

	"outside-bottom-left",
	"outside-bottom-center",
	"outside-bottom-right",
}
var LabelPositions map[string]struct{}

var LabelPositionsMapping = map[string]label.Position{
	"top-left":   label.InsideTopLeft,
	"top-center": label.InsideTopCenter,
	"top-right":  label.InsideTopRight,

	"center-left":   label.InsideMiddleLeft,
	"center-center": label.InsideMiddleCenter,
	"center-right":  label.InsideMiddleRight,

	"bottom-left":   label.InsideBottomLeft,
	"bottom-center": label.InsideBottomCenter,
	"bottom-right":  label.InsideBottomRight,

	"outside-top-left":   label.OutsideTopLeft,
	"outside-top-center": label.OutsideTopCenter,
	"outside-top-right":  label.OutsideTopRight,

	"outside-left-top":    label.OutsideLeftTop,
	"outside-left-center": label.OutsideLeftMiddle,
	"outside-left-bottom": label.OutsideLeftBottom,

	"outside-right-top":    label.OutsideRightTop,
	"outside-right-center": label.OutsideRightMiddle,
	"outside-right-bottom": label.OutsideRightBottom,

	"outside-bottom-left":   label.OutsideBottomLeft,
	"outside-bottom-center": label.OutsideBottomCenter,
	"outside-bottom-right":  label.OutsideBottomRight,
}

var FillPatterns = []string{
	"none",
	"dots",
	"lines",
	"grain",
	"paper",
}

var TextTransforms = []string{"none", "uppercase", "lowercase", "capitalize"}

// BoardKeywords contains the keywords that create new boards.
var BoardKeywords = map[string]struct{}{
	"layers":    {},
	"scenarios": {},
	"steps":     {},
}

func init() {
	ReservedKeywords = make(map[string]struct{})
	for k, v := range SimpleReservedKeywords {
		ReservedKeywords[k] = v
	}
	for k, v := range StyleKeywords {
		ReservedKeywords[k] = v
	}
	for k, v := range ReservedKeywordHolders {
		CompositeReservedKeywords[k] = v
	}
	for k, v := range BoardKeywords {
		CompositeReservedKeywords[k] = v
	}
	for k, v := range CompositeReservedKeywords {
		ReservedKeywords[k] = v
	}

	NearConstants = make(map[string]struct{}, len(NearConstantsArray))
	for _, k := range NearConstantsArray {
		NearConstants[k] = struct{}{}
	}

	LabelPositions = make(map[string]struct{}, len(LabelPositionsArray))
	for _, k := range LabelPositionsArray {
		LabelPositions[k] = struct{}{}
	}
}
