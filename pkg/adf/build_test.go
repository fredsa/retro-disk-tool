package adf_test

import (
	"encoding/binary"
	"strings"
	"unicode"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/adf"
)

// diskBuilder builds small OFS or FFS DD disks for tests.
type diskBuilder struct {
	data []byte
	ffs  bool
	next uint32 // next free block
	used []uint32
}

const (
	rootBlock   = 880
	bitmapBlock = 881
)

func newDisk(ffs bool, name string) *diskBuilder {
	b := &diskBuilder{data: make([]byte, adf.DiskSize), ffs: ffs, next: 882}
	copy(b.data, "DOS\x00")
	if ffs {
		b.data[3] = 1
	}
	root := b.block(rootBlock)
	b.put(root, 0, adf.TypeHeader)
	b.put(root, 12, 72) // hash table size
	b.put(root, 312, 0xFFFFFFFF)
	b.put(root, 316, bitmapBlock)
	b.put(root, 508, adf.SecTypeRoot)
	b.setName(root, name)
	b.used = append(b.used, rootBlock, bitmapBlock)
	return b
}

func (b *diskBuilder) block(n uint32) []byte { return b.data[n*512 : (n+1)*512] }

func (b *diskBuilder) put(blk []byte, off int, v uint32) { binary.BigEndian.PutUint32(blk[off:], v) }

func (b *diskBuilder) setName(blk []byte, name string) {
	blk[432] = byte(len(name))
	copy(blk[433:], name)
}

func (b *diskBuilder) alloc() uint32 {
	n := b.next
	b.next++
	b.used = append(b.used, n)
	return n
}

// hash is the AmigaDOS name hash.
func hash(name string) uint32 {
	h := uint32(len(name))
	for _, c := range strings.ToUpper(name) {
		h = (h*13 + uint32(unicode.ToUpper(c))) & 0x7FF
	}
	return h % 72
}

// link adds a header block to a directory's hash table (chaining on
// collisions).
func (b *diskBuilder) link(dir, header uint32, name string) {
	slot := 24 + int(hash(name))*4
	if first := binary.BigEndian.Uint32(b.block(dir)[slot:]); first != 0 {
		b.put(b.block(header), 496, first)
	}
	b.put(b.block(dir), slot, header)
}

func (b *diskBuilder) header(parent uint32, name string, secType int32) uint32 {
	n := b.alloc()
	blk := b.block(n)
	b.put(blk, 0, adf.TypeHeader)
	b.put(blk, 4, n)
	b.put(blk, 500, parent)
	b.put(blk, 508, uint32(secType))
	b.put(blk, 420, 17844) // 2026-11-07
	b.put(blk, 424, 14*60)
	b.setName(blk, name)
	b.link(parent, n, name)
	return n
}

func (b *diskBuilder) dir(parent uint32, name string) uint32 {
	return b.header(parent, name, adf.SecTypeDir)
}

// file writes a file: data blocks (with a 24 byte header on OFS) listed in
// the header block and, past 72 blocks, in extension blocks.
func (b *diskBuilder) file(parent uint32, name string, content []byte) uint32 {
	h := b.header(parent, name, adf.SecTypeFile)
	b.put(b.block(h), 324, uint32(len(content)))
	chunk := 488
	if b.ffs {
		chunk = 512
	}
	var blocks []uint32
	for off := 0; off < len(content); off += chunk {
		blocks = append(blocks, b.alloc())
	}
	for i, n := range blocks {
		part := content[i*chunk : min((i+1)*chunk, len(content))]
		blk := b.block(n)
		if b.ffs {
			copy(blk, part)
			continue
		}
		b.put(blk, 0, adf.TypeData)
		b.put(blk, 4, h)
		b.put(blk, 8, uint32(i+1))
		b.put(blk, 12, uint32(len(part)))
		if i+1 < len(blocks) {
			b.put(blk, 16, blocks[i+1])
		}
		copy(blk[24:], part)
	}
	if len(blocks) > 0 {
		b.put(b.block(h), 16, blocks[0])
	}
	// Block tables: 72 pointers per block, stored from the end.
	table := h
	for i := 0; i < len(blocks); i += 72 {
		if i > 0 {
			ext := b.alloc()
			blk := b.block(ext)
			b.put(blk, 0, adf.TypeList)
			b.put(blk, 4, ext)
			b.put(blk, 500, h)
			fileType := int32(adf.SecTypeFile)
			b.put(blk, 508, uint32(fileType))
			b.put(b.block(table), 504, ext)
			table = ext
		}
		group := blocks[i:min(i+72, len(blocks))]
		b.put(b.block(table), 8, uint32(len(group)))
		for j, n := range group {
			b.put(b.block(table), 24+(71-j)*4, n)
		}
	}
	return h
}

// bytes finishes the bitmap (a set bit is a free block) and returns the
// image.
func (b *diskBuilder) bytes() []byte {
	bm := b.block(bitmapBlock)
	for blk := 2; blk < adf.TotalSectors; blk++ {
		word := 4 + (blk-2)/32*4
		b.put(bm, word, binary.BigEndian.Uint32(bm[word:])|1<<((blk-2)%32))
	}
	for _, n := range b.used {
		word := 4 + (int(n)-2)/32*4
		b.put(bm, word, binary.BigEndian.Uint32(bm[word:])&^(1<<((int(n)-2)%32)))
	}
	return b.data
}
