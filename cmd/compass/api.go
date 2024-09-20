package main

import (
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/compass/chains"
	"github.com/mapprotocol/compass/config"
	connection "github.com/mapprotocol/compass/connections/ethereum"
	"github.com/mapprotocol/compass/core"
	"github.com/mapprotocol/compass/internal/chain"
	"github.com/mapprotocol/compass/internal/server"
	"github.com/mapprotocol/compass/mapprotocol"
	"github.com/mapprotocol/compass/msg"
	"github.com/mapprotocol/compass/pkg/util"
	"github.com/urfave/cli/v2"
	"strconv"
)

var apiCommand = cli.Command{
	Name:        "server",
	Usage:       "server server",
	Description: "The server command is provide server server",
	Action:      api,
	Flags:       append(app.Flags, cliFlags...),
}

func api(ctx *cli.Context) error {
	// step1 : 读取配置
	cfg, err := config.GetConfig(ctx)
	if err != nil {
		return err
	}
	util.Init(cfg.Other.Env, cfg.Other.MonitorUrl)
	allChains := make([]config.RawChainConfig, 0, len(cfg.Chains)+1)
	allChains = append(allChains, cfg.MapChain)
	allChains = append(allChains, cfg.Chains...)
	for _, ele := range allChains {
		cId, err := strconv.ParseInt(ele.Id, 10, 64)
		if err != nil {
			return err
		}
		chainId, err := strconv.Atoi(ele.Id)
		if err != nil {
			return err
		}
		chainConfig := &core.ChainConfig{
			Name:     ele.Name,
			Id:       msg.ChainId(chainId),
			Endpoint: ele.Endpoint,
			From:     ele.From,
			Network:  ele.Network,
			Opts:     ele.Opts,
		}
		var conn core.Connection
		switch ele.Type {
		case chains.Near:
		default:
			conn, err = chain.NewApi(chainConfig, connection.NewConnection)
		}
		if err != nil {
			return err
		}

		mapprotocol.OnlineChaId[msg.ChainId(cId)] = ele.Name
		mapprotocol.OnlineConn[msg.ChainId(cId)] = conn
	}

	g := gin.New()
	v1 := g.Group("/compass/api/v1")
	v1.POST("/get/proof", server.GetProofHandler)
	return nil
}
