package apple2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

var (
	woz1Magic = []byte{'W', 'O', 'Z', '1', 0xFF, 0x0A, 0x0D, 0x0A}
	woz2Magic = []byte{'W', 'O', 'Z', '2', 0xFF, 0x0A, 0x0D, 0x0A}
)

// bitstreamToNibbles simulates the Apple II Disk II controller shift register
// over 1.5 revolutions of a track bitstream to extract synchronized disk nibbles.
func bitstreamToNibbles(bits []byte, bitCount int) []byte {
	if bitCount <= 0 || (bitCount+7)/8 > len(bits) {
		return nil
	}
	totalBits := bitCount + bitCount/2
	nibbles := make([]byte, 0, totalBits/8)
	var reg byte
	for b := 0; b < totalBits; b++ {
		idx := b % bitCount
		bit := (bits[idx>>3] >> (7 - (idx & 7))) & 1
		reg = (reg << 1) | bit
		if (reg & 0x80) != 0 {
			nibbles = append(nibbles, reg)
			reg = 0
		}
	}
	return nibbles
}

func openWOZ(data []byte) (Image, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("invalid WOZ file: too short (%d bytes)", len(data))
	}
	isWOZ1 := bytes.Equal(data[:8], woz1Magic)
	isWOZ2 := bytes.Equal(data[:8], woz2Magic)
	if !isWOZ1 && !isWOZ2 {
		return nil, fmt.Errorf("invalid WOZ file header")
	}

	expectedCRC := binary.LittleEndian.Uint32(data[8:12])
	if expectedCRC != 0 {
		actualCRC := crc32.ChecksumIEEE(data[12:])
		if actualCRC != expectedCRC {
			return nil, fmt.Errorf("WOZ CRC32 mismatch: expected 0x%08X, got 0x%08X", expectedCRC, actualCRC)
		}
	}

	var (
		info []byte
		tmap []byte
		trks []byte
	)
	off := 12
	for off+8 <= len(data) {
		chunkID := string(data[off : off+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		if chunkSize < 0 || off+8+chunkSize > len(data) {
			return nil, fmt.Errorf("truncated WOZ chunk %q", chunkID)
		}
		chunkData := data[off+8 : off+8+chunkSize]
		switch chunkID {
		case "INFO":
			info = chunkData
		case "TMAP":
			tmap = chunkData
		case "TRKS":
			trks = chunkData
		}
		off += 8 + chunkSize
	}

	if len(info) < 37 {
		return nil, fmt.Errorf("missing or invalid WOZ INFO chunk")
	}
	diskType := info[1]
	if diskType != 1 {
		return nil, fmt.Errorf("unsupported WOZ disk type: %d (only 5.25\" Disk II supported)", diskType)
	}
	if len(tmap) < 160 {
		return nil, fmt.Errorf("missing or invalid WOZ TMAP chunk")
	}
	if len(trks) == 0 {
		return nil, fmt.Errorf("missing WOZ TRKS chunk")
	}

	var tracks [35][]byte
	for t := 0; t < 35; t++ {
		tIdx := tmap[t*4]
		if tIdx == 0xFF {
			continue
		}
		if isWOZ1 {
			const woz1TrackLen = 6656
			trkOff := int(tIdx) * woz1TrackLen
			if trkOff+woz1TrackLen > len(trks) {
				continue
			}
			trkEntry := trks[trkOff : trkOff+woz1TrackLen]
			bytesUsed := int(binary.LittleEndian.Uint16(trkEntry[6646:6648]))
			bitCount := int(binary.LittleEndian.Uint16(trkEntry[6648:6650]))
			if bytesUsed > 6646 {
				bytesUsed = 6646
			}
			tracks[t] = bitstreamToNibbles(trkEntry[:bytesUsed], bitCount)
		} else {
			entryOff := int(tIdx) * 8
			if entryOff+8 > len(trks) || entryOff+8 > 1280 {
				continue
			}
			startBlock := int(binary.LittleEndian.Uint16(trks[entryOff : entryOff+2]))
			blockCount := int(binary.LittleEndian.Uint16(trks[entryOff+2 : entryOff+4]))
			bitCount := int(binary.LittleEndian.Uint32(trks[entryOff+4 : entryOff+8]))
			byteStart := startBlock * 512
			byteEnd := (startBlock + blockCount) * 512
			if startBlock == 0 || byteEnd > len(data) {
				continue
			}
			tracks[t] = bitstreamToNibbles(data[byteStart:byteEnd], bitCount)
		}
	}

	return decodeGCRTracks(tracks)
}

