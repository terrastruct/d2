package color

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/mazznoer/csscolorparser"

	"oss.terrastruct.com/util-go/go2"
)

var themeColorRegex = regexp.MustCompile(`^(N[1-7]|B[1-6]|AA[245]|AB[45])$`)

func IsThemeColor(colorString string) bool {
	return themeColorRegex.MatchString(colorString)
}

func Darken(colorString string) (string, error) {
	if IsThemeColor(colorString) {
		switch {
		case colorString[0] == 'B':
			switch colorString[1] {
			case '1', '2':
				return B1, nil
			case '3':
				return B2, nil
			case '4':
				return B3, nil
			case '5':
				return B4, nil
			case '6':
				return B5, nil
			}

		case colorString[0:2] == "AA":
			switch colorString[2] {
			case '2', '4':
				return AA2, nil
			case '5':
				return AA4, nil
			}

		case colorString[0:2] == "AB":
			switch colorString[2] {
			case '4':
				return AB4, nil
			case '5':
				return AB5, nil
			}

		case colorString[0] == 'N':
			switch colorString[1] {
			case '1', '2':
				return N1, nil
			case '3':
				return N2, nil
			case '4':
				return N3, nil
			case '5':
				return N4, nil
			case '6':
				return N5, nil
			case '7':
				return N6, nil
			}
		}

		return "", fmt.Errorf("invalid color \"%s\"", colorString)
	}

	return darkenCSS(colorString)
}

func darkenCSS(colorString string) (string, error) {
	c, err := csscolorparser.Parse(colorString)
	if err != nil {
		return "", err
	}
	h, s, l := colorful.Color{R: c.R, G: c.G, B: c.B}.Hsl()
	// decrease luminance by 10%
	return colorful.Hsl(h, s, l-.1).Clamped().Hex(), nil
}

func LuminanceCategory(colorString string) (string, error) {
	// check if colorString matches the `url('#grad-<sha1-hash>')` format
	// which is used to refer to a <linearGradient> or <radialGradient> element.
	if IsURLGradientID(colorString) {
		return "normal", nil
	}
	l, err := Luminance(colorString)
	if err != nil {
		return "", err
	}

	switch {
	case l >= .88:
		return "bright", nil
	case l >= .55:
		return "normal", nil
	case l >= .30:
		return "dark", nil
	default:
		return "darker", nil
	}
}

func Luminance(colorString string) (float64, error) {
	c, err := csscolorparser.Parse(colorString)
	if err != nil {
		return 0, err
	}

	// https://stackoverflow.com/a/596243
	l := float64(
		float64(0.299)*float64(c.R) +
			float64(0.587)*float64(c.G) +
			float64(0.114)*float64(c.B),
	)
	return l, nil
}

const (
	N1 = "N1" // foreground color
	N2 = "N2"
	N3 = "N3"
	N4 = "N4"
	N5 = "N5"
	N6 = "N6"
	N7 = "N7" // background color

	// Base Colors: used for containers
	B1 = "B1"
	B2 = "B2"
	B3 = "B3"
	B4 = "B4"
	B5 = "B5"
	B6 = "B6"

	// Alternative colors A
	AA2 = "AA2"
	AA4 = "AA4"
	AA5 = "AA5"

	// Alternative colors B
	AB4 = "AB4"
	AB5 = "AB5"

	// Special
	Empty = ""
	None  = "none"
)

type RGB struct {
	Red   uint8
	Green uint8
	Blue  uint8
}

// https://github.com/go-playground/colors/blob/main/rgb.go#L89
func (c *RGB) IsLight() bool {
	r := float64(c.Red)
	g := float64(c.Green)
	b := float64(c.Blue)

	hsp := math.Sqrt(0.299*math.Pow(r, 2) + 0.587*math.Pow(g, 2) + 0.114*math.Pow(b, 2))

	return hsp > 130
}

// https://gist.github.com/CraigChilds94/6514edbc6a2db5e434a245487c525c75
func Hex2RGB(hex string) (RGB, error) {
	var rgb RGB
	if len(hex) > 3 && hex[0] == '#' {
		hex = hex[1:]
	} else {
		return RGB{}, fmt.Errorf("cannot parse hex color %v", hex)
	}
	values, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return RGB{}, err
	}

	rgb = RGB{
		Red:   uint8(values >> 16),
		Green: uint8((values >> 8) & 0xFF),
		Blue:  uint8(values & 0xFF),
	}

	return rgb, nil
}

