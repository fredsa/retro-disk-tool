package util

import (
	"fmt"
	"io"
	"strings"

	"github.com/beevik/go6502/cpu"
	"github.com/beevik/go6502/disasm"
)

// Disassemble6502 disassembles 6502 machine code from data[startOffset:endOffset]
// loaded at baseAddr in memory, writing one instruction per line to w.
func Disassemble6502(w io.Writer, data []byte, baseAddr uint16, startOffset, endOffset int) {
	if startOffset < 0 {
		startOffset = 0
	}
	if endOffset > len(data) {
		endOffset = len(data)
	}
	if startOffset >= endOffset {
		return
	}
	m := cpu.NewFlatMemory()
	m.StoreBytes(baseAddr, data[:endOffset])
	c := cpu.NewCPU(cpu.NMOS, m)
	endAddr := uint32(baseAddr) + uint32(endOffset)
	for addr := uint32(baseAddr) + uint32(startOffset); addr < endAddr; {
		line, next := disasm.Disassemble(c, uint16(addr), disasm.ShowBasic, "", nil)
		fmt.Fprintf(w, "  %s\n", strings.TrimRight(line, " "))
		if uint32(next) <= addr {
			break
		}
		addr = uint32(next)
	}
}
