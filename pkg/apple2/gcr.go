package apple2

import (
	"encoding/binary"
	"fmt"
)

const (
	nibTrackSize = 6656
	nibDiskSize  = 35 * nibTrackSize // 232,960 bytes
)

// 5-and-3 GCR disk nibble table (32 entries -> 5-bit values 0x00..0x1F)
var gcr53Table = [32]byte{
	0xAB, 0xAD, 0xAE, 0xAF, 0xB5, 0xB6, 0xB7, 0xBA,
	0xBB, 0xBD, 0xBE, 0xBF, 0xD6, 0xD7, 0xDA, 0xDB,
	0xDD, 0xDE, 0xDF, 0xEA, 0xEB, 0xED, 0xEE, 0xEF,
	0xF5, 0xF6, 0xF7, 0xFA, 0xFB, 0xFD, 0xFE, 0xFF,
}

// 6-and-2 GCR disk nibble table (64 entries -> 6-bit values 0x00..0x3F)
var gcr62Table = [64]byte{
	0x96, 0x97, 0x9A, 0x9B, 0x9D, 0x9E, 0x9F, 0xA6,
	0xA7, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF, 0xB2, 0xB3,
	0xB4, 0xB5, 0xB6, 0xB7, 0xB9, 0xBA, 0xBB, 0xBC,
	0xBD, 0xBE, 0xBF, 0xCB, 0xCD, 0xCE, 0xCF, 0xD3,
	0xD6, 0xD7, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE,
	0xDF, 0xE5, 0xE6, 0xE7, 0xE9, 0xEA, 0xEB, 0xEC,
	0xED, 0xEE, 0xEF, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6,
	0xF7, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF,
}

var (
	decode53 [256]byte
	decode62 [256]byte
)

// Physical sector ID (in 16-sector D5 AA 96 address field) to DOS 3.3 logical sector
var physToDosSector = [16]int{
	0, 7, 14, 6, 13, 5, 12, 4, 11, 3, 10, 2, 9, 1, 8, 15,
}

func init() {
	for i := range decode53 {
		decode53[i] = 0xFF
		decode62[i] = 0xFF
	}
	for i, b := range gcr53Table {
		decode53[b] = byte(i)
	}
	for i, b := range gcr62Table {
		decode62[b] = byte(i)
	}
}

func decode44(odd, even byte) byte {
	return ((odd << 1) | 1) & even
}

// decode53Sector decodes 411 5-and-3 GCR nibbles (410 data + 1 checksum) into 256 bytes.
func decode53Sector(nibs []byte) ([]byte, bool) {
	if len(nibs) < 411 {
		return nil, false
	}
	var buf [410]byte
	var acc byte
	for i := 0; i < 410; i++ {
		v := decode53[nibs[i]]
		if v == 0xFF {
			return nil, false
		}
		acc ^= v
		if i < 154 {
			buf[153-i] = acc
		} else {
			buf[i] = acc
		}
	}
	chk := decode53[nibs[410]]
	if chk == 0xFF || (acc^chk) != 0 {
		return nil, false
	}

	out := make([]byte, 256)
	for j := 0; j < 51; j++ {
		i := 50 - j
		thr1 := buf[i]
		thr2 := buf[51+i]
		thr3 := buf[102+i]

		out[5*j+0] = (buf[154+i] << 3) | ((thr1 >> 2) & 0x07)
		out[5*j+1] = (buf[154+51+i] << 3) | ((thr2 >> 2) & 0x07)
		out[5*j+2] = (buf[154+102+i] << 3) | ((thr3 >> 2) & 0x07)
		out[5*j+3] = (buf[154+153+i] << 3) | ((thr1 & 0x02) << 1) | (thr2 & 0x02) | ((thr3 & 0x02) >> 1)
		out[5*j+4] = (buf[154+204+i] << 3) | ((thr1 & 0x01) << 2) | ((thr2 & 0x01) << 1) | (thr3 & 0x01)
	}
	out[255] = (buf[409] << 3) | (buf[153] & 0x07)
	return out, true
}

