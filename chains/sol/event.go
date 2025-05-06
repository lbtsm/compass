package sol

import (
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	EventMessageRelay = "MessageRelay"
)

var (
	messageRelayABI, _ = abi.JSON(strings.NewReader(`[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"bytes32","name":"orderId","type":"bytes32"},{"indexed":true,"internalType":"uint256","name":"chainAndGasLimit","type":"uint256"},{"indexed":false,"internalType":"bytes","name":"payload","type":"bytes"}],"name":"MessageRelay","type":"event"}]`))
)

type Swap struct {
	ToToken   []byte
	Receiver  []byte
	MinAmount *big.Int
}

type Relay struct {
	Version     *big.Int
	MessageType *big.Int
	OrderId     [32]byte // MessageRelay.OrderId
	SrcChain    *big.Int // MessageRelay.ChainAndGasLimit 0:8
	Sender      string   // MessageRelayPayload.From
	DstChain    *big.Int // MessageRelay.ChainAndGasLimit 8:16
	DstToken    []byte   // MessageRelayPayload.TokenAddress
	OutAmount   *big.Int // MessageRelayPayload.TokenAmount
	Receiver    []byte   // MessageRelayPayload.To
	Payload     []byte
	Swap        *Swap
}

type Contract struct {
	abi abi.ABI
}

func NewContract(abi abi.ABI) *Contract {
	return &Contract{abi: abi}
}

func (c *Contract) UnpackLog(ret interface{}, event string, log types.Log) error {
	if log.Topics[0] != c.abi.Events[event].ID {
		return fmt.Errorf("event signature mismatch")
	}
	if len(log.Data) > 0 {
		if err := c.abi.UnpackIntoInterface(ret, event, log.Data); err != nil {
			return err
		}
	}
	var indexed abi.Arguments
	for _, arg := range c.abi.Events[event].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	return abi.ParseTopics(ret, indexed, log.Topics[1:])
}

type MessageRelay struct {
	OrderId          [32]byte
	ChainAndGasLimit *big.Int // fromChain (8 bytes) | toChain (8 bytes) | reserved (8 bytes) | gasLimit (8 bytes)
	Payload          []byte   // MessageRelayPayload
}

func UnpackMessageRelay(log types.Log) (*MessageRelay, error) {
	ret := &MessageRelay{}
	if err := NewContract(messageRelayABI).UnpackLog(ret, EventMessageRelay, log); err != nil {
		return nil, err
	}
	return ret, nil
}

func DecodeMessageRelay(topics []common.Hash, data string) (*Relay, error) {
	bytesData, err := hex.DecodeString(data)
	if err != nil {
		return nil, err
	}
	log := types.Log{
		Topics: topics,
		Data:   bytesData,
	}
	messageRelay, err := UnpackMessageRelay(log)
	if err != nil {
		return nil, err
	}

	payload := messageRelay.Payload
	// Helper function to parse a big.Int from a substring
	parseBigInt := func(start, length int) *big.Int {
		substr := payload[start : start+length]
		return new(big.Int).SetBytes(substr)
	}

	version := parseBigInt(0, 1)
	//ret.Version = version
	messageType := parseBigInt(1, 1)
	//ret.MessageType = messageType
	// Extract values based on offsets
	tokenLen := parseBigInt(2, 1)
	mosLen := parseBigInt(3, 1)
	fromLen := parseBigInt(4, 1)
	toLen := parseBigInt(5, 1)
	payloadLen := parseBigInt(6, 2)
	tokenAmount := parseBigInt(16, 16)

	// Calculate dynamic offsets
	// token
	start := 32
	end := start + int(tokenLen.Int64())
	tokenAddress := payload[start:end]

	// mos
	start = end
	end = start + int(mosLen.Int64())

	// from
	start = end
	end = start + int(fromLen.Int64())
	from := common.BytesToAddress(payload[start:end])

	// to
	start = end
	end = start + int(toLen.Int64())
	to := payload[start:end]

	start = end
	end = start + int(payloadLen.Int64())
	swapData := payload[start:end]

	b := common.LeftPadBytes(messageRelay.ChainAndGasLimit.Bytes(), 32)
	ret := &Relay{
		Version:     version,
		MessageType: messageType,
		OrderId:     messageRelay.OrderId,
		SrcChain:    new(big.Int).SetBytes(b[:8]),
		Sender:      from.Hex(),
		DstChain:    new(big.Int).SetBytes(b[8:16]),
		DstToken:    tokenAddress,
		OutAmount:   tokenAmount,
		Receiver:    to,
		Payload:     swapData,
	}
	if len(swapData) != 0 {
		s := Swap{
			ToToken:   swapData[2:34],
			Receiver:  swapData[34:66],
			MinAmount: big.NewInt(0).SetBytes(swapData[66:98]),
		}
		ret.Swap = &s
	}

	return ret, nil
}
