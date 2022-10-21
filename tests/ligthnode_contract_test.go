package tests

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/mapprotocol/compass/pkg/redis"

	"github.com/mapprotocol/compass/internal/near"
	nearclient "github.com/mapprotocol/near-api-go/pkg/client"
	"github.com/mapprotocol/near-api-go/pkg/client/block"

	"github.com/ethereum/go-ethereum"
	eth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	maptypes "github.com/mapprotocol/atlas/core/types"
	"github.com/mapprotocol/compass/mapprotocol"
	"github.com/mapprotocol/compass/pkg/ethclient"
	utils "github.com/mapprotocol/compass/shared/ethereum"
)

func Test_Key(t *testing.T) {
	var key []byte
	key = rlp.AppendUint64(key[:0], uint64(0))
	fmt.Println("index=0,length=3,hex=", "0x"+common.Bytes2Hex(key2Hex(key, 3)))
	fmt.Println("index=0,length=1,hex=", "0x"+common.Bytes2Hex(key2Hex(key, 1)))
	var key1 []byte
	key1 = rlp.AppendUint64(key1[:0], uint64(190))
	fmt.Println("index=190,length=5,hex=", "0x"+common.Bytes2Hex(key2Hex(key1, 5)))
}

func Test_Redis(t *testing.T) {
	//fmt.Println("0x" + common.Bytes2Hex([]byte("mcs_token_0")))
	//fmt.Println("0x" + common.Bytes2Hex([]byte("zmmap.testnet")))
	redis.Init("redis://:F6U3gV0L6Xwyw1Ko@46.137.199.126:6379/0")
	bytes, err := ioutil.ReadFile("./json.txt")
	if err != nil {
		t.Fatalf("readFile failed err is %v", err)
	}
	redis.GetClient().LPush(context.Background(), redis.ListKey, string(bytes))
}

