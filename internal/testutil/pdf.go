package testutil

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/d2lang/d2/internal/testutil/imagediff"
)

const maxPDFContentStreamBytes = 64 << 20

// PDFInspection is the bounded structural and raster view of PDFs emitted by
// D2's go-pdf/fpdf writer. It intentionally is not a general PDF parser.
type PDFInspection struct {
	Pages         []PDFPage
	Images        []PDFImage
	ExternalLinks []string
	InternalLinks int
}

type PDFPage struct {
	ObjectNumber  int
	Width, Height float64
	ExternalLinks []string
	InternalLinks int
}

type PDFImage struct {
	ObjectNumber  int
	Width, Height int
	Pixels        *image.NRGBA
}

var (
	pdfObjectRE        = regexp.MustCompile(`(?m)^([1-9][0-9]*) 0 obj\r?$`)
	pdfIntegerRE       = regexp.MustCompile(`/?%s[ \t\r\n]+([0-9]+)`)
	pdfNumberRE        = regexp.MustCompile(`[-+]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)`)
	pdfContentsRE      = regexp.MustCompile(`/Contents[ \t\r\n]+([0-9]+)[ \t\r\n]+0[ \t\r\n]+R`)
	pdfImageResourceRE = regexp.MustCompile(`/I([0-9a-f]+)[ \t\r\n]+([0-9]+)[ \t\r\n]+0[ \t\r\n]+R`)
	pdfImageUseRE      = regexp.MustCompile(`/I([0-9a-f]+)[ \t\r\n]+Do(?:[ \t\r\n]|$)`)
)

type pdfObject struct {
	number int
	body   []byte
}

type pdfImageObject struct {
	PDFImage
	colorSpace string
	softMask   int
}

// InspectD2PDF verifies page/link structure and decodes D2's embedded raster
// XObjects. Supported image streams are the exact FlateDecode + PNG predictor
// shape emitted by go-pdf/fpdf for D2's RGB/RGBA PNGs.
func InspectD2PDF(content []byte) (*PDFInspection, error) {
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		return nil, fmt.Errorf("missing PDF signature")
	}
	objects, err := splitPDFObjects(content)
	if err != nil {
		return nil, err
	}
	inspection := &PDFInspection{}
	images := make(map[int]*pdfImageObject)
	objectsByNumber := make(map[int]pdfObject, len(objects))
	var pageObjects []pdfObject
	for _, object := range objects {
		objectsByNumber[object.number] = object
		dictionary := object.body
		if stream := bytes.Index(dictionary, []byte("stream")); stream >= 0 {
			dictionary = dictionary[:stream]
		}
		if isPDFPage(dictionary) {
			page, err := inspectPDFPage(object)
			if err != nil {
				return nil, err
			}
			inspection.Pages = append(inspection.Pages, page)
			inspection.ExternalLinks = append(inspection.ExternalLinks, page.ExternalLinks...)
			inspection.InternalLinks += page.InternalLinks
			pageObjects = append(pageObjects, object)
		}
		if bytes.Contains(dictionary, []byte("/Subtype /Image")) {
			parsed, err := inspectPDFImage(object)
			if err != nil {
				return nil, err
			}
			images[object.number] = parsed
		}
	}
	if len(inspection.Pages) == 0 {
		return nil, fmt.Errorf("PDF contains no page objects")
	}
	sort.Slice(inspection.Pages, func(i, j int) bool { return inspection.Pages[i].ObjectNumber < inspection.Pages[j].ObjectNumber })

	softMasks := make(map[int]struct{})
	for _, current := range images {
		if current.softMask == 0 {
			continue
		}
		mask, ok := images[current.softMask]
		if !ok {
			return nil, fmt.Errorf("PDF image object %d references missing soft mask %d", current.ObjectNumber, current.softMask)
		}
		if mask.colorSpace != "DeviceGray" || mask.Width != current.Width || mask.Height != current.Height {
			return nil, fmt.Errorf("PDF image object %d has incompatible soft mask %d", current.ObjectNumber, current.softMask)
		}
		for y := 0; y < current.Height; y++ {
			for x := 0; x < current.Width; x++ {
				pixel := current.Pixels.NRGBAAt(x, y)
				pixel.A = mask.Pixels.NRGBAAt(x, y).R
				current.Pixels.SetNRGBA(x, y, pixel)
			}
		}
		softMasks[current.softMask] = struct{}{}
	}
	primaryImages := make(map[int]PDFImage)
	for number, current := range images {
		if _, isMask := softMasks[number]; isMask {
			continue
		}
		primaryImages[number] = current.PDFImage
	}
	if len(primaryImages) == 0 {
		return nil, fmt.Errorf("PDF contains no primary raster image XObjects")
	}
	inspection.Images, err = orderPDFImagesByPage(pageObjects, objectsByNumber, primaryImages)
	if err != nil {
		return nil, err
	}
	return inspection, nil
}

