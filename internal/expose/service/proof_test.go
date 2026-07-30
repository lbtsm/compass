package service

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mapprotocol/compass/internal/constant"
	"github.com/mapprotocol/compass/internal/expose"
)

const (
	testMCSAddress = "0x1111111111111111111111111111111111111111"
	testEvent      = "mapTransferOut(uint256,uint256,bytes32,bytes,bytes,bytes,uint256,bytes)"
	secondEvent    = "mapDepositOut(uint256,uint256,bytes32,address,bytes,address,uint256)"
)

func TestFindLogByIndexReturnsExactLog(t *testing.T) {
	logs := []types.Log{{Index: 3}, {Index: 7}}
	got, err := findLogByIndex(logs, 100, 7)
	if err != nil {
		t.Fatalf("findLogByIndex returned error: %v", err)
	}
	if got.Index != 7 {
		t.Fatalf("findLogByIndex returned index %d, want 7", got.Index)
	}
}

func TestFindLogByIndexRejectsMissingLog(t *testing.T) {
	_, err := findLogByIndex([]types.Log{{Index: 3}}, 100, 7)
	if err == nil {
		t.Fatal("findLogByIndex returned nil error for a missing log")
	}
	if !strings.Contains(err.Error(), "block(100)") || !strings.Contains(err.Error(), "logIndex(7)") {
		t.Fatalf("error lacks lookup context: %v", err)
	}
}

func TestBuildProofLogQueryRestrictsMCSAddressAndEventTopics(t *testing.T) {
	query, err := buildProofLogQuery(expose.RawChainConfig{
		Mcs:   testMCSAddress,
		Event: testEvent + "|" + secondEvent,
	}, 100)
	if err != nil {
		t.Fatalf("buildProofLogQuery returned error: %v", err)
	}

	if len(query.Addresses) != 1 || query.Addresses[0] != common.HexToAddress(testMCSAddress) {
		t.Fatalf("query addresses = %v, want only %s", query.Addresses, testMCSAddress)
	}
	if len(query.Topics) != 1 || len(query.Topics[0]) != 2 {
		t.Fatalf("query topics = %v, want one topic0 allowlist with two entries", query.Topics)
	}
	if query.Topics[0][0] != constant.EventSig(testEvent).GetTopic() ||
		query.Topics[0][1] != constant.EventSig(secondEvent).GetTopic() {
		t.Fatalf("query topic allowlist = %v, want configured event topics", query.Topics[0])
	}
}

func TestBuildProofLogQueryAcceptsRawEventTopic(t *testing.T) {
	want := constant.EventSig(testEvent).GetTopic()
	query, err := buildProofLogQuery(expose.RawChainConfig{
		Mcs:   testMCSAddress,
		Event: want.Hex(),
	}, 100)
	if err != nil {
		t.Fatalf("buildProofLogQuery returned error: %v", err)
	}
	if len(query.Topics) != 1 || len(query.Topics[0]) != 1 || query.Topics[0][0] != want {
		t.Fatalf("query topics = %v, want %s", query.Topics, want)
	}
}

func TestBuildProofLogQueryRejectsMissingWhitelistConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  expose.RawChainConfig
	}{
		{name: "missing MCS", cfg: expose.RawChainConfig{Event: testEvent}},
		{name: "invalid MCS", cfg: expose.RawChainConfig{Mcs: "not-an-address", Event: testEvent}},
		{name: "zero MCS", cfg: expose.RawChainConfig{Mcs: common.Address{}.Hex(), Event: testEvent}},
		{name: "missing events", cfg: expose.RawChainConfig{Mcs: testMCSAddress}},
		{name: "zero event topic", cfg: expose.RawChainConfig{Mcs: testMCSAddress, Event: common.Hash{}.Hex()}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := buildProofLogQuery(tt.cfg, 100); err == nil {
				t.Fatal("buildProofLogQuery returned nil error for invalid whitelist configuration")
			}
		})
	}
}

func TestValidateProofLogAcceptsConfiguredMapEvent(t *testing.T) {
	const destinationChainID uint64 = 1360108768460801
	orderID := common.HexToHash("0x1234")
	log := validProofLog(destinationChainID, orderID)

	got, err := validateProofLog(log, expose.RawChainConfig{Mcs: testMCSAddress, Event: testEvent},
		constant.MapChainId, destinationChainID, 100, 7)
	if err != nil {
		t.Fatalf("validateProofLog returned error: %v", err)
	}
	if got != orderID {
		t.Fatalf("validateProofLog returned order ID %s, want %s", got, orderID)
	}
}

func TestValidateProofLogRejectsLogsOutsideProofBoundary(t *testing.T) {
	const destinationChainID uint64 = 1360108768460801
	orderID := common.HexToHash("0x1234")
	valid := validProofLog(destinationChainID, orderID)
	cfg := expose.RawChainConfig{Mcs: testMCSAddress, Event: testEvent}

	tests := []struct {
		name       string
		log        types.Log
		srcChain   uint64
		desChain   uint64
		wantErrSub string
	}{
		{name: "wrong block", log: cloneProofLog(valid, func(log *types.Log) { log.BlockNumber = 101 }), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "block"},
		{name: "wrong index", log: cloneProofLog(valid, func(log *types.Log) { log.Index = 8 }), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "index"},
		{name: "wrong address", log: cloneProofLog(valid, func(log *types.Log) { log.Address = common.HexToAddress("0x2222222222222222222222222222222222222222") }), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "address"},
		{name: "missing topics", log: cloneProofLog(valid, func(log *types.Log) { log.Topics = nil }), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "topics"},
		{name: "wrong event", log: cloneProofLog(valid, func(log *types.Log) { log.Topics[0] = constant.EventSig(secondEvent).GetTopic() }), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "event"},
		{name: "zero order ID", log: cloneProofLog(valid, func(log *types.Log) { log.Topics[1] = common.Hash{} }), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "order ID"},
		{name: "missing target chain topic", log: cloneProofLog(valid, func(log *types.Log) { log.Topics = log.Topics[:2] }), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "target chain"},
		{name: "wrong target chain", log: validProofLog(destinationChainID+1, orderID), srcChain: constant.MapChainId, desChain: destinationChainID, wantErrSub: "target chain"},
		{name: "non-MAP source routed elsewhere", log: cloneProofLog(valid, func(log *types.Log) { log.Topics = log.Topics[:2] }), srcChain: 1, desChain: destinationChainID, wantErrSub: "MAP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateProofLog(tt.log, cfg, tt.srcChain, tt.desChain, 100, 7)
			if err == nil {
				t.Fatal("validateProofLog returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErrSub)
			}
		})
	}
}

func validProofLog(destinationChainID uint64, orderID common.Hash) types.Log {
	targetChainTopic := common.Hash{}
	binary.BigEndian.PutUint64(targetChainTopic[8:16], destinationChainID)
	return types.Log{
		Address:     common.HexToAddress(testMCSAddress),
		Topics:      []common.Hash{constant.EventSig(testEvent).GetTopic(), orderID, targetChainTopic},
		BlockNumber: 100,
		Index:       7,
	}
}

func cloneProofLog(log types.Log, mutate func(*types.Log)) types.Log {
	log.Topics = append([]common.Hash(nil), log.Topics...)
	mutate(&log)
	return log
}