func Test_NearMcs(t *testing.T) {
	bytes, err := ioutil.ReadFile("./hashjson.txt")
	if err != nil {
		t.Fatalf("readFile failed err is %v", err)
	}
	data := mapprotocol.StreamerMessage{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		t.Fatalf("unMarshal failed, err %v", err)
	}
	target := make([]mapprotocol.IndexerExecutionOutcomeWithReceipt, 0)
	for _, shard := range data.Shards {
		for _, outcome := range shard.ReceiptExecutionOutcomes {
			if outcome.ExecutionOutcome.Outcome.ExecutorID != "mcs.pandarr.testnet" { // 合约地址
				continue
			}
			if len(outcome.ExecutionOutcome.Outcome.Logs) == 0 {
				continue
			}
			for _, ls := range outcome.ExecutionOutcome.Outcome.Logs {
				//splits := strings.Split(ls, ":")
				//if len(splits) != 2 {
				//	continue
				//}
				//if !ExistInSlice(splits[0], mapprotocol.NearEventType) {
				//	continue
				//}
				//t.Logf("log is %v", ls)
				if !strings.HasPrefix(ls, mapprotocol.HashOfTransferOut) && !strings.HasPrefix(ls, mapprotocol.HashOfDepositOut) {
					continue
				}
			}

			target = append(target, outcome)
		}
	}

	if len(target) == 0 {
		t.Logf("data is %+v", data)
		return
	}

	cli, err := nearclient.NewClient("https://archival-rpc.testnet.near.org")
	if err != nil {
		t.Fatalf("unMarshal failed, err %v", err)
	}
	for _, tg := range target {
		// get fromChainId and toChainId
		logs := strings.SplitN(tg.ExecutionOutcome.Outcome.Logs[0], ":", 2)
		t.Logf("tg %+v ", logs)
		t.Logf("tg %+v ", logs[1])
		out := near.TransferOut{}
		err = json.Unmarshal([]byte(logs[1]), &out)
		if err != nil {
			t.Fatalf("logs format failed %v", err)
		}
		fmt.Println("out.to", out.ToChain)
		fmt.Println("out.from", out.FromChain)
		blk, err := cli.NextLightClientBlock(context.Background(), tg.ExecutionOutcome.BlockHash)
		if err != nil {
			t.Errorf("NextLightClientBlock failed, err %v", err)
		}

		clientHead, err := cli.BlockDetails(context.Background(), block.BlockID(blk.InnerLite.Height))
		if err != nil {
			t.Errorf("BlockDetails failed, err %v", err)
		}

		fmt.Printf("clientHead hash is %v \n", clientHead.Header.Hash)

		proof, err := cli.LightClientProof(context.Background(), nearclient.Receipt{
			ReceiptID:       tg.ExecutionOutcome.ID,
			ReceiverID:      tg.Receipt.ReceiverID,
			LightClientHead: clientHead.Header.Hash,
		})
		if err != nil {
			t.Errorf("LightClientProof failed, err %v", err)
		}

		d, _ := json.Marshal(blk)
		p, _ := json.Marshal(proof)
		t.Logf("block %+v, \n proof %+v \n", string(d), string(p))

		blkBytes := near.Borshify(blk)
		t.Logf("blockBytes, 0x%v", common.Bytes2Hex(blkBytes))
		proofBytes, err := near.BorshifyOutcomeProof(proof)
		if err != nil {
			t.Fatalf("borshifyOutcomeProof failed, err is %v", proofBytes)
		}
		t.Logf("proofBytes, 0x%v", common.Bytes2Hex(proofBytes))

		all, err := mapprotocol.Near.Methods["getBytes"].Inputs.Pack(blkBytes, proofBytes)
		if err != nil {
			t.Fatalf("getBytes failed, err is %v", err)
		}

		input, err := mapprotocol.Near.Pack(mapprotocol.MethodVerifyProofData, all)
		if err != nil {
			t.Fatalf("getBytes failed, err is %v", err)
		}

		fmt.Println("请求参数 ---------- ", "0x"+common.Bytes2Hex(all))
		fmt.Println("请求参数 ---------- input ", "0x"+common.Bytes2Hex(input))
		err = call(input, mapprotocol.Near, mapprotocol.MethodVerifyProofData)
		if err != nil {
			t.Fatalf("call failed, err is %v", err)
		}
	}
	//t.Logf("data is %+v", data)
}

func call(input []byte, useAbi abi.ABI, method string) error {
	to := common.HexToAddress("0x0000000000747856657269667941646472657373")
	count := 0
	for {
		count++
		if count == 5 {
			return errors.New("retry is full")
		}
		outPut, err := dialMapConn().CallContract(context.Background(),
			ethereum.CallMsg{
				From: common.HexToAddress("0xE0DC8D7f134d0A79019BEF9C2fd4b2013a64fCD6"),
				To:   &to,
				Data: input,
			},
			nil,
		)
		if err != nil {
			log.Printf("callContract failed, err is %v \n", err)
			if strings.Index(err.Error(), "read: connection reset by peer") != -1 || strings.Index(err.Error(), "EOF") != -1 {
				log.Printf("err is : %s, will retry \n", err)
				time.Sleep(time.Second * 5)
				continue
			}
			return err
		}

		resp, err := useAbi.Methods[method].Outputs.Unpack(outPut)
		if err != nil {
			return err
		}

		ret := struct {
			Success bool
			Message string
			Logs    []byte
		}{}

		err = useAbi.Methods[method].Outputs.Copy(&ret, resp)
		if err != nil {
			return err
		}
		if !ret.Success {
			return fmt.Errorf("verify proof failed, message is (%s)", ret.Message)
		}
		if ret.Success == true {
			log.Println("mcs verify log success", "success", ret.Success)
			log.Println("mcs verify log success", "logs", "0x"+common.Bytes2Hex(ret.Logs))
			return nil
		}
	}
}

func ExistInSlice(target string, dst []string) bool {
	for _, d := range dst {
		if target == d {
			return true
		}
	}

	return false
}

var ContractAddr = common.HexToAddress("0x0000000000747856657269667941646472657373")

