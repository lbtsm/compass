package mapo

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/compass/internal/op"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/pkg/errors"
)

const maxOPReceiptWorkers = 30

type OPReceiptReader interface {
	OpReceipt(context.Context, common.Hash) (*ethclient.OpReceipt, error)
}

type OPReceiptBuilder struct {
	reader      OPReceiptReader
	workerLimit int
}

func NewOPReceiptBuilder(reader OPReceiptReader) *OPReceiptBuilder {
	return &OPReceiptBuilder{
		reader:      reader,
		workerLimit: maxOPReceiptWorkers,
	}
}

func (b *OPReceiptBuilder) Build(ctx context.Context, receipts []*types.Receipt) (op.Receipts, error) {
	if len(receipts) == 0 {
		return op.Receipts{}, nil
	}
	type job struct {
		idx     int
		receipt *types.Receipt
	}
	type result struct {
		idx     int
		receipt *op.Receipt
		err     error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan job)
	results := make(chan result)
	workerCount := min(b.workerLimit, len(receipts))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for current := range jobs {
				receipt, err := b.buildOne(ctx, current.receipt)
				select {
				case results <- result{idx: current.idx, receipt: receipt, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for idx, receipt := range receipts {
			select {
			case jobs <- job{idx: idx, receipt: receipt}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	ret := make(op.Receipts, len(receipts))
	for result := range results {
		if result.err != nil {
			return nil, fmt.Errorf("build OP receipt %s at index %d: %w",
				receipts[result.idx].TxHash, result.idx, result.err)
		}
		ret[result.idx] = result.receipt
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

func (b *OPReceiptBuilder) buildOne(ctx context.Context, receipt *types.Receipt) (*op.Receipt, error) {
	raw, err := b.reader.OpReceipt(ctx, receipt.TxHash)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, errors.New("empty OP receipt")
	}
	version, err := parseOptionalHexUint64("depositReceiptVersion", raw.DepositReceiptVersion)
	if err != nil {
		return nil, err
	}
	nonce, err := parseOptionalHexUint64("depositNonce", raw.DepositNonce)
	if err != nil {
		return nil, err
	}
	return &op.Receipt{
		Receipt:               receipt,
		DepositReceiptVersion: version,
		DepositNonce:          nonce,
	}, nil
}

func parseOptionalHexUint64(field, value string) (*uint64, error) {
	if value == "" {
		return nil, nil
	}
	parsed, ok := new(big.Int).SetString(strings.TrimPrefix(value, "0x"), 16)
	if !ok || !parsed.IsUint64() {
		return nil, fmt.Errorf("invalid %s %q", field, value)
	}
	ret := parsed.Uint64()
	return &ret, nil
}
