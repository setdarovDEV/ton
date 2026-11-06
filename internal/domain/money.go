package domain

import (
	"fmt"
	"strings"
)

// NanoPerTON is the number of nanotons in one TON.
const NanoPerTON int64 = 1_000_000_000

// FormatTON renders a nanoton amount as a decimal TON string without trailing zeros.
func FormatTON(nano int64) string {
	neg := nano < 0
	if neg {
		nano = -nano
	}
	whole := nano / NanoPerTON
	frac := nano % NanoPerTON
	s := fmt.Sprintf("%d", whole)
	if frac != 0 {
		f := strings.TrimRight(fmt.Sprintf("%09d", frac), "0")
		s += "." + f
	}
	if neg {
		s = "-" + s
	}
	return s
}