// https://www.w3.org/TR/css-color-4/#svg-color
var namedRgbMap = map[string][]uint8{
	"aliceblue":            {240, 248, 255}, // #F0F8FF
	"antiquewhite":         {250, 235, 215}, // #FAEBD7
	"aqua":                 {0, 255, 255},   // #00FFFF
	"aquamarine":           {127, 255, 212}, // #7FFFD4
	"azure":                {240, 255, 255}, // #F0FFFF
	"beige":                {245, 245, 220}, // #F5F5DC
	"bisque":               {255, 228, 196}, // #FFE4C4
	"black":                {0, 0, 0},       // #000000
	"blanchedalmond":       {255, 235, 205}, // #FFEBCD
	"blue":                 {0, 0, 255},     // #0000FF
	"blueviolet":           {138, 43, 226},  // #8A2BE2
	"brown":                {165, 42, 42},   // #A52A2A
	"burlywood":            {222, 184, 135}, // #DEB887
	"cadetblue":            {95, 158, 160},  // #5F9EA0
	"chartreuse":           {127, 255, 0},   // #7FFF00
	"chocolate":            {210, 105, 30},  // #D2691E
	"coral":                {255, 127, 80},  // #FF7F50
	"cornflowerblue":       {100, 149, 237}, // #6495ED
	"cornsilk":             {255, 248, 220}, // #FFF8DC
	"crimson":              {220, 20, 60},   // #DC143C
	"cyan":                 {0, 255, 255},   // #00FFFF
	"darkblue":             {0, 0, 139},     // #00008B
	"darkcyan":             {0, 139, 139},   // #008B8B
	"darkgoldenrod":        {184, 134, 11},  // #B8860B
	"darkgray":             {169, 169, 169}, // #A9A9A9
	"darkgreen":            {0, 100, 0},     // #006400
	"darkgrey":             {169, 169, 169}, // #A9A9A9
	"darkkhaki":            {189, 183, 107}, // #BDB76B
	"darkmagenta":          {139, 0, 139},   // #8B008B
	"darkolivegreen":       {85, 107, 47},   // #556B2F
	"darkorange":           {255, 140, 0},   // #FF8C00
	"darkorchid":           {153, 50, 204},  // #9932CC
	"darkred":              {139, 0, 0},     // #8B0000
	"darksalmon":           {233, 150, 122}, // #E9967A
	"darkseagreen":         {143, 188, 143}, // #8FBC8F
	"darkslateblue":        {72, 61, 139},   // #483D8B
	"darkslategray":        {47, 79, 79},    // #2F4F4F
	"darkslategrey":        {47, 79, 79},    // #2F4F4F
	"darkturquoise":        {0, 206, 209},   // #00CED1
	"darkviolet":           {148, 0, 211},   // #9400D3
	"deeppink":             {255, 20, 147},  // #FF1493
	"deepskyblue":          {0, 191, 255},   // #00BFFF
	"dimgray":              {105, 105, 105}, // #696969
	"dimgrey":              {105, 105, 105}, // #696969
	"dodgerblue":           {30, 144, 255},  // #1E90FF
	"firebrick":            {178, 34, 34},   // #B22222
	"floralwhite":          {255, 250, 240}, // #FFFAF0
	"forestgreen":          {34, 139, 34},   // #228B22
	"fuchsia":              {255, 0, 255},   // #FF00FF
	"gainsboro":            {220, 220, 220}, // #DCDCDC
	"ghostwhite":           {248, 248, 255}, // #F8F8FF
	"gold":                 {255, 215, 0},   // #FFD700
	"goldenrod":            {218, 165, 32},  // #DAA520
	"gray":                 {128, 128, 128}, // #808080
	"green":                {0, 128, 0},     // #008000
	"greenyellow":          {173, 255, 47},  // #ADFF2F
	"grey":                 {128, 128, 128}, // #808080
	"honeydew":             {240, 255, 240}, // #F0FFF0
	"hotpink":              {255, 105, 180}, // #FF69B4
	"indianred":            {205, 92, 92},   // #CD5C5C
	"indigo":               {75, 0, 130},    // #4B0082
	"ivory":                {255, 255, 240}, // #FFFFF0
	"khaki":                {240, 230, 140}, // #F0E68C
	"lavender":             {230, 230, 250}, // #E6E6FA
	"lavenderblush":        {255, 240, 245}, // #FFF0F5
	"lawngreen":            {124, 252, 0},   // #7CFC00
	"lemonchiffon":         {255, 250, 205}, // #FFFACD
	"lightblue":            {173, 216, 230}, // #ADD8E6
	"lightcoral":           {240, 128, 128}, // #F08080
	"lightcyan":            {224, 255, 255}, // #E0FFFF
	"lightgoldenrodyellow": {250, 250, 210}, // #FAFAD2
	"lightgray":            {211, 211, 211}, // #D3D3D3
	"lightgreen":           {144, 238, 144}, // #90EE90
	"lightgrey":            {211, 211, 211}, // #D3D3D3
	"lightpink":            {255, 182, 193}, // #FFB6C1
	"lightsalmon":          {255, 160, 122}, // #FFA07A
	"lightseagreen":        {32, 178, 170},  // #20B2AA
	"lightskyblue":         {135, 206, 250}, // #87CEFA
	"lightslategray":       {119, 136, 153}, // #778899
	"lightslategrey":       {119, 136, 153}, // #778899
	"lightsteelblue":       {176, 196, 222}, // #B0C4DE
	"lightyellow":          {255, 255, 224}, // #FFFFE0
	"lime":                 {0, 255, 0},     // #00FF00
	"limegreen":            {50, 205, 50},   // #32CD32
	"linen":                {250, 240, 230}, // #FAF0E6
	"magenta":              {255, 0, 255},   // #FF00FF
	"maroon":               {128, 0, 0},     // #800000
	"mediumaquamarine":     {102, 205, 170}, // #66CDAA
	"mediumblue":           {0, 0, 205},     // #0000CD
	"mediumorchid":         {186, 85, 211},  // #BA55D3
	"mediumpurple":         {147, 112, 219}, // #9370DB
	"mediumseagreen":       {60, 179, 113},  // #3CB371
	"mediumslateblue":      {123, 104, 238}, // #7B68EE
	"mediumspringgreen":    {0, 250, 154},   // #00FA9A
	"mediumturquoise":      {72, 209, 204},  // #48D1CC
	"mediumvioletred":      {199, 21, 133},  // #C71585
	"midnightblue":         {25, 25, 112},   // #191970
	"muintcream":           {245, 255, 250}, // #F5FFFA
	"mistyrose":            {255, 228, 225}, // #FFE4E1
	"moccasin":             {255, 228, 181}, // #FFE4B5
	"navajowhite":          {255, 222, 173}, // #FFDEAD
	"navy":                 {0, 0, 128},     // #000080
	"oldlace":              {253, 245, 230}, // #FDF5E6
	"olive":                {128, 128, 0},   // #808000
	"olivedrab":            {107, 142, 35},  // #6B8E23
	"orange":               {255, 165, 0},   // #FFA500
	"orangered":            {255, 69, 0},    // #FF4500
	"orchid":               {218, 112, 214}, // #DA70D6
	"palegoldenrod":        {238, 232, 170}, // #EEE8AA
	"palegreen":            {152, 251, 152}, // #98FB98
	"paleturquoise":        {175, 238, 238}, // #AFEEEE
	"palevioletred":        {219, 112, 147}, // #DB7093
	"papayawhip":           {255, 239, 213}, // #FFEFD5
	"peachpuff":            {255, 218, 185}, // #FFDAB9
	"peru":                 {205, 133, 63},  // #CD853F
	"pink":                 {255, 192, 203}, // #FFC0CB
	"plum":                 {221, 160, 221}, // #DDA0DD
	"powderblue":           {176, 224, 230}, // #B0E0E6
	"purple":               {128, 0, 128},   // #800080
	"red":                  {255, 0, 0},     // #FF0000
	"rebeccapurple":        {102, 51, 153},  // #663399
	"rosybrown":            {188, 143, 143}, // #BC8F8F
	"royalblue":            {65, 105, 225},  // #4169E1
	"saddlebrown":          {139, 69, 19},   // #8B4513
	"salmon":               {250, 128, 114}, // #FA8072
	"sandybrown":           {244, 164, 96},  // #F4A460
	"seagreen":             {46, 139, 87},   // #2E8B57
	"seashell":             {255, 245, 238}, // #FFF5EE
	"sienna":               {160, 82, 45},   // #A0522D
	"silver":               {192, 192, 192}, // #C0C0C0
	"skyblue":              {135, 206, 235}, // #87CEEB
	"slateblue":            {106, 90, 205},  // #6A5ACD
	"slategray":            {112, 128, 144}, // #708090
	"slategrey":            {112, 128, 144}, // #708090
	"snow":                 {255, 250, 250}, // #FFFAFA
	"springgreen":          {0, 255, 127},   // #00FF7F
	"steelblue":            {70, 130, 180},  // #4682B4
	"tan":                  {210, 180, 140}, // #D2B48C
	"teal":                 {0, 128, 128},   // #008080
	"thistle":              {216, 191, 216}, // #D8BFD8
	"tomato":               {255, 99, 71},   // #FF6347
	"turquoise":            {64, 224, 208},  // #40E0D0
	"violet":               {238, 130, 238}, // #EE82EE
	"wheat":                {245, 222, 179}, // #F5DEB3
	"white":                {255, 255, 255}, // #FFFFFF
	"whitesmoke":           {245, 245, 245}, // #F5F5F5
	"yellow":               {255, 255, 0},   // #FFFF00
	"yellowgreen":          {154, 205, 50},  // #9ACD32
}

