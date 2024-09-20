package server

type GetProofRequest struct {
	TxHash      string `json:"tx_hash"`
	BlockNumber string `json:"block_number"`
	ChainID     string `json:"chain_id"`
	LogIndex    int    `json:"log_index"`
	ToChain     string `json:"to_chain"`
	ProofType   int    `json:"proof_type"`
}

type GetProofResponse struct {
	Proof string `json:"proof"`
}

// ErrResponse 定义了发生错误时的返回消息.
type ErrResponse struct {
	// Code 指定了业务错误码.
	Code string `json:"code"`

	// Message 包含了可以直接对外展示的错误信息.
	Message string `json:"message"`
}

type SuccessResponse struct {
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}
