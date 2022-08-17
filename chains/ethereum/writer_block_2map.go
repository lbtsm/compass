package ethereum

import (
	"strings"
	"time"

	"github.com/mapprotocol/compass/mapprotocol"

	"github.com/mapprotocol/compass/msg"
)

// exeSyncMsg executes sync msg, and send tx to the destination blockchain
// the current function is only responsible for sending messages and is not responsible for processing data formats，
func (w *writer) exeSyncMsg(m msg.Message) bool {
	//return w.callContractWithMsg(,  m)
	for i := 0; i < TxRetryLimit; i++ {
		select {
		case <-w.stop:
			return false
		default:
			err := w.conn.LockAndUpdateOpts()
			if err != nil {
				w.log.Error("Failed to update nonce", "err", err)
				return false
			}
			// These store the gas limit and price before a transaction is sent for logging in case of a failure
			// This is necessary as tx will be nil in the case of an error when sending VoteProposal()
			gasLimit := w.conn.Opts().GasLimit
			gasPrice := w.conn.Opts().GasPrice

			marshal, _ := m.Payload[0].([]byte)
			// save header data
			data, err := mapprotocol.SaveHeaderTxData(marshal)
			if err != nil {
				w.log.Error("Failed to pack abi data", "err", err)
				w.conn.UnlockOpts()
				return false
			}
			tx, err := w.sendTx(&w.cfg.lightNode, nil, data)
			w.conn.UnlockOpts()
			if err == nil {
				// message successfully handled
				w.log.Info("Sync Header to map tx execution", "tx", tx.Hash(), "src", m.Source, "dst", m.Destination)
				time.Sleep(time.Second * 2)
				// waited till successful mined
				err = w.blockForPending(tx.Hash())
				if err != nil {
					w.log.Warn("Sync Header to map blockForPending error", "err", err)
				}
				m.DoneCh <- struct{}{}
				return true
			} else if err.Error() == ErrNonceTooLow.Error() || err.Error() == ErrTxUnderpriced.Error() {
				w.log.Error("Sync Header to map Nonce too low, will retry")
				time.Sleep(TxRetryInterval)
			} else if strings.Index(err.Error(), "EOF") != -1 { // When requesting the lightNode to return EOF, it indicates that there may be a problem with the network and it needs to be retried
				w.log.Error("Sync Header to map encounter EOF, will retry")
				time.Sleep(TxRetryInterval)
			} else if strings.Index(err.Error(), "max fee per gas less than block base fee") != -1 {
				w.log.Error("gas maybe less than base fee, will retry")
				time.Sleep(TxRetryInterval)
			} else {
				w.log.Warn("Sync Header to map Execution failed, header may already been synced", "gasLimit", gasLimit, "gasPrice", gasPrice, "err", err)
				m.DoneCh <- struct{}{}
				return true
			}
		}
	}
	w.log.Error("Sync Header to map Submission of Sync Header transaction failed", "source", m.Source, "dest", m.Destination, "depositNonce", m.DepositNonce)
	w.sysErr <- ErrFatalTx
	return false
}