// orderPDFImagesByPage follows each page's content-stream XObject references.
// go-pdf assigns image object numbers while ranging over a map, so object
// number is not page order. Repeated images are returned only on first use,
// matching go-pdf's content-based image deduplication.
func orderPDFImagesByPage(pageObjects []pdfObject, objectsByNumber map[int]pdfObject, images map[int]PDFImage) ([]PDFImage, error) {
	resources := make(map[string]int)
	for _, object := range objectsByNumber {
		dictionary := object.body
		if stream := bytes.Index(dictionary, []byte("stream")); stream >= 0 {
			dictionary = dictionary[:stream]
		}
		for _, match := range pdfImageResourceRE.FindAllSubmatch(dictionary, -1) {
			objectNumber, err := strconv.Atoi(string(match[2]))
			if err != nil {
				return nil, fmt.Errorf("parse PDF image resource object number: %w", err)
			}
			name := string(match[1])
			if prior, ok := resources[name]; ok && prior != objectNumber {
				return nil, fmt.Errorf("PDF image resource /I%s references both objects %d and %d", name, prior, objectNumber)
			}
			resources[name] = objectNumber
		}
	}

	sort.Slice(pageObjects, func(i, j int) bool { return pageObjects[i].number < pageObjects[j].number })
	ordered := make([]PDFImage, 0, len(images))
	seen := make(map[int]struct{}, len(images))
	for _, page := range pageObjects {
		match := pdfContentsRE.FindSubmatch(page.body)
		if match == nil {
			return nil, fmt.Errorf("PDF page object %d has no direct content stream reference", page.number)
		}
		contentNumber, err := strconv.Atoi(string(match[1]))
		if err != nil {
			return nil, fmt.Errorf("PDF page object %d content stream reference: %w", page.number, err)
		}
		contentObject, ok := objectsByNumber[contentNumber]
		if !ok {
			return nil, fmt.Errorf("PDF page object %d references missing content stream %d", page.number, contentNumber)
		}
		content, err := decodePDFContentStream(contentObject)
		if err != nil {
			return nil, fmt.Errorf("PDF page object %d: %w", page.number, err)
		}
		for _, use := range pdfImageUseRE.FindAllSubmatch(content, -1) {
			name := string(use[1])
			imageNumber, ok := resources[name]
			if !ok {
				return nil, fmt.Errorf("PDF page object %d uses missing image resource /I%s", page.number, name)
			}
			image, ok := images[imageNumber]
			if !ok {
				return nil, fmt.Errorf("PDF page object %d image resource /I%s references non-primary image object %d", page.number, name, imageNumber)
			}
			if _, duplicate := seen[imageNumber]; duplicate {
				continue
			}
			seen[imageNumber] = struct{}{}
			ordered = append(ordered, image)
		}
	}

	// Preserve structural visibility for registered-but-unused images, while
	// keeping their otherwise unspecified order deterministic for diagnostics.
	var unused []PDFImage
	for number, image := range images {
		if _, ok := seen[number]; !ok {
			unused = append(unused, image)
		}
	}
	sort.Slice(unused, func(i, j int) bool { return unused[i].ObjectNumber < unused[j].ObjectNumber })
	return append(ordered, unused...), nil
}

func decodePDFContentStream(object pdfObject) ([]byte, error) {
	dictionary, stream, err := pdfStream(object)
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(dictionary, []byte("/Filter")) {
		return stream, nil
	}
	if !bytes.Contains(dictionary, []byte("/Filter /FlateDecode")) {
		return nil, fmt.Errorf("PDF content stream object %d uses an unsupported filter", object.number)
	}
	decoded, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		return nil, fmt.Errorf("PDF content stream object %d decompress: %w", object.number, err)
	}
	content, readErr := io.ReadAll(io.LimitReader(decoded, maxPDFContentStreamBytes+1))
	closeErr := decoded.Close()
	if readErr != nil {
		return nil, fmt.Errorf("PDF content stream object %d decompress: %w", object.number, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("PDF content stream object %d close decompressor: %w", object.number, closeErr)
	}
	if len(content) > maxPDFContentStreamBytes {
		return nil, fmt.Errorf("PDF content stream object %d exceeds %d decoded bytes", object.number, maxPDFContentStreamBytes)
	}
	return content, nil
}

