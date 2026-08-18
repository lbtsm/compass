package sol

import (
	"fmt"

	"github.com/ChainSafe/log15"
	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/internal/chain"
	"github.com/mapprotocol/compass/internal/mapprotocol"
	"github.com/mapprotocol/compass/pkg/keystore"
	"github.com/mapprotocol/compass/pkg/util"
)

type Chain struct {
	chain.Base
}

func New() *Chain {
	return &Chain{}
}

func (c *Chain) New(chainCfg *core.ChainConfig, logger log15.Logger, sysErr chan<- error, role mapprotocol.Role) (core.Chain, error) {
	return createChain(chainCfg, logger, sysErr, role)
}

func createChain(chainCfg *core.ChainConfig, logger log15.Logger, sysErr chan<- error, role mapprotocol.Role) (core.Chain, error) {
	config, err := parseCfg(chainCfg)
	if err != nil {
		return nil, err
	}

	conn := NewConnection(config.Endpoint, logger)
	err = conn.Connect()
	if err != nil {
		return nil, err
	}

	pswd := make([]byte, 0)
	if role == mapprotocol.RoleOfMessenger {
		pswd = keystore.GetPassword(fmt.Sprintf("Enter password for key %s:", chainCfg.From))

		pri, err := util.DecryptKeystoreText(string(pswd), config.KeystorePath)
		if err != nil {
			return nil, err
		}
		config.Pri = pri
	}

	var (
		stop   = make(chan int)
		listen core.Listener
	)
	bs, err := chain.SetupBlockStore(&config.Config, role)
	if err != nil {
		return nil, err
	}
	if chainCfg.StartLatest {
		if err := chain.StartLatestBlock(&config.Config, conn, logger); err != nil {
			return nil, err
		}
	}
	cs := chain.NewCommonSync(nil, &config.Config, logger, stop, sysErr, bs)
	cs.RegisterState(config.Name, string(role))

	switch role {
	case mapprotocol.RoleOfMessenger:
		listen = newSync(cs, mosHandler, conn, config, conn.cli)
	case mapprotocol.RoleOfOracle:
		listen = newSync(cs, oracleHandler, conn, config, conn.cli)
	}
	mapprotocol.MosMapping[config.Id] = config.McsContract[0]

	writer := newWriter(conn, config, logger, stop, sysErr)
	return &Chain{Base: chain.NewBase(chainCfg, conn, writer, listen, stop)}, nil
}
