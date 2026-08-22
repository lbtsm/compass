package tx

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/compass/pkg/ethclient"
)

func GetTxsHashByBlockNumber(conn *ethclient.Client, number *big.Int) ([]common.Hash, error) {
	block, err := conn.MAPBlockByNumber(context.Background(), number)
	if err != nil {
		return nil, err
	}

	txs := make([]common.Hash, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		txs = append(txs, common.HexToHash(tx.Hash))
	}
	return txs, nil
}

func GetReceiptsByTxsHash(conn *ethclient.Client, txsHash []common.Hash) ([]*types.Receipt, error) {
	return NewReceiptFetcher(conn, RetryNotFound).Fetch(context.Background(), txsHash)
}

func GetMaticReceiptsByTxsHash(conn *ethclient.Client, txsHash []common.Hash) ([]*types.Receipt, error) {
	return NewReceiptFetcher(conn, PreserveNotFound).Fetch(context.Background(), txsHash)
}