// ComparePDFImages compares the decoded PDF XObject pixels with the supplied
// standalone format-equivalent PNGs.
func ComparePDFImages(pdfContent []byte, expectedPNGs [][]byte, options imagediff.Options) (*PagedImageComparison, error) {
	inspection, err := InspectD2PDF(pdfContent)
	if err != nil {
		return nil, err
	}
	if len(inspection.Images) != len(expectedPNGs) {
		return &PagedImageComparison{}, fmt.Errorf("PDF embedded image count = %d, want %d", len(inspection.Images), len(expectedPNGs))
	}
	comparison := &PagedImageComparison{}
	for page := range expectedPNGs {
		expected, _, err := image.Decode(bytes.NewReader(expectedPNGs[page]))
		if err != nil {
			return comparison, fmt.Errorf("decode expected PNG for PDF page %d: %w", page+1, err)
		}
		result, err := imagediff.CompareImages(expected, inspection.Images[page].Pixels, options)
		if err != nil {
			comparison.Page = page + 1
			comparison.Result = result
			return comparison, fmt.Errorf("PDF page %d embedded pixels differ: %w", page+1, err)
		}
	}
	return comparison, nil
}

func splitPDFObjects(content []byte) ([]pdfObject, error) {
	matches := pdfObjectRE.FindAllSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("PDF contains no indirect objects")
	}
	objects := make([]pdfObject, 0, len(matches))
	for index, match := range matches {
		number, err := strconv.Atoi(string(content[match[2]:match[3]]))
		if err != nil {
			return nil, fmt.Errorf("parse PDF object number: %w", err)
		}
		bodyStart := match[1]
		for bodyStart < len(content) && (content[bodyStart] == '\r' || content[bodyStart] == '\n') {
			bodyStart++
		}
		bodyEnd := len(content)
		if index+1 < len(matches) {
			bodyEnd = matches[index+1][0]
		}
		endObject := bytes.LastIndex(content[bodyStart:bodyEnd], []byte("endobj"))
		if endObject < 0 {
			return nil, fmt.Errorf("PDF object %d has no endobj marker", number)
		}
		objects = append(objects, pdfObject{number: number, body: content[bodyStart : bodyStart+endObject]})
	}
	return objects, nil
}

func isPDFPage(dictionary []byte) bool {
	index := bytes.Index(dictionary, []byte("/Type /Page"))
	if index < 0 {
		return false
	}
	after := index + len("/Type /Page")
	return after == len(dictionary) || dictionary[after] != 's'
}

func inspectPDFPage(object pdfObject) (PDFPage, error) {
	page := PDFPage{ObjectNumber: object.number}
	mediaBox := bytes.Index(object.body, []byte("/MediaBox"))
	if mediaBox < 0 {
		return page, fmt.Errorf("PDF page object %d has no MediaBox", object.number)
	}
	start := bytes.IndexByte(object.body[mediaBox:], '[')
	end := bytes.IndexByte(object.body[mediaBox+start+1:], ']')
	if start < 0 || end < 0 {
		return page, fmt.Errorf("PDF page object %d has malformed MediaBox", object.number)
	}
	values := pdfNumberRE.FindAll(object.body[mediaBox+start+1:mediaBox+start+1+end], -1)
	if len(values) != 4 {
		return page, fmt.Errorf("PDF page object %d MediaBox has %d values", object.number, len(values))
	}
	numbers := make([]float64, 4)
	for index := range values {
		parsed, err := strconv.ParseFloat(string(values[index]), 64)
		if err != nil {
			return page, fmt.Errorf("PDF page object %d MediaBox: %w", object.number, err)
		}
		numbers[index] = parsed
	}
	page.Width, page.Height = numbers[2]-numbers[0], numbers[3]-numbers[1]
	if page.Width <= 0 || page.Height <= 0 {
		return page, fmt.Errorf("PDF page object %d has invalid dimensions %.2fx%.2f", object.number, page.Width, page.Height)
	}
	page.ExternalLinks = extractPDFURIs(object.body)
	page.InternalLinks = bytes.Count(object.body, []byte("/Dest ["))
	return page, nil
}