func dialConn() *ethclient.Client {
	conn, err := ethclient.Dial("http://18.142.54.137:7445")
	if err != nil {
		log.Fatalf("Failed to connect to the atlas: %v", err)
	}
	return conn
}

func dialMapConn() *ethclient.Client {
	conn, err := ethclient.Dial("http://18.142.54.137:7445")
	if err != nil {
		log.Fatalf("Failed to connect to the atlas: %v", err)
	}
	return conn
}

func TestLoadPrivate(t *testing.T) {
	path := "/Users/t/data/atlas-1/keystore/UTC--2022-06-07T04-22-55.836701000Z--f9803e9021e56e68662351fe43773934c4a276b8"
	password := ""
	addr, private := LoadPrivate(path, password)
	fmt.Println("============================== addr: ", addr)
	fmt.Printf("============================== private key: %x\n", crypto.FromECDSA(private))
}
func TestUpdateHeader(t *testing.T) {
	cli := dialConn()
	for i := 1; i < 21; i++ {
		number := int64(i * 1000)
		fmt.Println("============================== number: ", number)
		header, err := cli.MAPHeaderByNumber(context.Background(), big.NewInt(number))
		if err != nil {
			t.Fatalf(err.Error())
		}

		h := mapprotocol.ConvertHeader(header)
		aggPK, err := mapprotocol.GetAggPK(cli, header.Number, header.Extra)
		if err != nil {
			t.Fatalf(err.Error())
		}

		//printHeader(header)
		//printAggPK(aggPK)
		//_ = h

		input, err := mapprotocol.PackInput(mapprotocol.LightManger, mapprotocol.MethodUpdateBlockHeader, h, aggPK)
		if err != nil {
			t.Fatalf(err.Error())
		}

		path := "/Users/xm/Desktop/WL/code/atlas/node-1/keystore/UTC--2022-06-15T07-51-25.301943000Z--e0dc8d7f134d0a79019bef9c2fd4b2013a64fcd6"
		password := "1234"
		from, private := LoadPrivate(path, password)
		if err := SendContractTransaction(cli, from, ContractAddr, nil, private, input); err != nil {
			t.Fatalf(err.Error())
		}
	}
}

