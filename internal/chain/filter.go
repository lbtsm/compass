package chain

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/internal/stream"
	"github.com/pkg/errors"
)

func (m *Messenger) filterMosHandler(latestBlock uint64) (int, uint64, error) {
	count := 0
	progressBlock := uint64(0)
	topic := BuildFilterTopic(m.Cfg.Events)
	back, err := m.ListMosLogs(constant.ProjectOfMsger, topic, 1)
	if err != nil {
		return 0, progressBlock, err
	}
	if len(back.List) == 0 {
		return 0, latestBlock, nil
	}

	for _, ele := range back.List {
		progressBlock = ele.BlockNumber
		idx := m.Match(ele.ContractAddress)
		if idx == -1 {
			m.Log.Info("Filter Log Address Not Match", "id", ele.Id, "address", ele.ContractAddress)
			m.Cfg.StartBlock = big.NewInt(ele.Id)
			continue
		}
		if latestBlock-ele.BlockNumber < m.BlockConfirmations.Uint64() {
			m.Log.Debug("Block not ready, will retry", "currentBlock", ele.BlockNumber, "latest", latestBlock)
			continue
		}

		log := MosRespToEthLog(ele)

		send, err := log2Msg(m, log, idx)
		if err != nil {
			return 0, progressBlock, err
		}
		count += send
		m.Cfg.StartBlock = big.NewInt(ele.Id)
	}

	return count, progressBlock, nil
}

func (m *Oracle) filterOracle() error {
	topic := BuildFilterTopic(m.Cfg.Events)

	tmp := []int{}
	var err error
	defer func() {
		if len(tmp) == 0 {
			return
		}
		if err != nil {
			return
		}
		sort.Ints(tmp) // less - big
		if int64(tmp[0]) > m.Cfg.StartBlock.Int64() {
			m.Cfg.StartBlock = big.NewInt(int64(tmp[0]))
		}
	}()
	projectIDs := []int64{constant.ProjectOfOracle, constant.ProjectOfMsger}
	batches := make([]*stream.MosListResp, 0, len(projectIDs))
	for _, pid := range projectIDs {
		back, fetchErr := m.ListMosLogs(pid, topic, 1)
		if fetchErr != nil {
			err = fetchErr
			return err
		}
		batches = append(batches, back)
	}

	latestBlock, latestErr := m.Conn.LatestBlock()
	if latestErr != nil {
		err = latestErr
		return err
	}
	finalized, pending := m.filterBatchesFinalized(batches, latestBlock)
	if !finalized {
		m.Log.Debug("Filter oracle log not finalized", "id", pending.Id, "block", pending.BlockNumber,
			"latest", latestBlock, "confirmations", m.BlockConfirmations)
		return nil
	}

	for _, back := range batches {
		for _, ele := range back.List {
			idx := m.Match(ele.ContractAddress) // 新版 oracle
			if idx == -1 {
				m.Log.Info("Filter Log Address Not Match", "id", ele.Id, "address", ele.ContractAddress)
				tmp = append(tmp, int(ele.Id))
				continue
			}

			log := MosRespToEthLog(ele)
			err = log2Oracle(m, []types.Log{*log}, big.NewInt(0).SetUint64(ele.BlockNumber), ele.Id)
			if err != nil {
				return err
			}
			tmp = append(tmp, int(ele.Id))
		}
	}
	return nil
}

func (m *Oracle) filterBatchesFinalized(batches []*stream.MosListResp, latestBlock *big.Int) (bool, *stream.GetMosResp) {
	for _, batch := range batches {
		for _, ele := range batch.List {
			if m.Match(ele.ContractAddress) == -1 {
				continue
			}
			eventBlock := new(big.Int).SetUint64(ele.BlockNumber)
			if latestBlock.Cmp(eventBlock) < 0 || new(big.Int).Sub(latestBlock, eventBlock).Cmp(m.BlockConfirmations) < 0 {
				return false, ele
			}
		}
	}
	return true, nil
}

func Request(urlPath string) (interface{}, error) {
	return RequestWithAPIKey(urlPath, "")
}

func RequestWithAPIKey(urlPath, apiKey string) (interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request failed")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request get failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("request unauthorized")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "readAll failed")
	}
	ret := stream.CommonResp{}
	err = json.Unmarshal(data, &ret)
	if err != nil {
		return nil, errors.Wrap(err, "unMarshal resp failed")
	}
	if ret.Code != http.StatusOK && ret.Code != 2000 {
		msg := ret.Message
		if msg == "" {
			msg = ret.Msg
		}
		return nil, fmt.Errorf("request code is not success, msg is %s", msg)
	}

	return ret.Data, nil
}
