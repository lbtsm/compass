package service

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/compass/pkg/ethclient"

	"github.com/mapprotocol/compass/chains"
	"github.com/mapprotocol/compass/internal/butter"
	"github.com/mapprotocol/compass/internal/chain"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/internal/expose"
	"github.com/mapprotocol/compass/internal/stream"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pkg/errors"
)

type ProofSrv struct {
	cfg *expose.Config
	pri *ecdsa.PrivateKey
}

func NewProof(cfg *expose.Config, pri *ecdsa.PrivateKey) *ProofSrv {
	return &ProofSrv{cfg: cfg, pri: pri}
}

func (s *ProofSrv) TxExec(req *stream.TxExecOfRequest) (map[string]interface{}, error) {
	switch req.Status {
	case constant.StatusOfRelayFailed:
		return s.RouterRetryMessageIn(s.cfg.Other.Butter, req.RelayChain, req.RelayTxHash)
	case constant.StatusOfSwapFailed, constant.StatusOfDesFailed:
		if req.Slippage == "" {
			req.Slippage = "100"
		}
		if !strings.HasPrefix(req.DesTxHash, "0x") {
			req.DesTxHash = "0x" + req.DesTxHash
		}
		return s.RouterExecSwap(s.cfg.Other.Butter, req.DesChain, req.DesTxHash, req.Slippage, req.Entrance)
	case constant.StatusOfInit:
		desChain := req.DesChain
		desChainInt, _ := strconv.ParseInt(desChain, 10, 64)
		srcChainInt, _ := strconv.ParseInt(req.SrcChain, 10, 64)
		if srcChainInt != constant.MapChainId && desChainInt != constant.MapChainId {
			desChain = strconv.FormatInt(constant.MapChainId, 10)
		}
		return s.SuccessProof(req.SrcChain, desChain, req.SrcBlockNumber, req.SrcLogIndex)
	case constant.StatusOfRelayFinish:
		return s.SuccessProof(req.RelayChain, req.DesChain, req.RelayBlockNumber, req.RelayLogIndex)
	default:
	}

	return nil, nil
}

func (s *ProofSrv) RouterExecSwap(butterHost, toChain, txHash, slippage, entrance string) (map[string]interface{}, error) {
	data, err := butter.ExecSwap(butterHost, fmt.Sprintf("toChainId=%s&txHash=%s&slippage=%s&entrance=%s", toChain, txHash, slippage, entrance))
	if err != nil {
		return nil, err
	}

	var desTo string
	for _, ele := range s.cfg.Chains {
		if ele.Id == toChain {
			desTo = ele.Mcs
			break
		}
	}

	resp := butter.ExecSwapResp{}
	err = json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Errno != 0 {
		return nil, fmt.Errorf("swap failed with errno: %s", resp.Message)
	}

	return map[string]interface{}{
		"userRouter": true,
		"exec_chain": toChain,
		"exec_to":    desTo,
		"exec_data":  "0x",
		"exec_desc":  "failed tx retry exec",
		"exec_route": resp,
	}, nil
}