func TestVerifyProof(t *testing.T) {
	tstr := "0xf9094e944c359fb750a5fda0a64377b7fb1d071c98d6fa0f944c359fb750a5fda0a64377b7fb1d071c98d6fa0f82868281d4b9091cf90919f904810182d231b90100200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008220000000000000000000000000008000000000000000000000000080000040000000000000000020000000000000000000801000000000000000000000010000000000008000080000000000000000000000000000000000000008000000000000000020000000020000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000040000000000000000020000010800000000002000000020000000040000000000000000000000000000000f90377f89b940f256fa34e55f33c6a3a0c7baa383925a8f04704f863a08c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925a0000000000000000000000000dce4fe8edde09acceada2b9cd38f72f149b06f7fa00000000000000000000000004c359fb750a5fda0a64377b7fb1d071c98d6fa0fa00000000000000000000000000000000000000001431e0f9e2a1bfd4276d00000f89b940f256fa34e55f33c6a3a0c7baa383925a8f04704f863a0ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3efa0000000000000000000000000dce4fe8edde09acceada2b9cd38f72f149b06f7fa00000000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000001043561a8829300000f9023a944c359fb750a5fda0a64377b7fb1d071c98d6fa0fe1a0aca0a1067548270e80c1209ec69b5381d80bdaf345ad70cf7f00af9c6ed3f9b4b9020000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000140fa22614b27c6d41c83f0c77683472b911eccb8350b51a26ac2b32b1c97d1d761000000000000000000000000000000000000000000000000000000000000868200000000000000000000000000000000000000000000000000000000000000d4000000000000000000000000000000000000000000000000000000000000018000000000000000000000000000000000000000000000001043561a882930000000000000000000000000000000000000000000000000000000000000000001c000000000000000000000000000000000000000000000000000000000000000140f256fa34e55f33c6a3a0c7baa383925a8f047040000000000000000000000000000000000000000000000000000000000000000000000000000000000000014dce4fe8edde09acceada2b9cd38f72f149b06f7f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000142e784874ddb32cd7975d68565b509412a5b519f400000000000000000000000000000000000000000000000000000000000000000000000000000000000000140000000000000000000000000000000000000000000000000000000000000000f9048df9048a822080b90484f904810182d231b9010020000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008220000000000000000000000000008000000000000000000000000080000040000000000000000020000000000000000000801000000000000000000000010000000000008000080000000000000000000000000000000000000008000000000000000020000000020000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000040000000000000000020000010800000000002000000020000000040000000000000000000000000000000f90377f89b940f256fa34e55f33c6a3a0c7baa383925a8f04704f863a08c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925a0000000000000000000000000dce4fe8edde09acceada2b9cd38f72f149b06f7fa00000000000000000000000004c359fb750a5fda0a64377b7fb1d071c98d6fa0fa00000000000000000000000000000000000000001431e0f9e2a1bfd4276d00000f89b940f256fa34e55f33c6a3a0c7baa383925a8f04704f863a0ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3efa0000000000000000000000000dce4fe8edde09acceada2b9cd38f72f149b06f7fa00000000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000001043561a8829300000f9023a944c359fb750a5fda0a64377b7fb1d071c98d6fa0fe1a0aca0a1067548270e80c1209ec69b5381d80bdaf345ad70cf7f00af9c6ed3f9b4b9020000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000140fa22614b27c6d41c83f0c77683472b911eccb8350b51a26ac2b32b1c97d1d761000000000000000000000000000000000000000000000000000000000000868200000000000000000000000000000000000000000000000000000000000000d4000000000000000000000000000000000000000000000000000000000000018000000000000000000000000000000000000000000000001043561a882930000000000000000000000000000000000000000000000000000000000000000001c000000000000000000000000000000000000000000000000000000000000000140f256fa34e55f33c6a3a0c7baa383925a8f047040000000000000000000000000000000000000000000000000000000000000000000000000000000000000014dce4fe8edde09acceada2b9cd38f72f149b06f7f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000142e784874ddb32cd7975d68565b509412a5b519f4000000000000000000000000000000000000000000000000000000000000000000000000000000000000001400000000000000000000000000000000000000000000000000000000000000008305d4f580"
	payloads := common.Hex2Bytes(tstr)
	input, err := mapprotocol.PackInput(mapprotocol.Near, mapprotocol.MethodVerifyProofData, payloads)
	if err != nil {
		t.Fatalf("pack input failed, err is %v", err)
	}
	err = call(input, mapprotocol.Near, mapprotocol.MethodVerifyProofData)
	if err != nil {
		t.Fatalf("verify proof failed, err is %v", err)
	}
}

func TestGetLog(t *testing.T) {
	//number       = big.NewInt(4108)
	//number       = big.NewInt(55342)
	var MapTransferOut utils.EventSig = "mapTransferOut(bytes,bytes,bytes32,uint256,uint256,bytes,uint256,bytes)"
	query := buildQuery(common.HexToAddress("0xf03aDB732FBa8Fca38C00253B1A1aa72CCA026E6"),
		MapTransferOut, big.NewInt(106020), big.NewInt(106020))

	// querying for logs
	logs, err := dialConn().FilterLogs(context.Background(), query)
	if err != nil {
		t.Fatalf("unable to Filter Logs: %s", err)
	}
	t.Logf("log len is %v", len(logs))
}

// buildQuery constructs a query for the bridgeContract by hashing sig to get the event topic
func buildQuery(contract common.Address, sig utils.EventSig, startBlock *big.Int, endBlock *big.Int) eth.FilterQuery {
	query := eth.FilterQuery{
		FromBlock: startBlock,
		ToBlock:   endBlock,
		Addresses: []common.Address{contract},
		Topics: [][]common.Hash{
			{sig.GetTopic()},
		},
	}
	return query
}

