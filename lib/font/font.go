// Sfnt2Woff is a native go port of the JS library
// https://github.com/fontello/ttf2woff
// that converts sfnt fonts (.ttf and .otf) to .woff fonts

package font

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

var (
	// sfnt2Woff offset
	SFNT_OFFSET_TAG      = 0
	SFNT_OFFSET_CHECKSUM = 4
	SFNT_OFFSET_OFFSET   = 8
	SFNT_OFFSET_LENGTH   = 12

	// sfnt2Woff entry offset
	SFNT_ENTRY_OFFSET_FLAVOR              = 0
	SFNT_ENTRY_OFFSET_VERSION_MAJ         = 4
	SFNT_ENTRY_OFFSET_VERSION_MIN         = 6
	SFNT_ENTRY_OFFSET_CHECKSUM_ADJUSTMENT = 8

	// woff offset
	WOFF_OFFSET_MAGIC            = 0
	WOFF_OFFSET_FLAVOR           = 4
	WOFF_OFFSET_SIZE             = 8
	WOFF_OFFSET_NUM_TABLES       = 12
	WOFF_OFFSET_RESERVED         = 14
	WOFF_OFFSET_SFNT_SIZE        = 16
	WOFF_OFFSET_VERSION_MAJ      = 20
	WOFF_OFFSET_VERSION_MIN      = 22
	WOFF_OFFSET_META_OFFSET      = 24
	WOFF_OFFSET_META_LENGTH      = 28
	WOFF_OFFSET_META_ORIG_LENGTH = 32
	WOFF_OFFSET_PRIV_OFFSET      = 36
	WOFF_OFFSET_PRIV_LENGTH      = 40

	// woff entry offset
	WOFF_ENTRY_OFFSET_TAG          = 0
	WOFF_ENTRY_OFFSET_OFFSET       = 4
	WOFF_ENTRY_OFFSET_COMPR_LENGTH = 8
	WOFF_ENTRY_OFFSET_LENGTH       = 12
	WOFF_ENTRY_OFFSET_CHECKSUM     = 16

	// magic
	MAGIC_WOFF                = 0x774f4646
	MAGIC_CHECKSUM_ADJUSTMENT = 0xb1b0afba

	// sizes
	SIZE_OF_WOFF_HEADER      = 44
	SIZE_OF_WOFF_ENTRY       = 20
	SIZE_OF_SFNT_HEADER      = 12
	SIZE_OF_SFNT_TABLE_ENTRY = 16
)

type TableEntry struct {
	Tag      []byte
	CheckSum uint32
	Offset   uint32
	Length   uint32
}

func longAlign(n uint32) uint32 {
	return (n + 3) & ^uint32(3)
}

func calcChecksum(buf []byte) uint32 {
	var sum uint32 = 0
	var nlongs = len(buf) / 4

	for i := 0; i < nlongs; i++ {
		var t = binary.BigEndian.Uint32(buf[i*4:])
		sum = sum + t
	}
	return sum
}