func Name2RGB(name string) RGB {
	if rgb, ok := namedRgbMap[strings.ToLower(name)]; ok {
		return RGB{
			Red:   rgb[0],
			Green: rgb[1],
			Blue:  rgb[2],
		}
	}
	return RGB{}
}

var NamedColors = []string{
	"currentcolor",
	"transparent",
	"aliceblue",
	"antiquewhite",
	"aqua",
	"aquamarine",
	"azure",
	"beige",
	"bisque",
	"black",
	"blanchedalmond",
	"blue",
	"blueviolet",
	"brown",
	"burlywood",
	"cadetblue",
	"chartreuse",
	"chocolate",
	"coral",
	"cornflowerblue",
	"cornsilk",
	"crimson",
	"cyan",
	"darkblue",
	"darkcyan",
	"darkgoldenrod",
	"darkgray",
	"darkgrey",
	"darkgreen",
	"darkkhaki",
	"darkmagenta",
	"darkolivegreen",
	"darkorange",
	"darkorchid",
	"darkred",
	"darksalmon",
	"darkseagreen",
	"darkslateblue",
	"darkslategray",
	"darkslategrey",
	"darkturquoise",
	"darkviolet",
	"deeppink",
	"deepskyblue",
	"dimgray",
	"dimgrey",
	"dodgerblue",
	"firebrick",
	"floralwhite",
	"forestgreen",
	"fuchsia",
	"gainsboro",
	"ghostwhite",
	"gold",
	"goldenrod",
	"gray",
	"grey",
	"green",
	"greenyellow",
	"honeydew",
	"hotpink",
	"indianred",
	"indigo",
	"ivory",
	"khaki",
	"lavender",
	"lavenderblush",
	"lawngreen",
	"lemonchiffon",
	"lightblue",
	"lightcoral",
	"lightcyan",
	"lightgoldenrodyellow",
	"lightgray",
	"lightgrey",
	"lightgreen",
	"lightpink",
	"lightsalmon",
	"lightseagreen",
	"lightskyblue",
	"lightslategray",
	"lightslategrey",
	"lightsteelblue",
	"lightyellow",
	"lime",
	"limegreen",
	"linen",
	"magenta",
	"maroon",
	"mediumaquamarine",
	"mediumblue",
	"mediumorchid",
	"mediumpurple",
	"mediumseagreen",
	"mediumslateblue",
	"mediumspringgreen",
	"mediumturquoise",
	"mediumvioletred",
	"midnightblue",
	"mintcream",
	"mistyrose",
	"moccasin",
	"navajowhite",
	"navy",
	"oldlace",
	"olive",
	"olivedrab",
	"orange",
	"orangered",
	"orchid",
	"palegoldenrod",
	"palegreen",
	"paleturquoise",
	"palevioletred",
	"papayawhip",
	"peachpuff",
	"peru",
	"pink",
	"plum",
	"powderblue",
	"purple",
	"rebeccapurple",
	"red",
	"rosybrown",
	"royalblue",
	"saddlebrown",
	"salmon",
	"sandybrown",
	"seagreen",
	"seashell",
	"sienna",
	"silver",
	"skyblue",
	"slateblue",
	"slategray",
	"slategrey",
	"snow",
	"springgreen",
	"steelblue",
	"tan",
	"teal",
	"thistle",
	"tomato",
	"turquoise",
	"violet",
	"wheat",
	"white",
	"whitesmoke",
	"yellow",
	"yellowgreen",
}

var ColorHexRegex = regexp.MustCompile(`^#(([0-9a-fA-F]{2}){3}|([0-9a-fA-F]){3})$`)

func ValidColor(color string) bool {
	if !go2.Contains(NamedColors, strings.ToLower(color)) && !ColorHexRegex.MatchString(color) && !IsGradient(color) {
		return false
	}

	return true
}