func SendContractTransaction(client *ethclient.Client, from, toAddress common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, input []byte) error {
	// Ensure a valid value field and resolve the account nonce
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		return err
	}
	//fmt.Printf("============================== from: %s, nonce: %d\n", from, nonce)

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}

	gasLimit := uint64(2100000) // in units
	// If the contract surely has code (or code is not needed), estimate the transaction
	msg := eth.CallMsg{From: from, To: &toAddress, GasPrice: gasPrice, Value: value, Data: input}
	gasLimit, err = client.EstimateGas(context.Background(), msg)
	if err != nil {
		return fmt.Errorf("contract exec failed, %s", err.Error())
	}
	if gasLimit < 1 {
		gasLimit = 866328
	}

	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, input)

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return err
	}
	//fmt.Println("TX data nonce ", nonce, " transfer value ", value, " gasLimit ", gasLimit, " gasPrice ", gasPrice, " chainID ", chainID)
	signer := types.LatestSignerForChainID(chainID)
	signedTx, err := types.SignTx(tx, signer, privateKey)
	if err != nil {
		return err
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return err
	}
	txHash := signedTx.Hash()
	count := 0
	for {
		time.Sleep(time.Millisecond * 1000)
		_, isPending, err := client.TransactionByHash(context.Background(), txHash)
		if err != nil {
			return err
		}
		count++
		if !isPending {
			break
		} else {
			log.Println("======================== pending...")
		}
	}
	receipt, err := client.TransactionReceipt(context.Background(), txHash)
	if err != nil {
		return err
	}
	if receipt.Status == types.ReceiptStatusSuccessful {
		logs, _ := json.Marshal(receipt.Logs)
		log.Printf("Transaction Success, number: %v, hash: %v， logs: %v\n", receipt.BlockNumber.Uint64(), receipt.BlockHash, string(logs))
	} else if receipt.Status == types.ReceiptStatusFailed {
		log.Println("Transaction Failed. ", "block number: ", receipt.BlockNumber.Uint64())
		return errors.New("transaction failed")
	}
	return nil
}

func LoadPrivate(path, password string) (common.Address, *ecdsa.PrivateKey) {
	bs, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	key, err := keystore.DecryptKey(bs, password)
	if err != nil || key == nil {
		panic(fmt.Errorf("error decrypting key: %v", err))
	}
	priKey := key.PrivateKey
	addr := crypto.PubkeyToAddress(priKey.PublicKey)

	if priKey == nil {
		panic("load privateKey failed")
	}
	return addr, priKey
}

func printHeader(header *maptypes.Header) {
	type blockHeader struct {
		ParentHash  string
		Coinbase    string
		Root        string
		TxHash      string
		ReceiptHash string
		Bloom       string
		Number      *big.Int
		GasLimit    *big.Int
		GasUsed     *big.Int
		Time        *big.Int
		ExtraData   string
		MixDigest   string
		Nonce       string
		BaseFee     *big.Int
	}
	h := blockHeader{
		ParentHash:  "0x" + common.Bytes2Hex(header.ParentHash[:]),
		Coinbase:    header.Coinbase.String(),
		Root:        "0x" + common.Bytes2Hex(header.Root[:]),
		TxHash:      "0x" + common.Bytes2Hex(header.TxHash[:]),
		ReceiptHash: "0x" + common.Bytes2Hex(header.ReceiptHash[:]),
		Bloom:       "0x" + common.Bytes2Hex(header.Bloom[:]),
		Number:      header.Number,
		GasLimit:    new(big.Int).SetUint64(header.GasLimit),
		GasUsed:     new(big.Int).SetUint64(header.GasUsed),
		Time:        new(big.Int).SetUint64(header.Time),
		ExtraData:   "0x" + common.Bytes2Hex(header.Extra),
		MixDigest:   "0x" + common.Bytes2Hex(header.MixDigest[:]),
		Nonce:       "0x" + common.Bytes2Hex(header.Nonce[:]),
		BaseFee:     header.BaseFee,
	}
	fmt.Printf("============================== header: %+v\n", h)
}