// decode62Sector decodes 343 6-and-2 GCR nibbles (342 data + 1 checksum) into 256 bytes.
func decode62Sector(nibs []byte) ([]byte, bool) {
	if len(nibs) < 343 {
		return nil, false
	}
	var aux [86]byte
	var pri [256]byte
	var acc byte
	for i := 0; i < 86; i++ {
		v := decode62[nibs[i]]
		if v == 0xFF {
			return nil, false
		}
		acc ^= v
		aux[85-i] = acc
	}
	for i := 0; i < 256; i++ {
		v := decode62[nibs[86+i]]
		if v == 0xFF {
			return nil, false
		}
		acc ^= v
		pri[i] = acc
	}
	chk := decode62[nibs[342]]
	if chk == 0xFF || (acc^chk) != 0 {
		return nil, false
	}

	out := make([]byte, 256)
	for i := 0; i < 256; i++ {
		a := aux[85-(i%86)] >> ((i / 86) * 2)
		low2 := ((a & 0x01) << 1) | ((a >> 1) & 0x01)
		out[i] = (pri[i] << 2) | low2
	}
	return out, true
}

func openNIB(data []byte) (Image, error) {
	if len(data) != nibDiskSize {
		return nil, fmt.Errorf("invalid NIB disk size: expected %d bytes, got %d", nibDiskSize, len(data))
	}
	var tracks [35][]byte
	for t := 0; t < 35; t++ {
		trk := data[t*nibTrackSize : (t+1)*nibTrackSize]
		wrapped := make([]byte, nibTrackSize+1024)
		copy(wrapped, trk)
		copy(wrapped[nibTrackSize:], trk[:1024])
		tracks[t] = wrapped
	}
	return decodeGCRTracks(tracks)
}

func decodeGCRTracks(tracks [35][]byte) (Image, error) {
	data16 := make([]byte, 35*16*256)
	data13 := make([]byte, 35*13*256)
	headers16 := 0
	headers13 := 0

	for t := 0; t < 35; t++ {
		nibs := tracks[t]
		if len(nibs) < 500 {
			continue
		}
		var found16 [16]bool
		var found13 [13]bool

		for i := 0; i < len(nibs)-14; i++ {
			if nibs[i] != 0xD5 || nibs[i+1] != 0xAA {
				continue
			}
			addrType := nibs[i+2]
			if addrType != 0x96 && addrType != 0xB5 {
				continue
			}

			vol := decode44(nibs[i+3], nibs[i+4])
			trk := decode44(nibs[i+5], nibs[i+6])
			sec := decode44(nibs[i+7], nibs[i+8])
			chk := decode44(nibs[i+9], nibs[i+10])
			if (vol^trk^sec) != chk || int(trk) != t {
				continue
			}

			if addrType == 0x96 && sec < 16 {
				headers16++
				if found16[sec] {
					continue
				}
				for j := i + 11; j < i+100 && j+3+343 <= len(nibs); j++ {
					if nibs[j] == 0xD5 && nibs[j+1] == 0xAA {
						if nibs[j+2] == 0xAD {
							if dec, ok := decode62Sector(nibs[j+3 : j+3+343]); ok {
								found16[sec] = true
								dosSec := physToDosSector[sec]
								copy(data16[(t*16+dosSec)*256:], dec)
							}
						}
						break
					}
				}
			} else if addrType == 0xB5 && sec < 13 {
				headers13++
				if found13[sec] {
					continue
				}
				for j := i + 11; j < i+100 && j+3+411 <= len(nibs); j++ {
					if nibs[j] == 0xD5 && nibs[j+1] == 0xAA {
						if nibs[j+2] == 0xAD {
							if dec, ok := decode53Sector(nibs[j+3 : j+3+411]); ok {
								found13[sec] = true
								copy(data13[(t*13+int(sec))*256:], dec)
							}
						}
						break
					}
				}
			}
		}
	}

	if headers16 > 0 {
		if img, err := open140K(data16); err == nil {
			return img, nil
		}
	}
	if headers13 > 0 && isDOS13VTOC(data13) {
		return newDOSDisk(data13, 13, false)
	}
	return nil, fmt.Errorf("unrecognized Apple II GCR disk filesystem")
}

func isDOS13VTOC(data []byte) bool {
	vtoc, err := readDOSSector(data, 17, 0, 13, false)
	if err != nil {
		return false
	}
	catTrack := vtoc[1]
	catSector := vtoc[2]
	tracksPerDisk := vtoc[0x34]
	sectorsPerTrack := vtoc[0x35]
	bytesPerSector := binary.LittleEndian.Uint16(vtoc[0x36:0x38])
	return catTrack == 17 && catSector >= 1 && catSector < 13 &&
		tracksPerDisk == 35 && sectorsPerTrack == 13 && bytesPerSector == 256
}

