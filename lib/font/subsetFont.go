/*
 * The following code is part of from https://github.com/jung-kurt/gofpdf
 *
 * Copyright (c) 2019 Arteom Korotkiy (Gmail: arteomkorotkiy)
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package font

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// flags
const symbolWords = 1 << 0
const symbolScale = 1 << 3
const symbolContinue = 1 << 5
const symbolAllScale = 1 << 6
const symbol2x2 = 1 << 7

type fontBoxType struct {
	Xmin, Ymin, Xmax, Ymax int
}

type utf8FontFile struct {
	fileReader           *fileReader
	LastRune             int
	tableDescriptions    map[string]*tableDescription
	outTablesData        map[string][]byte
	symbolPosition       []int
	charSymbolDictionary map[int]int
	Ascent               int
	Descent              int
	Bbox                 fontBoxType
	CapHeight            int
	StemV                int
	ItalicAngle          int
	Flags                int
	UnderlinePosition    float64
	UnderlineThickness   float64
	CharWidths           []int
	DefaultWidth         float64
	symbolData           map[int]map[string][]int
	CodeSymbolDictionary map[int]int
}

type tableDescription struct {
	name     string
	checksum []int
	position int
	size     int
}

type fileReader struct {
	readerPosition int64
	array          []byte
}

func (fr *fileReader) Read(s int) []byte {
	b := fr.array[fr.readerPosition : fr.readerPosition+int64(s)]
	fr.readerPosition += int64(s)
	return b
}

func (fr *fileReader) seek(shift int64, flag int) (int64, error) {
	if flag == 0 {
		fr.readerPosition = shift
	} else if flag == 1 {
		fr.readerPosition += shift
	} else if flag == 2 {
		fr.readerPosition = int64(len(fr.array)) - shift
	}
	return int64(fr.readerPosition), nil
}

func newUTF8Font(reader *fileReader) *utf8FontFile {
	utf := utf8FontFile{
		fileReader: reader,
	}
	return &utf
}

func (utf *utf8FontFile) generateTableDescriptions() {

	tablesCount := utf.readUint16()
	_ = utf.readUint16()
	_ = utf.readUint16()
	_ = utf.readUint16()
	utf.tableDescriptions = make(map[string]*tableDescription)

	for i := 0; i < tablesCount; i++ {
		record := tableDescription{
			name:     utf.readTableName(),
			checksum: []int{utf.readUint16(), utf.readUint16()},
			position: utf.readUint32(),
			size:     utf.readUint32(),
		}
		utf.tableDescriptions[record.name] = &record
	}
}

func (utf *utf8FontFile) readTableName() string {
	return string(utf.fileReader.Read(4))
}

func (utf *utf8FontFile) readUint16() int {
	s := utf.fileReader.Read(2)
	return (int(s[0]) << 8) + int(s[1])
}

func (utf *utf8FontFile) readUint32() int {
	s := utf.fileReader.Read(4)
	return (int(s[0]) * 16777216) + (int(s[1]) << 16) + (int(s[2]) << 8) + int(s[3]) // 	16777216  = 1<<24
}

func (utf *utf8FontFile) calcInt32(x, y []int) []int {
	answer := make([]int, 2)
	if y[1] > x[1] {
		x[1] += 1 << 16
		x[0]++
	}
	answer[1] = x[1] - y[1]
	if y[0] > x[0] {
		x[0] += 1 << 16
	}
	answer[0] = x[0] - y[0]
	answer[0] = answer[0] & 0xFFFF
	return answer
}

func (utf *utf8FontFile) generateChecksum(data []byte) []int {
	if (len(data) % 4) != 0 {
		for i := 0; (len(data) % 4) != 0; i++ {
			data = append(data, 0)
		}
	}
	answer := []int{0x0000, 0x0000}
	for i := 0; i < len(data); i += 4 {
		answer[0] += (int(data[i]) << 8) + int(data[i+1])
		answer[1] += (int(data[i+2]) << 8) + int(data[i+3])
		answer[0] += answer[1] >> 16
		answer[1] = answer[1] & 0xFFFF
		answer[0] = answer[0] & 0xFFFF
	}
	return answer
}

func (utf *utf8FontFile) seek(shift int) {
	_, _ = utf.fileReader.seek(int64(shift), 0)
}

func (utf *utf8FontFile) skip(delta int) {
	_, _ = utf.fileReader.seek(int64(delta), 1)
}

// SeekTable position
func (utf *utf8FontFile) SeekTable(name string) int {
	return utf.seekTable(name, 0)
}

func (utf *utf8FontFile) seekTable(name string, offsetInTable int) int {
	_, _ = utf.fileReader.seek(int64(utf.tableDescriptions[name].position+offsetInTable), 0)
	return int(utf.fileReader.readerPosition)
}

func (utf *utf8FontFile) readInt16() int16 {
	s := utf.fileReader.Read(2)
	a := (int16(s[0]) << 8) + int16(s[1])
	if (int(a) & (1 << 15)) == 0 {
		a = int16(int(a) - (1 << 16))
	}
	return a
}

func (utf *utf8FontFile) getUint16(pos int) int {
	_, _ = utf.fileReader.seek(int64(pos), 0)
	s := utf.fileReader.Read(2)
	return (int(s[0]) << 8) + int(s[1])
}

func (utf *utf8FontFile) splice(stream []byte, offset int, value []byte) []byte {
	stream = append([]byte{}, stream...)
	return append(append(stream[:offset], value...), stream[offset+len(value):]...)
}

func (utf *utf8FontFile) insertUint16(stream []byte, offset int, value int) []byte {
	up := make([]byte, 2)
	binary.BigEndian.PutUint16(up, uint16(value))
	return utf.splice(stream, offset, up)
}

func (utf *utf8FontFile) getRange(pos, length int) []byte {
	_, _ = utf.fileReader.seek(int64(pos), 0)
	if length < 1 {
		return make([]byte, 0)
	}
	s := utf.fileReader.Read(length)
	return s
}

func (utf *utf8FontFile) getTableData(name string) []byte {
	desckrip := utf.tableDescriptions[name]
	if desckrip == nil {
		return nil
	}
	if desckrip.size == 0 {
		return nil
	}
	_, _ = utf.fileReader.seek(int64(desckrip.position), 0)
	s := utf.fileReader.Read(desckrip.size)
	return s
}

func (utf *utf8FontFile) setOutTable(name string, data []byte) {
	if data == nil {
		return
	}
	if name == "head" {
		data = utf.splice(data, 8, []byte{0, 0, 0, 0})
	}
	utf.outTablesData[name] = data
}

func (utf *utf8FontFile) generateCMAP() map[int][]int {
	cmapPosition := utf.SeekTable("cmap")
	utf.skip(2)
	cmapTableCount := utf.readUint16()
	runeCmapPosition := 0
	for i := 0; i < cmapTableCount; i++ {
		system := utf.readUint16()
		coder := utf.readUint16()
		position := utf.readUint32()
		oldPosition := utf.fileReader.readerPosition
		if (system == 3 && coder == 1) || system == 0 {
			format := utf.getUint16(cmapPosition + position)
			if format == 4 {
				runeCmapPosition = cmapPosition + position
				break
			}
		}
		utf.seek(int(oldPosition))
	}

	if runeCmapPosition == 0 {
		fmt.Printf("Font does not have cmap for Unicode\n")
		return nil
	}

	symbolCharDictionary := make(map[int][]int)
	charSymbolDictionary := make(map[int]int)
	utf.generateSCCSDictionaries(runeCmapPosition, symbolCharDictionary, charSymbolDictionary)

	utf.charSymbolDictionary = charSymbolDictionary

	return symbolCharDictionary
}

func (utf *utf8FontFile) parseSymbols(usedRunes map[int]int) (map[int]int, map[int]int, map[int]int, []int) {
	symbolCollection := map[int]int{0: 0}
	charSymbolPairCollection := make(map[int]int)
	for _, char := range usedRunes {
		if _, OK := utf.charSymbolDictionary[char]; OK {
			symbolCollection[utf.charSymbolDictionary[char]] = char
			charSymbolPairCollection[char] = utf.charSymbolDictionary[char]

		}
		utf.LastRune = max(utf.LastRune, char)
	}

	begin := utf.tableDescriptions["glyf"].position

	symbolArray := make(map[int]int)
	symbolCollectionKeys := keySortInt(symbolCollection)

	symbolCounter := 0
	maxRune := 0
	for _, oldSymbolIndex := range symbolCollectionKeys {
		maxRune = max(maxRune, symbolCollection[oldSymbolIndex])
		symbolArray[oldSymbolIndex] = symbolCounter
		symbolCounter++
	}
	charSymbolPairCollectionKeys := keySortInt(charSymbolPairCollection)
	runeSymbolPairCollection := make(map[int]int)
	for _, runa := range charSymbolPairCollectionKeys {
		runeSymbolPairCollection[runa] = symbolArray[charSymbolPairCollection[runa]]
	}
	utf.CodeSymbolDictionary = runeSymbolPairCollection

	symbolCollectionKeys = keySortInt(symbolCollection)
	for _, oldSymbolIndex := range symbolCollectionKeys {
		_, symbolArray, symbolCollection, symbolCollectionKeys = utf.getSymbols(oldSymbolIndex, &begin, symbolArray, symbolCollection, symbolCollectionKeys)
	}

	return runeSymbolPairCollection, symbolArray, symbolCollection, symbolCollectionKeys
}

func (utf *utf8FontFile) generateCMAPTable(cidSymbolPairCollection map[int]int, numSymbols int) []byte {
	cidSymbolPairCollectionKeys := keySortInt(cidSymbolPairCollection)
	cidID := 0
	cidArray := make(map[int][]int)
	prevCid := -2
	prevSymbol := -1
	for _, cid := range cidSymbolPairCollectionKeys {
		if cid == (prevCid+1) && cidSymbolPairCollection[cid] == (prevSymbol+1) {
			if n, OK := cidArray[cidID]; !OK || n == nil {
				cidArray[cidID] = make([]int, 0)
			}
			cidArray[cidID] = append(cidArray[cidID], cidSymbolPairCollection[cid])
		} else {
			cidID = cid
			cidArray[cidID] = make([]int, 0)
			cidArray[cidID] = append(cidArray[cidID], cidSymbolPairCollection[cid])
		}
		prevCid = cid
		prevSymbol = cidSymbolPairCollection[cid]
	}
	cidArrayKeys := keySortArrayRangeMap(cidArray)
	segCount := len(cidArray) + 1

	searchRange := 1
	entrySelector := 0
	for searchRange*2 <= segCount {
		searchRange = searchRange * 2
		entrySelector = entrySelector + 1
	}
	searchRange = searchRange * 2
	rangeShift := segCount*2 - searchRange

	data := []int{0, 1, 3, 1, 0, 12, 4}

	cmap := []int{segCount * 2, searchRange, entrySelector, rangeShift}
	for _, start := range cidArrayKeys {
		endCode := start + (len(cidArray[start]) - 1)
		cmap = append(cmap, endCode)
	}
	cmap = append(cmap, 0xFFFF)
	cmap = append(cmap, 0)

	cmap = append(cmap, cidArrayKeys...)
	cmap = append(cmap, 0xFFFF)
	for _, cidKey := range cidArrayKeys {
		idDelta := -(cidKey - cidArray[cidKey][0])
		cmap = append(cmap, idDelta)
	}
	cmap = append(cmap, 1)
	for range cidArray {
		cmap = append(cmap, 0)

	}
	cmap = append(cmap, 0)
	for _, start := range cidArrayKeys {
		cmap = append(cmap, cidArray[start]...)
	}
	cmap = append(cmap, 0)

	// Calculating cmap length based off of fpdf https://github.com/Setasign/FPDF/blob/f4104a04c9a3f95c4c26a0a0531abebcc980987a/makefont/ttfparser.php#L476
	// https://learn.microsoft.com/en-us/typography/opentype/spec/cmap#format-4-segment-mapping-to-delta-values
	// The 3 extra bytes are for encodingID, offset, format, which are also part of the cmap
	length := (3 + len(cmap)) * 2
	data = append(data, length, 0)
	data = append(data, cmap...)

	cmapstr := make([]byte, 0)
	for _, cm := range data {
		cmapstr = append(cmapstr, packUint16(cm)...)
	}

	return cmapstr
}

// GenerateCutFont fill utf8FontFile from .utf file, only with runes from usedRunes
func (utf *utf8FontFile) GenerateCutFont(usedRunes map[int]int) []byte {
	utf.fileReader.readerPosition = 0
	utf.symbolPosition = make([]int, 0)
	utf.charSymbolDictionary = make(map[int]int)
	utf.tableDescriptions = make(map[string]*tableDescription)
	utf.outTablesData = make(map[string][]byte)
	utf.Ascent = 0
	utf.Descent = 0
	utf.skip(4)
	utf.LastRune = 0
	utf.generateTableDescriptions()

	utf.SeekTable("head")
	utf.skip(50)
	LocaFormat := utf.readUint16()

	utf.SeekTable("hhea")
	utf.skip(34)
	metricsCount := utf.readUint16()
	oldMetrics := metricsCount

	utf.SeekTable("maxp")
	utf.skip(4)
	numSymbols := utf.readUint16()

	symbolCharDictionary := utf.generateCMAP()
	if symbolCharDictionary == nil {
		return nil
	}

	utf.parseHMTXTable(metricsCount, numSymbols, symbolCharDictionary, 1.0)

	utf.parseLOCATable(LocaFormat, numSymbols)

	cidSymbolPairCollection, symbolArray, symbolCollection, symbolCollectionKeys := utf.parseSymbols(usedRunes)

	metricsCount = len(symbolCollection)
	numSymbols = metricsCount

	utf.setOutTable("name", utf.getTableData("name"))
	utf.setOutTable("cvt ", utf.getTableData("cvt "))
	utf.setOutTable("fpgm", utf.getTableData("fpgm"))
	utf.setOutTable("prep", utf.getTableData("prep"))
	utf.setOutTable("gasp", utf.getTableData("gasp"))

	postTable := utf.getTableData("post")
	postTable = append(append([]byte{0x00, 0x03, 0x00, 0x00}, postTable[4:16]...), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}...)
	utf.setOutTable("post", postTable)

	delete(cidSymbolPairCollection, 0)

	utf.setOutTable("cmap", utf.generateCMAPTable(cidSymbolPairCollection, numSymbols))

	symbolData := utf.getTableData("glyf")

	offsets := make([]int, 0)
	glyfData := make([]byte, 0)
	pos := 0
	hmtxData := make([]byte, 0)
	utf.symbolData = make(map[int]map[string][]int, 0)

	for _, originalSymbolIdx := range symbolCollectionKeys {
		hm := utf.getMetrics(oldMetrics, originalSymbolIdx)
		hmtxData = append(hmtxData, hm...)

		offsets = append(offsets, pos)
		symbolPos := utf.symbolPosition[originalSymbolIdx]
		symbolLen := utf.symbolPosition[originalSymbolIdx+1] - symbolPos
		data := symbolData[symbolPos : symbolPos+symbolLen]
		var up int
		if symbolLen > 0 {
			up = unpackUint16(data[0:2])
		}

		if symbolLen > 2 && (up&(1<<15)) != 0 {
			posInSymbol := 10
			flags := symbolContinue
			nComponentElements := 0
			for (flags & symbolContinue) != 0 {
				nComponentElements++
				up = unpackUint16(data[posInSymbol : posInSymbol+2])
				flags = up
				up = unpackUint16(data[posInSymbol+2 : posInSymbol+4])
				symbolIdx := up
				if _, OK := utf.symbolData[originalSymbolIdx]; !OK {
					utf.symbolData[originalSymbolIdx] = make(map[string][]int)
				}
				if _, OK := utf.symbolData[originalSymbolIdx]["compSymbols"]; !OK {
					utf.symbolData[originalSymbolIdx]["compSymbols"] = make([]int, 0)
				}
				utf.symbolData[originalSymbolIdx]["compSymbols"] = append(utf.symbolData[originalSymbolIdx]["compSymbols"], symbolIdx)
				data = utf.insertUint16(data, posInSymbol+2, symbolArray[symbolIdx])
				posInSymbol += 4
				if (flags & symbolWords) != 0 {
					posInSymbol += 4
				} else {
					posInSymbol += 2
				}
				if (flags & symbolScale) != 0 {
					posInSymbol += 2
				} else if (flags & symbolAllScale) != 0 {
					posInSymbol += 4
				} else if (flags & symbol2x2) != 0 {
					posInSymbol += 8
				}
			}
		}

		glyfData = append(glyfData, data...)
		pos += symbolLen
		if pos%4 != 0 {
			padding := 4 - (pos % 4)
			glyfData = append(glyfData, make([]byte, padding)...)
			pos += padding
		}
	}

	offsets = append(offsets, pos)
	utf.setOutTable("glyf", glyfData)

	utf.setOutTable("hmtx", hmtxData)

	locaData := make([]byte, 0)
	if ((pos + 1) >> 1) > 0xFFFF {
		LocaFormat = 1
		for _, offset := range offsets {
			locaData = append(locaData, packUint32(offset)...)
		}
	} else {
		LocaFormat = 0
		for _, offset := range offsets {
			locaData = append(locaData, packUint16(offset/2)...)
		}
	}
	utf.setOutTable("loca", locaData)

	headData := utf.getTableData("head")
	headData = utf.insertUint16(headData, 50, LocaFormat)
	utf.setOutTable("head", headData)

	hheaData := utf.getTableData("hhea")
	hheaData = utf.insertUint16(hheaData, 34, metricsCount)
	utf.setOutTable("hhea", hheaData)

	maxp := utf.getTableData("maxp")
	maxp = utf.insertUint16(maxp, 4, numSymbols)
	utf.setOutTable("maxp", maxp)

	os2Data := utf.getTableData("OS/2")
	utf.setOutTable("OS/2", os2Data)

	return utf.assembleTables()
}

func (utf *utf8FontFile) getSymbols(originalSymbolIdx int, start *int, symbolSet map[int]int, SymbolsCollection map[int]int, SymbolsCollectionKeys []int) (*int, map[int]int, map[int]int, []int) {
	symbolPos := utf.symbolPosition[originalSymbolIdx]
	symbolSize := utf.symbolPosition[originalSymbolIdx+1] - symbolPos
	if symbolSize == 0 {
		return start, symbolSet, SymbolsCollection, SymbolsCollectionKeys
	}
	utf.seek(*start + symbolPos)

	lineCount := utf.readInt16()

	if lineCount < 0 {
		utf.skip(8)
		flags := symbolContinue
		for flags&symbolContinue != 0 {
			flags = utf.readUint16()
			symbolIndex := utf.readUint16()
			if _, OK := symbolSet[symbolIndex]; !OK {
				symbolSet[symbolIndex] = len(SymbolsCollection)
				SymbolsCollection[symbolIndex] = 1
				SymbolsCollectionKeys = append(SymbolsCollectionKeys, symbolIndex)
			}
			oldPosition, _ := utf.fileReader.seek(0, 1)
			_, _, _, SymbolsCollectionKeys = utf.getSymbols(symbolIndex, start, symbolSet, SymbolsCollection, SymbolsCollectionKeys)
			utf.seek(int(oldPosition))
			if flags&symbolWords != 0 {
				utf.skip(4)
			} else {
				utf.skip(2)
			}
			if flags&symbolScale != 0 {
				utf.skip(2)
			} else if flags&symbolAllScale != 0 {
				utf.skip(4)
			} else if flags&symbol2x2 != 0 {
				utf.skip(8)
			}
		}
	}
	return start, symbolSet, SymbolsCollection, SymbolsCollectionKeys
}

func (utf *utf8FontFile) parseHMTXTable(numberOfHMetrics, numSymbols int, symbolToChar map[int][]int, scale float64) {
	var widths int
	start := utf.SeekTable("hmtx")
	arrayWidths := 0
	var arr []int
	utf.CharWidths = make([]int, 256*256)
	charCount := 0
	arr = unpackUint16Array(utf.getRange(start, numberOfHMetrics*4))
	for symbol := 0; symbol < numberOfHMetrics; symbol++ {
		arrayWidths = arr[(symbol*2)+1]
		if _, OK := symbolToChar[symbol]; OK || symbol == 0 {

			if arrayWidths >= (1 << 15) {
				arrayWidths = 0
			}
			if symbol == 0 {
				utf.DefaultWidth = scale * float64(arrayWidths)
				continue
			}
			for _, char := range symbolToChar[symbol] {
				if char != 0 && char != 65535 {
					widths = int(math.Round(scale * float64(arrayWidths)))
					if widths == 0 {
						widths = 65535
					}
					if char < 196608 {
						utf.CharWidths[char] = widths
						charCount++
					}
				}
			}
		}
	}
	diff := numSymbols - numberOfHMetrics
	for pos := 0; pos < diff; pos++ {
		symbol := pos + numberOfHMetrics
		if _, OK := symbolToChar[symbol]; OK {
			for _, char := range symbolToChar[symbol] {
				if char != 0 && char != 65535 {
					widths = int(math.Round(scale * float64(arrayWidths)))
					if widths == 0 {
						widths = 65535
					}
					if char < 196608 {
						utf.CharWidths[char] = widths
						charCount++
					}
				}
			}
		}
	}
	utf.CharWidths[0] = charCount
}

func (utf *utf8FontFile) getMetrics(metricCount, gid int) []byte {
	start := utf.SeekTable("hmtx")
	var metrics []byte
	if gid < metricCount {
		utf.seek(start + (gid * 4))
		metrics = utf.fileReader.Read(4)
	} else {
		utf.seek(start + ((metricCount - 1) * 4))
		metrics = utf.fileReader.Read(2)
		utf.seek(start + (metricCount * 2) + (gid * 2))
		metrics = append(metrics, utf.fileReader.Read(2)...)
	}
	return metrics
}

func (utf *utf8FontFile) parseLOCATable(format, numSymbols int) {
	start := utf.SeekTable("loca")
	utf.symbolPosition = make([]int, 0)
	if format == 0 {
		data := utf.getRange(start, (numSymbols*2)+2)
		arr := unpackUint16Array(data)
		for n := 0; n <= numSymbols; n++ {
			utf.symbolPosition = append(utf.symbolPosition, arr[n+1]*2)
		}
	} else if format == 1 {
		data := utf.getRange(start, (numSymbols*4)+4)
		arr := unpackUint32Array(data)
		for n := 0; n <= numSymbols; n++ {
			utf.symbolPosition = append(utf.symbolPosition, arr[n+1])
		}
	} else {
		fmt.Printf("Unknown loca table format %d\n", format)
		return
	}
}

func (utf *utf8FontFile) generateSCCSDictionaries(runeCmapPosition int, symbolCharDictionary map[int][]int, charSymbolDictionary map[int]int) {
	maxRune := 0
	utf.seek(runeCmapPosition + 2)
	size := utf.readUint16()
	rim := runeCmapPosition + size
	utf.skip(2)

	segmentSize := utf.readUint16() / 2
	utf.skip(6)
	completers := make([]int, 0)
	for i := 0; i < segmentSize; i++ {
		completers = append(completers, utf.readUint16())
	}
	utf.skip(2)
	beginners := make([]int, 0)
	for i := 0; i < segmentSize; i++ {
		beginners = append(beginners, utf.readUint16())
	}
	sizes := make([]int, 0)
	for i := 0; i < segmentSize; i++ {
		sizes = append(sizes, int(utf.readInt16()))
	}
	readerPositionStart := utf.fileReader.readerPosition
	positions := make([]int, 0)
	for i := 0; i < segmentSize; i++ {
		positions = append(positions, utf.readUint16())
	}
	var symbol int
	for n := 0; n < segmentSize; n++ {
		completePosition := completers[n] + 1
		for char := beginners[n]; char < completePosition; char++ {
			if positions[n] == 0 {
				symbol = (char + sizes[n]) & 0xFFFF
			} else {
				position := (char-beginners[n])*2 + positions[n]
				position = int(readerPositionStart) + 2*n + position
				if position >= rim {
					symbol = 0
				} else {
					symbol = utf.getUint16(position)
					if symbol != 0 {
						symbol = (symbol + sizes[n]) & 0xFFFF
					}
				}
			}
			charSymbolDictionary[char] = symbol
			if char < 196608 {
				maxRune = max(char, maxRune)
			}
			symbolCharDictionary[symbol] = append(symbolCharDictionary[symbol], char)
		}
	}
}

func max(i, n int) int {
	if n > i {
		return n
	}
	return i
}

func (utf *utf8FontFile) assembleTables() []byte {
	answer := make([]byte, 0)
	tablesCount := len(utf.outTablesData)
	findSize := 1
	writer := 0
	for findSize*2 <= tablesCount {
		findSize = findSize * 2
		writer = writer + 1
	}
	findSize = findSize * 16
	rOffset := tablesCount*16 - findSize

	answer = append(answer, packHeader(0x00010000, tablesCount, findSize, writer, rOffset)...)

	tables := utf.outTablesData
	tablesNames := keySortStrings(tables)

	offset := 12 + tablesCount*16
	begin := 0

	for _, name := range tablesNames {
		if name == "head" {
			begin = offset
		}
		answer = append(answer, []byte(name)...)
		checksum := utf.generateChecksum(tables[name])
		answer = append(answer, pack2Uint16(checksum[0], checksum[1])...)
		answer = append(answer, pack2Uint32(offset, len(tables[name]))...)
		paddedLength := (len(tables[name]) + 3) &^ 3
		offset = offset + paddedLength
	}

	for _, key := range tablesNames {
		data := append([]byte{}, tables[key]...)
		data = append(data, []byte{0, 0, 0}...)
		answer = append(answer, data[:(len(data)&^3)]...)
	}

	checksum := utf.generateChecksum([]byte(answer))
	checksum = utf.calcInt32([]int{0xB1B0, 0xAFBA}, checksum)
	answer = utf.splice(answer, (begin + 8), pack2Uint16(checksum[0], checksum[1]))
	return answer
}

func unpackUint16Array(data []byte) []int {
	answer := make([]int, 1)
	r := bytes.NewReader(data)
	bs := make([]byte, 2)
	var e error
	var c int
	c, e = r.Read(bs)
	for e == nil && c > 0 {
		answer = append(answer, int(binary.BigEndian.Uint16(bs)))
		c, e = r.Read(bs)
	}
	return answer
}

func unpackUint32Array(data []byte) []int {
	answer := make([]int, 1)
	r := bytes.NewReader(data)
	bs := make([]byte, 4)
	var e error
	var c int
	c, e = r.Read(bs)
	for e == nil && c > 0 {
		answer = append(answer, int(binary.BigEndian.Uint32(bs)))
		c, e = r.Read(bs)
	}
	return answer
}

func unpackUint16(data []byte) int {
	return int(binary.BigEndian.Uint16(data))
}

func packHeader(N uint32, n1, n2, n3, n4 int) []byte {
	answer := make([]byte, 0)
	bs4 := make([]byte, 4)
	binary.BigEndian.PutUint32(bs4, N)
	answer = append(answer, bs4...)
	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, uint16(n1))
	answer = append(answer, bs...)
	binary.BigEndian.PutUint16(bs, uint16(n2))
	answer = append(answer, bs...)
	binary.BigEndian.PutUint16(bs, uint16(n3))
	answer = append(answer, bs...)
	binary.BigEndian.PutUint16(bs, uint16(n4))
	answer = append(answer, bs...)
	return answer
}

func pack2Uint16(n1, n2 int) []byte {
	answer := make([]byte, 0)
	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, uint16(n1))
	answer = append(answer, bs...)
	binary.BigEndian.PutUint16(bs, uint16(n2))
	answer = append(answer, bs...)
	return answer
}

func pack2Uint32(n1, n2 int) []byte {
	answer := make([]byte, 0)
	bs := make([]byte, 4)
	binary.BigEndian.PutUint32(bs, uint32(n1))
	answer = append(answer, bs...)
	binary.BigEndian.PutUint32(bs, uint32(n2))
	answer = append(answer, bs...)
	return answer
}

func packUint32(n1 int) []byte {
	bs := make([]byte, 4)
	binary.BigEndian.PutUint32(bs, uint32(n1))
	return bs
}

func packUint16(n1 int) []byte {
	bs := make([]byte, 2)
	binary.BigEndian.PutUint16(bs, uint16(n1))
	return bs
}

func keySortStrings(s map[string][]byte) []string {
	keys := make([]string, len(s))
	i := 0
	for key := range s {
		keys[i] = key
		i++
	}
	sort.Strings(keys)
	return keys
}

func keySortInt(s map[int]int) []int {
	keys := make([]int, len(s))
	i := 0
	for key := range s {
		keys[i] = key
		i++
	}
	sort.Ints(keys)
	return keys
}

func keySortArrayRangeMap(s map[int][]int) []int {
	keys := make([]int, len(s))
	i := 0
	for key := range s {
		keys[i] = key
		i++
	}
	sort.Ints(keys)
	return keys
}

// UTF8CutFont is a utility function that generates a TrueType font composed
// only of the runes included in cutset. The rune glyphs are copied from This
// function is demonstrated in ExampleUTF8CutFont().
func UTF8CutFont(inBuf []byte, cutset string) (outBuf []byte) {
	f := newUTF8Font(&fileReader{readerPosition: 0, array: inBuf})
	runes := map[int]int{}
	for i, r := range cutset {
		runes[i] = int(r)
	}
	outBuf = f.GenerateCutFont(runes)
	return
}
