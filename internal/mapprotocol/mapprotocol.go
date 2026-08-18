// Copyright 2021 Compass Systems
// SPDX-License-Identifier: LGPL-3.0-only

package mapprotocol

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mapprotocol/compass/internal/constant"
	nearclient "github.com/mapprotocol/near-api-go/pkg/client"
	"math/big"

	"github.com/mapprotocol/near-api-go/pkg/client/block"

	"github.com/ethereum/go-ethereum"
	goeth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/compass/pkg/ethclient"
	"github.com/pkg/errors"
)

type GetHeight func() (*big.Int, error)
type GetVerifyRange func() (*big.Int, *big.Int, error)

func Map2EthHeight(fromUser string, lightNode common.Address, client *ethclient.Client) GetHeight {
	return func() (*big.Int, error) {
		from := common.HexToAddress(fromUser)
		input, err := PackInput(Height, MethodOfHeaderHeight)
		if err != nil {
			return nil, fmt.Errorf("pack lightNode headerHeight Input failed, err is %v", err.Error())
		}
		output, err := client.CallContract(context.Background(),
			ethereum.CallMsg{
				From: from,
				To:   &lightNode,
				Data: input,
			},
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("headerHeight callContract failed, err is %v", err.Error())
		}

		return UnpackHeaderHeightOutput(output)
	}
}

func Map2NearHeight(lightNode string, client *nearclient.Client) GetHeight {
	return func() (*big.Int, error) {
		res, err := client.ContractViewCallFunction(context.Background(), lightNode, NearHeaderHeight,
			"e30=", block.FinalityFinal())
		if err != nil {
			return nil, errors.Wrap(err, "call near lightNode to headerHeight failed")
		}

		if res.Error != nil {
			return nil, fmt.Errorf("call near lightNode to get headerHeight resp exist error(%v)", *res.Error)
		}

		result := "" // use string return
		err = json.Unmarshal(res.Result, &result)
		if err != nil {
			return nil, errors.Wrap(err, "near lightNode headerHeight resp json marshal failed")
		}
		ret := new(big.Int)
		ret.SetString(result, 10)
		return ret, nil
	}
}

func PackInput(commonAbi abi.ABI, abiMethod string, params ...interface{}) ([]byte, error) {
	input, err := commonAbi.Pack(abiMethod, params...)
	if err != nil {
		return nil, err
	}
	return input, nil
}

func UnpackHeaderHeightOutput(output []byte) (*big.Int, error) {
	outputs := Height.Methods[MethodOfHeaderHeight].Outputs
	unpack, err := outputs.Unpack(output)
	if err != nil {
		return big.NewInt(0), err
	}

	height := new(big.Int)
	if err = outputs.Copy(&height, unpack); err != nil {
		return big.NewInt(0), err
	}
	return height, nil
}

// HeaderHeight reads a headerHeight style getter on the MAP chain.
func HeaderHeight(caller ContractCaller, to common.Address, input []byte) (*big.Int, error) {
	output, err := caller.CallContract(context.Background(), goeth.CallMsg{From: constant.ZeroAddress, To: &to, Data: input}, nil)
	if err != nil {
		return nil, err
	}
	height, err := UnpackHeaderHeightOutput(output)
	if err != nil {
		return nil, err
	}
	return height, nil
}
