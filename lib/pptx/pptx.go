package pptx

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"
)

// Measurements in OOXML are made in English Metric Units (EMUs) where 1 inch = 914,400 EMUs
// The intent is to have a measurement unit that doesn't require floating points when dealing with centimeters, inches, points (DPI).
// Office Open XML (OOXML) http://officeopenxml.com/prPresentation.php
// https://startbigthinksmall.wordpress.com/2010/01/04/points-inches-and-emus-measuring-units-in-office-open-xml/
const SLIDE_WIDTH = 9_144_000
const SLIDE_HEIGHT = 5_143_500
const HEADER_HEIGHT = 392_471

const IMAGE_HEIGHT = SLIDE_HEIGHT - HEADER_HEIGHT

// keep the right aspect ratio: SLIDE_WIDTH / SLIDE_HEIGHT = IMAGE_WIDTH / IMAGE_HEIGHT
const IMAGE_WIDTH = 8_446_273
const IMAGE_ASPECT_RATIO = float64(IMAGE_WIDTH) / float64(IMAGE_HEIGHT)

//go:embed template.pptx
var pptx_template []byte

func copyPptxTemplateTo(w *zip.Writer) error {
	reader := bytes.NewReader(pptx_template)
	zipReader, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		fmt.Printf("error creating zip reader: %v", err)
	}

	for _, f := range zipReader.File {
		if err := w.Copy(f); err != nil {
			return fmt.Errorf("error copying %s: %v", f.Name, err)
		}
	}
	return nil
}

func addFile(zipFile *zip.Writer, filePath, content string) error {
	w, err := zipFile.Create(filePath)
	if err != nil {
		return err
	}
	w.Write([]byte(content))
	return nil
}

const RELS_SLIDE_XML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout7.xml" /><Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/%s.png" /></Relationships>`

func getRelsSlideXml(imageID string) string {
	return fmt.Sprintf(RELS_SLIDE_XML, imageID, imageID)
}

const SLIDE_XML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name="" /><p:cNvGrpSpPr /><p:nvPr /></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0" /><a:ext cx="0" cy="0" /><a:chOff x="0" y="0" /><a:chExt cx="0" cy="0" /></a:xfrm></p:grpSpPr><p:pic><p:nvPicPr><p:cNvPr id="2" name="%s" descr="%s" /><p:cNvPicPr><a:picLocks noChangeAspect="1" /></p:cNvPicPr><p:nvPr /></p:nvPicPr><p:blipFill><a:blip r:embed="%s" /><a:stretch><a:fillRect /></a:stretch></p:blipFill><p:spPr><a:xfrm><a:off x="%d" y="%d" /><a:ext cx="%d" cy="%d" /></a:xfrm><a:prstGeom prst="rect"><a:avLst /></a:prstGeom></p:spPr></p:pic><p:sp><p:nvSpPr><p:cNvPr id="95" name="%s" /><p:cNvSpPr txBox="1" /><p:nvPr /></p:nvSpPr><p:spPr><a:xfrm><a:off x="4001" y="6239" /><a:ext cx="9135998" cy="%d" /></a:xfrm><a:prstGeom prst="rect"><a:avLst /></a:prstGeom><a:ln w="12700"><a:miter lim="400000" /></a:ln><a:extLst><a:ext uri="{C572A759-6A51-4108-AA02-DFA0A04FC94B}"><ma14:wrappingTextBoxFlag xmlns:ma14="http://schemas.microsoft.com/office/mac/drawingml/2011/main" xmlns="" val="1" /></a:ext></a:extLst></p:spPr><p:txBody><a:bodyPr lIns="45719" rIns="45719"><a:spAutoFit /></a:bodyPr><a:lstStyle><a:lvl1pPr><a:defRPr sz="2400" /></a:lvl1pPr></a:lstStyle><a:p>%s</a:p></p:txBody></p:sp></p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping /></p:clrMapOvr></p:sld>`

func getSlideXml(boardPath []string, imageID string, top, left, width, height int) string {
	var slideTitle string
	boardName := boardPath[len(boardPath)-1]
	prefixPath := boardPath[:len(boardPath)-1]
	if len(prefixPath) > 0 {
		prefix := strings.Join(prefixPath, "  /  ") + "  /  "
		slideTitle = fmt.Sprintf(`<a:r><a:t>%s</a:t></a:r><a:r><a:rPr b="1" /><a:t>%s</a:t></a:r>`, prefix, boardName)
	} else {
		slideTitle = fmt.Sprintf(`<a:r><a:rPr b="1" /><a:t>%s</a:t></a:r>`, boardName)
	}
	slideDescription := strings.Join(boardPath, " / ")
	top += HEADER_HEIGHT
	return fmt.Sprintf(SLIDE_XML, slideDescription, slideDescription, imageID, left, top, width, height, slideDescription, HEADER_HEIGHT, slideTitle)
}