func Sfnt2Woff(fontBuf []byte) ([]byte, error) {
	numTables := binary.BigEndian.Uint16(fontBuf[4:])

	woffHeader := make([]byte, SIZE_OF_WOFF_HEADER)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_MAGIC:], uint32(MAGIC_WOFF))
	binary.BigEndian.PutUint16(woffHeader[WOFF_OFFSET_NUM_TABLES:], numTables)
	binary.BigEndian.PutUint16(woffHeader[WOFF_OFFSET_SFNT_SIZE:], 0)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_META_OFFSET:], 0)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_META_LENGTH:], 0)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_META_ORIG_LENGTH:], 0)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_PRIV_OFFSET:], 0)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_PRIV_LENGTH:], 0)

	var entries []TableEntry

	for i := 0; i < int(numTables); i++ {
		table := fontBuf[SIZE_OF_SFNT_HEADER+i*SIZE_OF_SFNT_TABLE_ENTRY:]

		entry := TableEntry{
			Tag:      table[SFNT_OFFSET_TAG : SFNT_OFFSET_TAG+4],
			CheckSum: binary.BigEndian.Uint32(table[SFNT_OFFSET_CHECKSUM:]),
			Offset:   binary.BigEndian.Uint32(table[SFNT_OFFSET_OFFSET:]),
			Length:   binary.BigEndian.Uint32(table[SFNT_OFFSET_LENGTH:]),
		}

		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return string(entries[i].Tag) < string(entries[j].Tag)
	})

	sfntSize := uint32(SIZE_OF_SFNT_HEADER + int(numTables)*SIZE_OF_SFNT_TABLE_ENTRY)
	tableInfo := make([]byte, int(numTables)*SIZE_OF_WOFF_ENTRY)

	for i := 0; i < len(entries); i++ {
		tableEntry := entries[i]
		if string(tableEntry.Tag) != "head" {
			alignTable := fontBuf[tableEntry.Offset : tableEntry.Offset+longAlign(tableEntry.Length)]

			if calcChecksum(alignTable) != tableEntry.CheckSum {
				return nil, fmt.Errorf("checksum error in table: %v", string(tableEntry.Tag))
			}
		}

		binary.BigEndian.PutUint32(tableInfo[i*SIZE_OF_WOFF_ENTRY+WOFF_ENTRY_OFFSET_TAG:], binary.BigEndian.Uint32(tableEntry.Tag))
		binary.BigEndian.PutUint32(tableInfo[i*SIZE_OF_WOFF_ENTRY+WOFF_ENTRY_OFFSET_LENGTH:], tableEntry.Length)
		binary.BigEndian.PutUint32(tableInfo[i*SIZE_OF_WOFF_ENTRY+WOFF_ENTRY_OFFSET_CHECKSUM:], tableEntry.CheckSum)

		sfntSize += longAlign(tableEntry.Length)
	}

	sfntOffset := uint32(SIZE_OF_SFNT_HEADER + len(entries)*SIZE_OF_SFNT_TABLE_ENTRY)
	csum := calcChecksum(fontBuf[:SIZE_OF_SFNT_HEADER])
	for i := 0; i < len(entries); i++ {
		tableEntry := entries[i]

		b := make([]byte, SIZE_OF_SFNT_TABLE_ENTRY)
		binary.BigEndian.PutUint32(b[SFNT_OFFSET_TAG:], binary.BigEndian.Uint32(tableEntry.Tag))
		binary.BigEndian.PutUint32(b[SFNT_OFFSET_CHECKSUM:], tableEntry.CheckSum)
		binary.BigEndian.PutUint32(b[SFNT_OFFSET_OFFSET:], sfntOffset)
		binary.BigEndian.PutUint32(b[SFNT_OFFSET_LENGTH:], tableEntry.Length)

		sfntOffset += longAlign(tableEntry.Length)
		csum += calcChecksum(b)
		csum += tableEntry.CheckSum
	}

	var checksumAdjustment = uint32(MAGIC_CHECKSUM_ADJUSTMENT) - csum

	majorVersion := uint16(0)
	minVersion := uint16(1)
	flavor := uint32(0)
	offset := SIZE_OF_WOFF_HEADER + int(numTables)*SIZE_OF_WOFF_ENTRY
	var tableBytes []byte

	for i := 0; i < len(entries); i++ {
		tableEntry := entries[i]

		sfntData := fontBuf[tableEntry.Offset : tableEntry.Offset+tableEntry.Length]
		if string(tableEntry.Tag) == "head" {
			majorVersion = binary.BigEndian.Uint16(sfntData[SFNT_ENTRY_OFFSET_VERSION_MAJ:])
			minVersion = binary.BigEndian.Uint16(sfntData[SFNT_ENTRY_OFFSET_VERSION_MIN:])
			flavor = binary.BigEndian.Uint32(sfntData[SFNT_ENTRY_OFFSET_FLAVOR:])
			binary.BigEndian.PutUint32(sfntData[SFNT_ENTRY_OFFSET_CHECKSUM_ADJUSTMENT:], uint32(checksumAdjustment))
		}

		var res bytes.Buffer
		w := zlib.NewWriter(&res)
		w.Write(sfntData)
		w.Flush()
		w.Close()

		compLength := math.Min(float64(len(res.Bytes())), float64(len(sfntData)))
		length := longAlign(uint32(compLength))

		table := make([]byte, length)
		// only deflate data if the deflated data is actually smaller
		if len(res.Bytes()) >= len(sfntData) {
			copy(table, sfntData)
		} else {
			copy(table, res.Bytes())
		}

		binary.BigEndian.PutUint32(tableInfo[i*SIZE_OF_WOFF_ENTRY+WOFF_ENTRY_OFFSET_OFFSET:], uint32(offset))

		offset += len(table)

		binary.BigEndian.PutUint32(tableInfo[i*SIZE_OF_WOFF_ENTRY+WOFF_ENTRY_OFFSET_COMPR_LENGTH:], uint32(compLength))

		tableBytes = append(tableBytes, table...)

	}

	woffSize := uint32(len(woffHeader) + len(tableInfo) + len(tableBytes))
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_SIZE:], woffSize)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_SFNT_SIZE:], sfntSize)
	binary.BigEndian.PutUint16(woffHeader[WOFF_OFFSET_VERSION_MAJ:], majorVersion)
	binary.BigEndian.PutUint16(woffHeader[WOFF_OFFSET_VERSION_MIN:], minVersion)
	binary.BigEndian.PutUint32(woffHeader[WOFF_OFFSET_FLAVOR:], flavor)

	var out []byte
	out = append(out, woffHeader...)
	out = append(out, tableInfo...)
	out = append(out, tableBytes...)

	return out, nil
}