func printAggPK(aggPk *mapprotocol.G2) {
	type G2Str struct {
		xr string
		xi string
		yr string
		yi string
	}
	g2 := G2Str{
		xr: "0x" + common.Bytes2Hex(aggPk.Xr.Bytes()),
		xi: "0x" + common.Bytes2Hex(aggPk.Xi.Bytes()),
		yr: "0x" + common.Bytes2Hex(aggPk.Yr.Bytes()),
		yi: "0x" + common.Bytes2Hex(aggPk.Yi.Bytes()),
	}
	fmt.Printf("============================== aggPk: %+v\n", g2)
}

func printReceipt(r *mapprotocol.TxReceipt) {
	type txLog struct {
		Addr   common.Address
		Topics []string
		Data   string
	}

	type receipt struct {
		ReceiptType       *big.Int
		PostStateOrStatus string
		CumulativeGasUsed *big.Int
		Bloom             string
		Logs              []txLog
	}

	logs := make([]txLog, 0, len(r.Logs))
	for _, lg := range r.Logs {
		topics := make([]string, 0, len(lg.Topics))
		for _, tp := range lg.Topics {
			topics = append(topics, "0x"+common.Bytes2Hex(tp))
		}
		logs = append(logs, txLog{
			Addr:   lg.Addr,
			Topics: topics,
			Data:   "0x" + common.Bytes2Hex(lg.Data),
		})
	}

	rr := receipt{
		ReceiptType:       r.ReceiptType,
		PostStateOrStatus: "0x" + common.Bytes2Hex(r.PostStateOrStatus),
		CumulativeGasUsed: r.CumulativeGasUsed,
		Bloom:             "0x" + common.Bytes2Hex(r.Bloom),
		Logs:              logs,
	}
	fmt.Printf("============================== Receipt: %+v\n", rr)
}

func printProof(proof [][]byte) {
	p := make([]string, 0, len(proof))
	for _, v := range proof {
		p = append(p, "0x"+common.Bytes2Hex(v))
	}
	fmt.Println("============================== proof: ", p)
}

func getProof(receipts []*types.Receipt, txIndex uint) ([][]byte, error) {
	tr, err := trie.New(common.Hash{}, trie.NewDatabase(memorydb.New()))
	if err != nil {
		return nil, err
	}

	tr = utils.DeriveTire(receipts, tr)
	ns := light.NewNodeSet()
	key, err := rlp.EncodeToBytes(txIndex)
	if err != nil {
		return nil, err
	}
	if err = tr.Prove(key, 0, ns); err != nil {
		return nil, err
	}

	proof := make([][]byte, 0, len(ns.NodeList()))
	for _, v := range ns.NodeList() {
		proof = append(proof, v)
	}
	return proof, nil
}

func getTransactionsHashByBlockNumber(conn *ethclient.Client, number *big.Int) ([]common.Hash, error) {
	block, err := conn.MAPBlockByNumber(context.Background(), number)
	if err != nil {
		return nil, err
	}

	txs := make([]common.Hash, 0, len(block.Transactions()))
	for _, tx := range block.Transactions() {
		txs = append(txs, tx.Hash())
	}
	return txs, nil
}

func getReceiptsByTxsHash(conn *ethclient.Client, txsHash []common.Hash) ([]*types.Receipt, error) {
	rs := make([]*types.Receipt, 0, len(txsHash))
	for _, h := range txsHash {
		r, err := conn.TransactionReceipt(context.Background(), h)
		if err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	return rs, nil
}

func keyBytesToHex(str []byte) []byte {
	l := len(str)*2 + 1
	var nibbles = make([]byte, l)
	for i, b := range str {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}
	//nibbles[l-1] = 16
	return nibbles
}

func key2Hex(str []byte, proofLength int) []byte {
	ret := make([]byte, 0)
	if len(ret)+1 == proofLength {
		ret = append(ret, str...)
	} else {
		for _, b := range str {
			ret = append(ret, b/16)
			ret = append(ret, b%16)
		}
	}
	return ret
}