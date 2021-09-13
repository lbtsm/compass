package ethereum

import (
	"context"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	types2 "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	abi2 "github.com/mapprotocol/compass/abi"
	"github.com/mapprotocol/compass/chain_tools"
	"github.com/mapprotocol/compass/chains"
	"github.com/mapprotocol/compass/types"
	log "github.com/sirupsen/logrus"
	"math/big"
	"strings"
	"time"
)

type TypeEther struct {
	base                       chains.ChainImplBase
	client                     *ethclient.Client
	address                    common.Address    //if SetTarget is not called ,it's nil
	privateKey                 *ecdsa.PrivateKey //if SetTarget is not called ,it's nil
	relayerContractAddress     common.Address
	headerStoreContractAddress common.Address
}

func (t *TypeEther) GetClient() *ethclient.Client {
	return t.client
}

func (t *TypeEther) GetPrivateKey() *ecdsa.PrivateKey {
	return t.privateKey
}

func (t *TypeEther) GetStableBlockBeforeHeader() uint64 {
	return t.base.StableBlockBeforeHeader

}

func (t *TypeEther) NumberOfSecondsOfBlockCreationTime() time.Duration {
	return t.base.NumberOfSecondsOfBlockCreationTime
}

func (t *TypeEther) Save(from types.ChainId, data *[]byte) {
	var abiStaking, _ = abi.JSON(strings.NewReader(abi2.HeaderStoreContractAbi))
	input := chain_tools.PackInput(abiStaking, "save",
		big.NewInt(int64(from)),
		big.NewInt(int64(t.GetChainId())),
		data,
	)
	tx := chain_tools.SendContractTransactionWithoutOutputUnlessError(t.client, t.address, t.headerStoreContractAddress, nil, t.GetPrivateKey(), input)
	if tx == nil {
		log.Infoln("Save failed")
		return
	}
	log.Infoln("Save tx hash :", tx.Hash().String())
	chain_tools.WaitingForEndPending(t.client, tx.Hash(), 50)
}

func NewEthChain(name string, chainId types.ChainId, seconds int, rpcUrl string, stableBlockBeforeHeader uint64,
	relayerContractAddressStr string, headerStoreContractAddressStr string) *TypeEther {
	ret := TypeEther{
		base: chains.ChainImplBase{
			Name:                               name,
			ChainId:                            chainId,
			NumberOfSecondsOfBlockCreationTime: time.Duration(seconds) * time.Second,
			RpcUrl:                             rpcUrl,
			StableBlockBeforeHeader:            stableBlockBeforeHeader,
		},
		client:                     chain_tools.GetClientByUrl(rpcUrl),
		relayerContractAddress:     common.HexToAddress(relayerContractAddressStr),
		headerStoreContractAddress: common.HexToAddress(headerStoreContractAddressStr),
	}
	return &ret
}

func (t *TypeEther) GetAddress() string {
	return t.address.String()
}

func (t *TypeEther) SetTarget(keystoreStr string, password string) {
	if t.relayerContractAddress.String() == "0x0000000000000000000000000000000000000000" ||
		t.headerStoreContractAddress.String() == "0x0000000000000000000000000000000000000000" {
		log.Fatal(t.GetName(), " cannot be target, relayer_contract_address and header_store_contract_address are required for target.")
	}
	key, _ := chain_tools.LoadPrivateKey(keystoreStr, password)
	t.privateKey = key.PrivateKey
	t.address = crypto.PubkeyToAddress(key.PrivateKey.PublicKey)

}

func (t *TypeEther) GetName() string {
	return t.base.Name
}

func (t *TypeEther) GetRpcUrl() string {
	return t.base.RpcUrl
}

func (t *TypeEther) GetChainId() types.ChainId {
	return t.base.ChainId
}

func (t *TypeEther) GetBlockNumber() uint64 {
	num, err := t.client.BlockNumber(context.Background())
	if err == nil {
		return num
	}
	return 0
}

func (t *TypeEther) GetBlockHeader(num uint64, limit uint64) (*[]byte, error) {
	var arr = make([]types2.Header, 0)
	var i uint64
	for i = 0; i < limit; i++ {
		block, err := t.client.BlockByNumber(context.Background(), big.NewInt(int64(num+i)))
		if err != nil {
			return &[]byte{}, err
		}
		arr = append(arr, *block.Header())
	}

	data, _ := rlp.EncodeToBytes(arr)
	return &data, nil
}
