package server

import (
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/gin-gonic/gin"
	"github.com/mapprotocol/compass/mapprotocol"
	"github.com/mapprotocol/compass/msg"
	"net/http"
	"strconv"
)

func GetProofHandler(ctx *gin.Context) {
	var req GetProofRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		WriteResponse(ctx, err, nil)
		return
	}
	cId, err := strconv.ParseInt(req.ChainID, 10, 64)
	if err != nil {
		WriteResponse(ctx, err, nil)
		return
	}
	if _, ok := mapprotocol.OnlineChaId[msg.ChainId(cId)]; !ok {
		WriteResponse(ctx, fmt.Errorf("chain id(%d), not support", cId), nil)
		return
	}

	conn := mapprotocol.OnlineConn[msg.ChainId(cId)]
	//tx, _, err := conn.Client().TransactionByHash(ctx, common.HexToHash(req.TxHash))
	//if err != nil {
	//	WriteResponse(ctx, err, nil)
	//	return
	//}

	conn.Client().FilterLogs(ctx, ethereum.FilterQuery{})

	//proofType, err = chain.PreSendTx(idx, uint64(m.Cfg.Id), req.ChainID, big.NewInt(0).SetUint64(log.BlockNumber), orderId)
	//switch cId {
	//case constant.CfxChainId:
	//case constant.BscChainId:
	//case constant.TronChainId:
	//case constant.NearChainId:
	//case constant.EthChainId:
	//case constant.KlaytnChainId:
	//case constant.MaticChainId:
	//default:
	//
	//}
}

func WriteResponse(c *gin.Context, err error, data interface{}) {
	if err != nil {
		//add err to middleware context
		c.JSON(http.StatusInternalServerError, ErrResponse{
			Code:    "500",
			Message: err.Error(),
		})

		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Data:    data,
		Message: "success",
	})
}
