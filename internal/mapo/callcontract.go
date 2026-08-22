// Copyright 2021 Compass Systems
// SPDX-License-Identifier: LGPL-3.0-only

package mapo

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	maptypes "github.com/mapprotocol/atlas/core/types"
	"github.com/mapprotocol/compass/internal/mapprotocol"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/mapprotocol/compass/pkg/msg"
)

func AssembleEthProof(conn *ethclient.Client, log *types.Log, receipts []*types.Receipt, header *maptypes.Header,
	method string, fromChainID msg.ChainId, proofType int64, sign [][]byte) ([]byte, error) {
	return NewProofAssembler(conn).AssembleEth(log, receipts, header, method, fromChainID, proofType, sign)
}

func AssembleMapProof(conn *ethclient.Client, log *types.Log, receipts []*types.Receipt,
	header *maptypes.Header, fromChainID msg.ChainId, method, zkURL string,
	proofType int64, sign [][]byte) (uint64, []byte, error) {
	return NewProofAssembler(conn).AssembleMap(log, receipts, header, fromChainID, method, zkURL, proofType, sign)
}

func Key2Hex(str []byte, proofLength int) []byte {
	ret := make([]byte, 0)
	if len(ret)+1 == proofLength {
		ret = append(ret, str...)
	} else {
		for _, b := range str {
			ret = append(ret, b/16)
			ret = append(ret, b%16)
		}
	}
	return ret
}

type TxReceipt struct {
	ReceiptType       string  `json:"receipt_type"`
	PostStateOrStatus string  `json:"post_state_or_status"`
	CumulativeGasUsed string  `json:"cumulative_gas_used"`
	Bloom             string  `json:"bloom"`
	Logs              []TxLog `json:"logs"`
}

type TxLog struct {
	Address common.Address `json:"address"`
	Topics  []string       `json:"topics"`
	Data    string         `json:"data"`
}

func ConvertNearReceipt(h *mapprotocol.TxReceipt) *TxReceipt {
	logs := make([]TxLog, 0, len(h.Logs))
	for _, log := range h.Logs {
		topics := make([]string, 0, len(log.Topics))
		for _, topic := range log.Topics {
			topics = append(topics, "0x"+common.Bytes2Hex(topic))
		}
		logs = append(logs, TxLog{
			Address: log.Addr,
			Topics:  topics,
			Data:    "0x" + common.Bytes2Hex(log.Data),
		})
	}
	return &TxReceipt{
		ReceiptType:       h.ReceiptType.String(),
		PostStateOrStatus: "0x" + common.Bytes2Hex(h.PostStateOrStatus),
		CumulativeGasUsed: h.CumulativeGasUsed.String(),
		Bloom:             "0x" + common.Bytes2Hex(h.Bloom),
		Logs:              logs,
	}
}