func (s *ProofSrv) RouterRetryMessageIn(butterHost, toChain, txHash string) (map[string]interface{}, error) {
	data, err := butter.RetryMessageIn(butterHost, fmt.Sprintf("txHash=%s", txHash))
	if err != nil {
		return nil, err
	}

	var desTo string
	for _, ele := range s.cfg.Chains {
		if ele.Id == toChain {
			desTo = ele.Mcs
			break
		}
	}

	resp := butter.RetryMessageInData{}
	err = json.Unmarshal(data, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Errno != 0 {
		return nil, fmt.Errorf("swap failed with errno: %s", resp.Message)
	}

	return map[string]interface{}{
		"userRouter": true,
		"exec_relay": true,
		"exec_chain": toChain,
		"exec_to":    desTo,
		"exec_data":  "0x",
		"exec_desc":  "exec failed relay tx retry",
		"exec_route": resp,
	}, nil
}

func (s *ProofSrv) SuccessProof(srcChain, desChain string, srcBlockNumber int64, logIndex uint) (map[string]interface{}, error) {
	var (
		err                                                                          error
		proofType                                                                    = int64(0)
		src, des                                                                     chains.Proffer
		srcClient                                                                    *ethclient.Client
		srcEndpoint, srcOracleNode, srcMcs, srcLightNode, desTo, desLight, desOracle string
		srcConfig                                                                    expose.RawChainConfig
	)
	srcChainId, err := strconv.ParseUint(srcChain, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid source chain ID %q: %w", srcChain, err)
	}
	desChainId, err := strconv.ParseUint(desChain, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid destination chain ID %q: %w", desChain, err)
	}
	for _, ele := range s.cfg.Chains {
		if ele.Id == srcChain {
			creator, ok := chains.CreateProffer(ele.Type)
			if !ok {
				return nil, fmt.Errorf("source chain %s has unsupported proof type %q", srcChain, ele.Type)
			}
			src = creator
			srcConfig = ele
			srcEndpoint = ele.Endpoint
			srcOracleNode = ele.OracleNode
			srcMcs = ele.Mcs
			srcLightNode = ele.LightNode
			srcClient, err = src.Connect(srcChain, srcEndpoint, srcMcs, srcLightNode, srcOracleNode)
			if err != nil {
				return nil, err
			}
		}
		if ele.Id == desChain {
			creator, ok := chains.CreateProffer(ele.Type)
			if !ok {
				return nil, fmt.Errorf("destination chain %s has unsupported proof type %q", desChain, ele.Type)
			}
			des = creator
			desTo = ele.Mcs
			desOracle = ele.OracleNode
			desLight = ele.LightNode
			_, err = des.Connect(desChain, ele.Endpoint, ele.Mcs, ele.LightNode, ele.OracleNode)
			if err != nil {
				return nil, err
			}
			if ele.Name == constant.Tron || ele.Name == constant.Ton || ele.Name == constant.Solana {
				proofType = constant.ProofTypeOfLogOracle
			}
		}
	}
	if src == nil {
		return nil, errors.New("srcChain unrecognized Chain Type")
	}

	if des == nil {
		return nil, errors.New("desChain unrecognized Chain Type")
	}

	// get log
	query, err := buildProofLogQuery(srcConfig, srcBlockNumber)
	if err != nil {
		return nil, err
	}
	logs, err := srcClient.FilterLogs(context.Background(), query)
	if err != nil {
		return nil, err
	}
	targetLog, err := findLogByIndex(logs, srcBlockNumber, logIndex)
	if err != nil {
		return nil, err
	}
	orderId, err := validateProofLog(targetLog, srcConfig, srcChainId, desChainId, srcBlockNumber, logIndex)
	if err != nil {
		return nil, err
	}
	if proofType == 0 {
		proofType, err = chain.PreSendTx(0, srcChainId, desChainId, big.NewInt(srcBlockNumber), orderId.Bytes())
		if errors.Is(err, chain.NotVerifyAble) { // maintainer
			updateHeader, err := src.Maintainer(srcClient, srcChainId, desChainId, srcEndpoint)
			if err != nil {
				return nil, errors.Wrap(err, "Assemble maintainer failed")
			}
			return map[string]interface{}{
				"userRouter": false,
				"exec_chain": desChain,
				"exec_to":    desLight,
				"exec_data":  "0x" + common.Bytes2Hex(updateHeader),
				"exec_desc":  "Execute maintainer transaction",
				"exec_route": struct{}{},
			}, nil
		}
		if err != nil {
			return nil, err
		}
	}
	var sign [][]byte
	if proofType == constant.ProofTypeOfNewOracle || proofType == constant.ProofTypeOfLogOracle {
		ret, err := chain.Signer(srcClient, srcChainId, constant.MapChainId, &targetLog, proofType)
		if errors.Is(err, chain.NotVerifyAble) {
			oracle, err := chain.ExternalOracleInput(int64(srcChainId), proofType, &targetLog, srcClient, s.pri) // private
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"userRouter": false,
				"exec_chain": desChain,
				"exec_to":    desOracle,
				"exec_data":  "0x" + common.Bytes2Hex(oracle),
				"exec_desc":  "Execute oracle transaction",
				"exec_route": struct{}{},
			}, nil
		}
		if err != nil {
			return nil, err
		}
		sign = ret.Signatures
	}
	// proof
	ret, err := src.Proof(srcClient, &targetLog, srcEndpoint, proofType, srcChainId, desChainId, sign)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"userRouter": false,
		"exec_chain": desChain,
		"exec_to":    desTo,
		"exec_data":  "0x" + common.Bytes2Hex(ret),
		"exec_desc":  "Execute mos transaction",
		"exec_route": struct{}{},
	}, nil
}

func findLogByIndex(logs []types.Log, blockNumber int64, logIndex uint) (types.Log, error) {
	for i := range logs {
		if logs[i].Index == logIndex {
			return logs[i], nil
		}
	}
	return types.Log{}, fmt.Errorf("log not found, block(%d), logIndex(%d)", blockNumber, logIndex)
}

