package libs

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/peterbourgon/diskv"
	"math/big"
	"os"
	filepath2 "path/filepath"
)

const (
	filepath = "sign.log"
)

var (
	SendTransactionValue   = big.NewInt(1000000000000000000)
	RpcUrl                 = BlockChainMap[ReadConfigWithCondition("selected_chain", "1", keyInBlockChainMap)].RpcUrl
	StakingContractAddress = BlockChainMap[ReadConfigWithCondition("selected_chain", "1", keyInBlockChainMap)].StakingContractAddress
	DataContractAddress    = BlockChainMap[ReadConfigWithCondition("selected_chain", "1", keyInBlockChainMap)].DataContractAddress

	SendTransactionGasLimit = uint64(21000)
	ToAddress               = common.HexToAddress("0x799E24dC6B48549BbD1Fc9fcCa4d72880d8c7a15")
	SignLogFile, _          = os.OpenFile(filepath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	RuntimeDirectory        = "runtime"
	ConfigDirectory         = filepath2.Join(RuntimeDirectory, "config")
	DiskCache               = diskv.New(diskv.Options{
		BasePath:     ConfigDirectory,
		CacheSizeMax: 1024 * 1024,
	})
	BlockChainMap = map[string]Chain{"1": {
		"https://rpc-mumbai.maticvigil.com/",
		common.HexToAddress("0x821dD65Dbeb1a9F4846ce5E0d74A22869Ef5755d"),
		common.HexToAddress("0x17c6b58499dF2E70882C2a2A8D22F2Decc6e8F98"),
	}}
)

func keyInBlockChainMap(key string) bool {
	_, ok := BlockChainMap[key]
	return ok
}

type Chain struct {
	RpcUrl                 string
	StakingContractAddress common.Address
	DataContractAddress    common.Address
}
