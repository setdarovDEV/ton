// Package codes embeds the compiled Getgems contract code (built with
// Blueprint from the vendored FunC sources under /contracts) so the backend can
// derive sale contract addresses deterministically.
package codes

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/xssnick/tonutils-go/tvm/cell"
)

//go:embed *.compiled.json
var fs embed.FS

type compiled struct {
	Hex string `json:"hex"`
}

// Load returns the code cell of a compiled contract by its Blueprint name,
// e.g. "NftFixpriceSaleV3R3".
func Load(name string) (*cell.Cell, error) {
	raw, err := fs.ReadFile(name + ".compiled.json")
	if err != nil {
		return nil, fmt.Errorf("read compiled %s: %w", name, err)
	}
	var c compiled
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("decode compiled %s: %w", name, err)
	}
	b, err := hex.DecodeString(c.Hex)
	if err != nil {
		return nil, fmt.Errorf("decode hex %s: %w", name, err)
	}
	code, err := cell.FromBOC(b)
	if err != nil {
		return nil, fmt.Errorf("parse boc %s: %w", name, err)
	}
	return code, nil
}