func inspectPDFImage(object pdfObject) (*pdfImageObject, error) {
	dictionary, stream, err := pdfStream(object)
	if err != nil {
		return nil, err
	}
	width, err := pdfDictionaryInteger(dictionary, "Width")
	if err != nil {
		return nil, fmt.Errorf("PDF image object %d: %w", object.number, err)
	}
	height, err := pdfDictionaryInteger(dictionary, "Height")
	if err != nil {
		return nil, fmt.Errorf("PDF image object %d: %w", object.number, err)
	}
	bits, err := pdfDictionaryInteger(dictionary, "BitsPerComponent")
	if err != nil || bits != 8 {
		return nil, fmt.Errorf("PDF image object %d requires 8-bit components, got %d/%v", object.number, bits, err)
	}
	if !bytes.Contains(dictionary, []byte("/Filter /FlateDecode")) || !bytes.Contains(dictionary, []byte("/Predictor 15")) {
		return nil, fmt.Errorf("PDF image object %d uses an unsupported image filter or predictor", object.number)
	}
	colorSpace, channels := "", 0
	switch {
	case bytes.Contains(dictionary, []byte("/ColorSpace /DeviceRGB")):
		colorSpace, channels = "DeviceRGB", 3
	case bytes.Contains(dictionary, []byte("/ColorSpace /DeviceGray")):
		colorSpace, channels = "DeviceGray", 1
	default:
		return nil, fmt.Errorf("PDF image object %d uses an unsupported color space", object.number)
	}
	columns, err := pdfDictionaryInteger(dictionary, "Columns")
	if err != nil || columns != width {
		return nil, fmt.Errorf("PDF image object %d predictor columns = %d/%v, want %d", object.number, columns, err, width)
	}
	declaredChannels, err := pdfDictionaryInteger(dictionary, "Colors")
	if err != nil || declaredChannels != channels {
		return nil, fmt.Errorf("PDF image object %d predictor colors = %d/%v, want %d", object.number, declaredChannels, err, channels)
	}
	decoded, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		return nil, fmt.Errorf("PDF image object %d decompress: %w", object.number, err)
	}
	filtered, readErr := io.ReadAll(decoded)
	closeErr := decoded.Close()
	if readErr != nil {
		return nil, fmt.Errorf("PDF image object %d decompress: %w", object.number, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("PDF image object %d close decompressor: %w", object.number, closeErr)
	}
	pixels, err := undoPNGPredictor(filtered, width, height, channels)
	if err != nil {
		return nil, fmt.Errorf("PDF image object %d: %w", object.number, err)
	}
	imageData := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			source := (y*width + x) * channels
			if channels == 3 {
				imageData.SetNRGBA(x, y, color.NRGBA{R: pixels[source], G: pixels[source+1], B: pixels[source+2], A: 255})
			} else {
				imageData.SetNRGBA(x, y, color.NRGBA{R: pixels[source], G: pixels[source], B: pixels[source], A: 255})
			}
		}
	}
	softMask := 0
	if match := regexp.MustCompile(`/SMask[ \t\r\n]+([0-9]+)[ \t\r\n]+0[ \t\r\n]+R`).FindSubmatch(dictionary); match != nil {
		softMask, _ = strconv.Atoi(string(match[1]))
	}
	return &pdfImageObject{
		PDFImage:   PDFImage{ObjectNumber: object.number, Width: width, Height: height, Pixels: imageData},
		colorSpace: colorSpace, softMask: softMask,
	}, nil
}

func pdfStream(object pdfObject) (dictionary, stream []byte, err error) {
	marker := bytes.Index(object.body, []byte("stream"))
	if marker < 0 {
		return nil, nil, fmt.Errorf("PDF image object %d has no stream", object.number)
	}
	dictionary = object.body[:marker]
	length, err := pdfDictionaryInteger(dictionary, "Length")
	if err != nil {
		return nil, nil, fmt.Errorf("PDF image object %d: %w", object.number, err)
	}
	start := marker + len("stream")
	if start < len(object.body) && object.body[start] == '\r' {
		start++
	}
	if start < len(object.body) && object.body[start] == '\n' {
		start++
	}
	if length < 0 || start+length > len(object.body) {
		return nil, nil, fmt.Errorf("PDF image object %d stream length %d exceeds object", object.number, length)
	}
	return dictionary, object.body[start : start+length], nil
}