func getPresentationXmlRels(slideFileNames []string) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/presProps" Target="presProps.xml" /><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/viewProps" Target="viewProps.xml" /><Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml" /><Relationship Id="rId6" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/tableStyles" Target="tableStyles.xml" /><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml" />`)

	for _, name := range slideFileNames {
		builder.WriteString(fmt.Sprintf(
			`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/%s.xml" />`, name, name,
		))
	}

	builder.WriteString("</Relationships>")

	return builder.String()
}

func getContentTypesXml(slideFileNames []string) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="jpeg" ContentType="image/jpeg" /><Default Extension="png" ContentType="image/png" /><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml" /><Default Extension="xml" ContentType="application/xml" /><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml" /><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml" /><Override PartName="/ppt/presProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presProps+xml" /><Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml" /><Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout10.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout11.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout2.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout3.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout4.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout5.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout6.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout7.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout8.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideLayouts/slideLayout9.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml" /><Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml" />`)

	for _, name := range slideFileNames {
		builder.WriteString(fmt.Sprintf(
			`<Override PartName="/ppt/slides/%s.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml" />`, name,
		))
	}

	builder.WriteString(`<Override PartName="/ppt/tableStyles.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml" /><Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml" /><Override PartName="/ppt/viewProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.viewProps+xml" /></Types>`)
	return builder.String()
}

func getPresentationXml(slideFileNames []string) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" saveSubsetFonts="1" autoCompressPictures="0"><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1" /></p:sldMasterIdLst>`)

	builder.WriteString("<p:sldIdLst>")
	for i, name := range slideFileNames {
		// in the exported presentation, the first slide ID was 256, so keeping it here for compatibility
		builder.WriteString(fmt.Sprintf(`<p:sldId id="%d" r:id="%s" />`, 256+i, name))
	}
	builder.WriteString("</p:sldIdLst>")

	builder.WriteString(fmt.Sprintf(
		`<p:sldSz cx="%d" cy="%d" type="screen4x3" /><p:notesSz cx="6858000" cy="9144000" /><p:defaultTextStyle><a:defPPr><a:defRPr lang="en-US" /></a:defPPr><a:lvl1pPr marL="0" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl1pPr><a:lvl2pPr marL="457200" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl2pPr><a:lvl3pPr marL="914400" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl3pPr><a:lvl4pPr marL="1371600" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl4pPr><a:lvl5pPr marL="1828800" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl5pPr><a:lvl6pPr marL="2286000" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl6pPr><a:lvl7pPr marL="2743200" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl7pPr><a:lvl8pPr marL="3200400" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl8pPr><a:lvl9pPr marL="3657600" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1800" kern="1200"><a:solidFill><a:schemeClr val="tx1" /></a:solidFill><a:latin typeface="+mn-lt" /><a:ea typeface="+mn-ea" /><a:cs typeface="+mn-cs" /></a:defRPr></a:lvl9pPr></p:defaultTextStyle><p:extLst><p:ext uri="{EFAFB233-063F-42B5-8137-9DF3F51BA10A}"><p15:sldGuideLst xmlns:p15="http://schemas.microsoft.com/office/powerpoint/2012/main"><p15:guide id="1" orient="horz" pos="2160"><p15:clr><a:srgbClr val="A4A3A4" /></p15:clr></p15:guide><p15:guide id="2" pos="2880"><p15:clr><a:srgbClr val="A4A3A4" /></p15:clr></p15:guide></p15:sldGuideLst></p:ext></p:extLst></p:presentation>`,
		SLIDE_WIDTH,
		SLIDE_HEIGHT,
	))
	return builder.String()
}

