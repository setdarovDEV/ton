package ton

import "encoding/json"

// Wire types for tonapi.io v2 (only the fields we consume).

type apiAccountAddress struct {
	Address string `json:"address"`
}

type apiCollection struct {
	Address       string             `json:"address"`
	NextItemIndex int64              `json:"next_item_index"`
	Owner         *apiAccountAddress `json:"owner"`
	Metadata      json.RawMessage    `json:"metadata"`
}

type apiItem struct {
	Address    string             `json:"address"`
	Index      int64              `json:"index"`
	Owner      *apiAccountAddress `json:"owner"`
	Collection *struct {
		Address string `json:"address"`
	} `json:"collection"`
	Metadata json.RawMessage `json:"metadata"`
	Sale     *struct {
		Address string             `json:"address"`
		Owner   *apiAccountAddress `json:"owner"`
		Price   struct {
			Value string `json:"value"`
		} `json:"price"`
	} `json:"sale"`
}

type apiItems struct {
	NFTItems []apiItem `json:"nft_items"`
}

type apiMessage struct {
	Hash        string             `json:"hash"`
	CreatedLT   uint64             `json:"created_lt"`
	Value       int64              `json:"value"`
	Bounced     bool               `json:"bounced"`
	Destination *apiAccountAddress `json:"destination"`
	Source      *apiAccountAddress `json:"source"`
	OpCode      string             `json:"op_code"`
}

type apiTransaction struct {
	Hash    string            `json:"hash"`
	LT      uint64            `json:"lt"`
	Account apiAccountAddress `json:"account"`
	Success bool              `json:"success"`
	Utime   int64             `json:"utime"`
	InMsg   *apiMessage       `json:"in_msg"`
	OutMsgs []apiMessage      `json:"out_msgs"`
}

type apiTransactions struct {
	Transactions []apiTransaction `json:"transactions"`
}

type apiPublicKey struct {
	PublicKey string `json:"public_key"`
}

type apiStackRecord struct {
	Type  string `json:"type"`
	Num   string `json:"num"`
	Cell  string `json:"cell"`
	Slice string `json:"slice"`
}

type apiMethodResult struct {
	Success  bool             `json:"success"`
	ExitCode int              `json:"exit_code"`
	Stack    []apiStackRecord `json:"stack"`
}

type apiError struct {
	Error string `json:"error"`
}

// metadataView extracts the display fields from TEP-64 metadata.
type metadataView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Image       string `json:"image"`
}