func pdfDictionaryInteger(dictionary []byte, key string) (int, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(pdfIntegerRE.String(), regexp.QuoteMeta(key)))
	match := pattern.FindSubmatch(dictionary)
	if match == nil {
		return 0, fmt.Errorf("missing /%s integer", key)
	}
	value, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return 0, fmt.Errorf("parse /%s integer: %w", key, err)
	}
	return value, nil
}

func undoPNGPredictor(filtered []byte, width, height, channels int) ([]byte, error) {
	rowBytes := width * channels
	want := height * (rowBytes + 1)
	if len(filtered) != want {
		return nil, fmt.Errorf("PNG predictor stream length = %d, want %d", len(filtered), want)
	}
	output := make([]byte, width*height*channels)
	for y := 0; y < height; y++ {
		filter := filtered[y*(rowBytes+1)]
		source := filtered[y*(rowBytes+1)+1 : (y+1)*(rowBytes+1)]
		row := output[y*rowBytes : (y+1)*rowBytes]
		var prior []byte
		if y != 0 {
			prior = output[(y-1)*rowBytes : y*rowBytes]
		}
		for x := range row {
			left, up, upperLeft := byte(0), byte(0), byte(0)
			if x >= channels {
				left = row[x-channels]
			}
			if prior != nil {
				up = prior[x]
				if x >= channels {
					upperLeft = prior[x-channels]
				}
			}
			switch filter {
			case 0:
				row[x] = source[x]
			case 1:
				row[x] = source[x] + left
			case 2:
				row[x] = source[x] + up
			case 3:
				row[x] = source[x] + byte((int(left)+int(up))/2)
			case 4:
				row[x] = source[x] + paeth(left, up, upperLeft)
			default:
				return nil, fmt.Errorf("unsupported PNG predictor filter %d on row %d", filter, y)
			}
		}
	}
	return output, nil
}

func paeth(left, up, upperLeft byte) byte {
	prediction := int(left) + int(up) - int(upperLeft)
	dLeft := absInt(prediction - int(left))
	dUp := absInt(prediction - int(up))
	dUpperLeft := absInt(prediction - int(upperLeft))
	if dLeft <= dUp && dLeft <= dUpperLeft {
		return left
	}
	if dUp <= dUpperLeft {
		return up
	}
	return upperLeft
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func extractPDFURIs(body []byte) []string {
	var uris []string
	for search := 0; search < len(body); {
		index := bytes.Index(body[search:], []byte("/URI"))
		if index < 0 {
			break
		}
		start := search + index + len("/URI")
		for start < len(body) && strings.ContainsRune(" \t\r\n", rune(body[start])) {
			start++
		}
		if start >= len(body) || body[start] != '(' {
			search = start
			continue
		}
		value, end, ok := parsePDFLiteralString(body, start)
		if ok {
			uris = append(uris, value)
			search = end
		} else {
			search = start + 1
		}
	}
	return uris
}

func parsePDFLiteralString(data []byte, start int) (string, int, bool) {
	if start >= len(data) || data[start] != '(' {
		return "", start, false
	}
	var output strings.Builder
	depth := 1
	for index := start + 1; index < len(data); index++ {
		value := data[index]
		if value == '\\' {
			index++
			if index >= len(data) {
				return "", index, false
			}
			escaped := data[index]
			switch escaped {
			case 'n':
				output.WriteByte('\n')
			case 'r':
				output.WriteByte('\r')
			case 't':
				output.WriteByte('\t')
			case 'b':
				output.WriteByte('\b')
			case 'f':
				output.WriteByte('\f')
			case '\r':
				if index+1 < len(data) && data[index+1] == '\n' {
					index++
				}
			case '\n':
			default:
				if escaped >= '0' && escaped <= '7' {
					octal := int(escaped - '0')
					for count := 1; count < 3 && index+1 < len(data) && data[index+1] >= '0' && data[index+1] <= '7'; count++ {
						index++
						octal = octal*8 + int(data[index]-'0')
					}
					output.WriteByte(byte(octal))
				} else {
					output.WriteByte(escaped)
				}
			}
			continue
		}
		switch value {
		case '(':
			depth++
			output.WriteByte(value)
		case ')':
			depth--
			if depth == 0 {
				return output.String(), index + 1, true
			}
			output.WriteByte(value)
		default:
			output.WriteByte(value)
		}
	}
	return "", len(data), false
}
