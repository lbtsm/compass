package tx

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type ReceiptReader interface {
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
}

type MissingReceiptPolicy uint8

const (
	RetryNotFound MissingReceiptPolicy = iota
	PreserveNotFound

	maxReceiptWorkers    = 30
	receiptRetryInterval = 100 * time.Millisecond
	receiptFetchTimeout  = 30 * time.Second
)

type ReceiptFetcher struct {
	reader        ReceiptReader
	policy        MissingReceiptPolicy
	workerLimit   int
	timeout       time.Duration
	retryInterval time.Duration
}

func NewReceiptFetcher(reader ReceiptReader, policy MissingReceiptPolicy) *ReceiptFetcher {
	return &ReceiptFetcher{
		reader:        reader,
		policy:        policy,
		workerLimit:   maxReceiptWorkers,
		timeout:       receiptFetchTimeout,
		retryInterval: receiptRetryInterval,
	}
}

func (f *ReceiptFetcher) Fetch(ctx context.Context, hashes []common.Hash) ([]*types.Receipt, error) {
	if len(hashes) == 0 {
		return []*types.Receipt{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	type job struct {
		idx  int
		hash common.Hash
	}
	type result struct {
		idx     int
		receipt *types.Receipt
		err     error
	}

	ctx, stopWorkers := context.WithCancel(ctx)
	defer stopWorkers()
	jobs := make(chan job)
	results := make(chan result)
	workerCount := min(f.workerLimit, len(hashes))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for current := range jobs {
				receipt, err := f.fetchOne(ctx, current.hash)
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
		for idx, hash := range hashes {
			select {
			case jobs <- job{idx: idx, hash: hash}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	receipts := make([]*types.Receipt, len(hashes))
	for result := range results {
		if result.err != nil {
			return nil, fmt.Errorf("fetch receipt %s at index %d: %w", hashes[result.idx], result.idx, result.err)
		}
		receipts[result.idx] = result.receipt
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func (f *ReceiptFetcher) fetchOne(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	for {
		receipt, err := f.reader.TransactionReceipt(ctx, hash)
		if err == nil {
			if receipt == nil && f.policy == RetryNotFound {
				return nil, errors.New("empty receipt")
			}
			return receipt, nil
		}
		if !errors.Is(err, ethereum.NotFound) {
			return nil, err
		}
		if f.policy == PreserveNotFound {
			return nil, nil
		}

		timer := time.NewTimer(f.retryInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		}
	}
}