func buildProofLogQuery(cfg expose.RawChainConfig, blockNumber int64) (ethereum.FilterQuery, error) {
	if blockNumber < 0 {
		return ethereum.FilterQuery{}, fmt.Errorf("invalid proof block number %d", blockNumber)
	}
	if !common.IsHexAddress(cfg.Mcs) {
		return ethereum.FilterQuery{}, fmt.Errorf("invalid MCS address %q for chain %s", cfg.Mcs, cfg.Id)
	}
	mcs := common.HexToAddress(cfg.Mcs)
	if mcs == (common.Address{}) {
		return ethereum.FilterQuery{}, fmt.Errorf("MCS address is zero for chain %s", cfg.Id)
	}
	topics, err := proofEventTopics(cfg.Event)
	if err != nil {
		return ethereum.FilterQuery{}, fmt.Errorf("invalid event whitelist for chain %s: %w", cfg.Id, err)
	}
	block := big.NewInt(blockNumber)
	return ethereum.FilterQuery{
		FromBlock: block,
		ToBlock:   new(big.Int).Set(block),
		Addresses: []common.Address{mcs},
		Topics:    [][]common.Hash{topics},
	}, nil
}

func proofEventTopics(configured string) ([]common.Hash, error) {
	if strings.TrimSpace(configured) == "" {
		return nil, errors.New("event whitelist is empty")
	}

	values := strings.Split(configured, "|")
	topics := make([]common.Hash, 0, len(values))
	seen := make(map[common.Hash]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("event whitelist contains an empty entry")
		}

		var topic common.Hash
		rawHash := strings.TrimPrefix(value, "0x")
		if len(rawHash) == common.HashLength*2 {
			decoded, err := hex.DecodeString(rawHash)
			if err != nil {
				return nil, fmt.Errorf("invalid event topic %q: %w", value, err)
			}
			topic = common.BytesToHash(decoded)
		} else {
			if !strings.Contains(value, "(") || !strings.HasSuffix(value, ")") {
				return nil, fmt.Errorf("invalid event signature %q", value)
			}
			topic = constant.EventSig(value).GetTopic()
		}
		if topic == (common.Hash{}) {
			return nil, errors.New("event whitelist contains a zero topic")
		}

		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		topics = append(topics, topic)
	}
	return topics, nil
}

func validateProofLog(log types.Log, cfg expose.RawChainConfig, srcChainID, desChainID uint64,
	blockNumber int64, logIndex uint) (common.Hash, error) {
	query, err := buildProofLogQuery(cfg, blockNumber)
	if err != nil {
		return common.Hash{}, err
	}
	if log.BlockNumber != uint64(blockNumber) {
		return common.Hash{}, fmt.Errorf("proof log block %d does not match requested block %d", log.BlockNumber, blockNumber)
	}
	if log.Index != logIndex {
		return common.Hash{}, fmt.Errorf("proof log index %d does not match requested index %d", log.Index, logIndex)
	}
	if log.Address != query.Addresses[0] {
		return common.Hash{}, fmt.Errorf("proof log address %s does not match configured MCS %s", log.Address, query.Addresses[0])
	}
	if len(log.Topics) < 2 {
		return common.Hash{}, fmt.Errorf("proof log topics length %d is insufficient", len(log.Topics))
	}

	allowedEvent := false
	for _, topic := range query.Topics[0] {
		if log.Topics[0] == topic {
			allowedEvent = true
			break
		}
	}
	if !allowedEvent {
		return common.Hash{}, fmt.Errorf("proof log event %s is not allowed", log.Topics[0])
	}

	orderID := log.Topics[1]
	if orderID == (common.Hash{}) {
		return common.Hash{}, errors.New("proof log order ID is zero")
	}
	if srcChainID == constant.MapChainId {
		if len(log.Topics) < 3 {
			return common.Hash{}, fmt.Errorf("proof log target chain topic is missing")
		}
		loggedDestination := binary.BigEndian.Uint64(log.Topics[2][8:16])
		if loggedDestination != desChainID {
			return common.Hash{}, fmt.Errorf("proof log target chain %d does not match requested destination %d", loggedDestination, desChainID)
		}
	} else if desChainID != constant.MapChainId {
		return common.Hash{}, fmt.Errorf("non-MAP source chain %d must route through MAP chain %d", srcChainID, constant.MapChainId)
	}
	return orderID, nil
}
