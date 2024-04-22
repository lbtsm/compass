package near

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/pkg/util"

	"github.com/mapprotocol/compass/internal/near"
	"github.com/mapprotocol/compass/mapprotocol"
	"github.com/mapprotocol/compass/msg"
	"github.com/mapprotocol/near-api-go/pkg/client/block"
)

var NearEpochSize = big.NewInt(43200)

type Maintainer struct {
	*CommonListen
	syncedHeight *big.Int
}

func NewMaintainer(cs *CommonListen) *Maintainer {
	return &Maintainer{
		CommonListen: cs,
	}
}

func (m *Maintainer) Sync() error {
	m.log.Debug("Starting listener...")
	go func() {
		err := m.sync()
		if err != nil {
			m.log.Error("Polling blocks failed", "err", err)
		}
	}()

	return nil
}

// sync function of Maintainer will poll for the latest block and proceed to parse the associated events as it sees new blocks.
// Polling begins at the block defined in `m.cfg.startBlock`. Failed attempts to fetch the latest block or parse
// a block will be retried up to RetryLimit times before continuing to the next block.
func (m Maintainer) sync() error {
	for {
		select {
		case <-m.stop:
			return errors.New("polling terminated")
		default:
			latestBlock, err := m.conn.LatestBlock()
			if err != nil {
				m.log.Error("Unable to get latest block", "block", latestBlock, "err", err)
				time.Sleep(RetryInterval)
				continue
			}

			if m.cfg.SyncToMap {
				// listen when catchup
				m.log.Info("Sync Header to Map Chain", "target", latestBlock)
				err = m.toMapChain(latestBlock)
				if err != nil {
					m.log.Error("Failed to listen header for block", "block", latestBlock, "err", err)
					time.Sleep(constant.QueryRetryInterval)
					util.Alarm(context.Background(), fmt.Sprintf("near sync header failed, err is %s", err.Error()))
					continue
				}
			}

			m.latestBlock.Height = big.NewInt(0).Set(latestBlock)
			m.latestBlock.LastUpdated = time.Now()
		}
	}
}

// toMapChain listen header from current chain to Map chain
func (m *Maintainer) toMapChain(latestBlock *big.Int) error {
	height, err := mapprotocol.Get2MapHeight(m.cfg.Id)
	if err != nil {
		return err
	}
	if latestBlock.Cmp(height) == -1 {
		return nil
	}

	blocks := new(big.Int).Sub(latestBlock, height)
	gap := new(big.Int).Sub(NearEpochSize, blocks).Int64()
	if gap > 0 {
		m.log.Info("wait for the next light client block to be generated", "target", new(big.Int).Add(height, NearEpochSize).Uint64())
		time.Sleep(time.Duration(gap/10) * time.Second)
		return nil
	}

	count := new(big.Int).Div(blocks, NearEpochSize).Uint64()
	number := height.Uint64()
	id := big.NewInt(0).SetUint64(uint64(m.cfg.Id))
	for i := uint64(0); i < count; i++ {
		blockDetails, err := m.conn.Client().BlockDetails(context.Background(), block.BlockID(number))
		if err != nil {
			m.log.Error("failed to get block", "err", err, "number", number)
			return err
		}
		m.log.Info("get block complete", "number", number, "hash", blockDetails.Header.Hash)

		lightBlock, err := m.conn.Client().NextLightClientBlock(context.Background(), blockDetails.Header.Hash)
		if err != nil {
			m.log.Error("failed to get next light client block", "err", err, "number", lightBlock.InnerLite.Height, "hash", lightBlock.NextBlockInnerHash)
			return err
		}
		m.log.Info("get next light client block complete", "number", lightBlock.InnerLite.Height, "hash", lightBlock.NextBlockInnerHash)

		number = lightBlock.InnerLite.Height

		message := msg.NewSyncToMap(m.cfg.Id, m.cfg.MapChainID, []interface{}{id, near.Borshify(lightBlock)}, m.msgCh)
		err = m.router.Send(message)
		if err != nil {
			m.log.Error("subscription error: failed to route message", "err", err)
			return nil
		}
		err = m.waitUntilMsgHandled(1)
		if err != nil {
			return err
		}
	}
	return nil
}