func getCoreXml(title, subject, description, creator string) string {
	var builder strings.Builder

	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	builder.WriteString(`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`)
	builder.WriteString(fmt.Sprintf(`<dc:title>%s</dc:title>`, title))
	builder.WriteString(fmt.Sprintf(`<dc:subject>%s</dc:subject>`, subject))
	builder.WriteString(fmt.Sprintf(`<dc:creator>%s</dc:creator>`, creator))
	builder.WriteString(`<cp:keywords />`)
	builder.WriteString(fmt.Sprintf(`<dc:description>%s</dc:description>`, description))
	builder.WriteString(fmt.Sprintf(`<cp:lastModifiedBy>%s</cp:lastModifiedBy>`, creator))
	builder.WriteString(`<cp:revision>1</cp:revision>`)
	dateTime := time.Now().Format(time.RFC3339)
	builder.WriteString(fmt.Sprintf(`<dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>`, dateTime))
	builder.WriteString(fmt.Sprintf(`<dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified>`, dateTime))
	builder.WriteString(`<cp:category />`)
	builder.WriteString(`</cp:coreProperties>`)

	return builder.String()
}

func getAppXml(slides []*Slide, d2version string) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	builder.WriteString(`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">`)
	builder.WriteString(`<TotalTime>1</TotalTime>`)
	builder.WriteString(`<Words>0</Words>`)
	builder.WriteString(`<Application>D2</Application>`)
	builder.WriteString(`<PresentationFormat>On-screen Show (4:3)</PresentationFormat>`)
	builder.WriteString(`<Paragraphs>0</Paragraphs>`)
	builder.WriteString(fmt.Sprintf(`<Slides>%d</Slides>`, len(slides)))
	builder.WriteString(`<Notes>0</Notes>`)
	builder.WriteString(`<HiddenSlides>0</HiddenSlides>`)
	builder.WriteString(`<MMClips>0</MMClips>`)
	builder.WriteString(`<ScaleCrop>false</ScaleCrop>`)
	builder.WriteString(`<HeadingPairs>`)
	builder.WriteString(`<vt:vector size="6" baseType="variant">`)
	builder.WriteString(`<vt:variant>`)
	builder.WriteString(`<vt:lpstr>Fonts</vt:lpstr>`)
	builder.WriteString(`</vt:variant>`)
	builder.WriteString(`<vt:variant>`)
	builder.WriteString(`<vt:i4>2</vt:i4>`)
	builder.WriteString(`</vt:variant>`)
	builder.WriteString(`<vt:variant>`)
	builder.WriteString(`<vt:lpstr>Theme</vt:lpstr>`)
	builder.WriteString(`</vt:variant>`)
	builder.WriteString(`<vt:variant>`)
	builder.WriteString(`<vt:i4>1</vt:i4>`)
	builder.WriteString(`</vt:variant>`)
	builder.WriteString(`<vt:variant>`)
	builder.WriteString(`<vt:lpstr>Slide Titles</vt:lpstr>`)
	builder.WriteString(`</vt:variant>`)
	builder.WriteString(`<vt:variant>`)
	builder.WriteString(fmt.Sprintf(`<vt:i4>%d</vt:i4>`, len(slides)))
	builder.WriteString(`</vt:variant>`)
	builder.WriteString(`</vt:vector>`)
	builder.WriteString(`</HeadingPairs>`)
	builder.WriteString(`<TitlesOfParts>`)
	// number of entries, len(slides) + Office Theme + 2 Fonts
	builder.WriteString(fmt.Sprintf(`<vt:vector size="%d" baseType="lpstr">`, len(slides)+3))
	builder.WriteString(`<vt:lpstr>Arial</vt:lpstr>`)
	builder.WriteString(`<vt:lpstr>Calibri</vt:lpstr>`)
	builder.WriteString(`<vt:lpstr>Office Theme</vt:lpstr>`)
	for _, slide := range slides {
		builder.WriteString(fmt.Sprintf(`<vt:lpstr>%s</vt:lpstr>`, strings.Join(slide.BoardPath, "/")))
	}
	builder.WriteString(`</vt:vector>`)
	builder.WriteString(`</TitlesOfParts>`)
	builder.WriteString(`<Manager></Manager>`)
	builder.WriteString(`<Company></Company>`)
	builder.WriteString(`<LinksUpToDate>false</LinksUpToDate>`)
	builder.WriteString(`<SharedDoc>false</SharedDoc>`)
	builder.WriteString(`<HyperlinkBase></HyperlinkBase>`)
	builder.WriteString(`<HyperlinksChanged>false</HyperlinksChanged>`)
	builder.WriteString(fmt.Sprintf(`<AppVersion>%s</AppVersion>`, d2version))
	builder.WriteString(`</Properties>`)
	return builder.String()
}
