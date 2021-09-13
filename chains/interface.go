package chains

import (
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mapprotocol/compass/types"
	"math/big"
	"time"
)

type ChainInterface interface {
	GetName() string
	GetClient() *ethclient.Client
	GetChainId() types.ChainId
	GetBlockNumber() uint64
	GetRpcUrl() string
	GetBlockHeader(num uint64, limit uint64) (*[]byte, error)
	GetAddress() string
	SetTarget(keystoreStr string, password string)
	GetPrivateKey() *ecdsa.PrivateKey
	Save(from types.ChainId, Cdata *[]byte)
	NumberOfSecondsOfBlockCreationTime() time.Duration
	GetStableBlockBeforeHeader() uint64
	ContractInterface
}
type ChainImplBase struct {
	Name                               string
	ChainId                            types.ChainId
	RpcUrl                             string
	NumberOfSecondsOfBlockCreationTime time.Duration
	StableBlockBeforeHeader            uint64
}
type ContractInterface interface {
	Register(value *big.Int) bool
	UnRegister(value *big.Int) bool
	GetRelayerBalance() types.GetRelayerBalanceResponse
	GetRelayer() types.GetRelayerResponse
	GetPeriodHeight() types.GetPeriodHeightResponse
}

// relayContractAddressStr is empty,it cannot be target,
