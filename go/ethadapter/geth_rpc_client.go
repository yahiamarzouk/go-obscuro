package ethadapter

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"time"

	gethlog "github.com/ethereum/go-ethereum/log"

	"github.com/obscuronet/go-obscuro/go/common/retry"

	"github.com/obscuronet/go-obscuro/go/common/log"

	"github.com/obscuronet/go-obscuro/go/common"

	"github.com/ethereum/go-ethereum"

	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	connRetryMaxWait  = 10 * time.Minute // after this duration, we will stop retrying to connect and return the failure
	connRetryInterval = 500 * time.Millisecond
)

// gethRPCClient implements the EthClient interface and allows connection to a real ethereum node
type gethRPCClient struct {
	client  *ethclient.Client  // the underlying eth rpc client
	l2ID    gethcommon.Address // the address of the Obscuro node this client is dedicated to
	timeout time.Duration      // the timeout for connecting to, or communicating with, the L1 node
	logger  gethlog.Logger
}

// NewEthClient instantiates a new ethadapter.EthClient that connects to an ethereum node
func NewEthClient(ipaddress string, port uint, timeout time.Duration, l2ID gethcommon.Address, logger gethlog.Logger) (EthClient, error) {
	client, err := connect(ipaddress, port, timeout)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to the eth node - %w", err)
	}

	logger.Trace(fmt.Sprintf("Initialized eth node connection - addr: %s port: %d", ipaddress, port))
	return &gethRPCClient{
		client:  client,
		l2ID:    l2ID,
		timeout: timeout,
		logger:  logger,
	}, nil
}

func (e *gethRPCClient) FetchHeadBlock() (*types.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	blk, err := e.client.BlockByNumber(ctx, nil)
	if err != nil {
		e.logger.Crit("could not fetch head block.", log.ErrKey, err)
	}
	return blk, nil
}

func (e *gethRPCClient) Info() Info {
	return Info{
		L2ID: e.l2ID,
	}
}

func (e *gethRPCClient) BlocksBetween(startingBlock *types.Block, lastBlock *types.Block) []*types.Block {
	// TODO this should be a stream
	var blocksBetween []*types.Block
	var err error

	for currentBlk := lastBlock; currentBlk != nil && !bytes.Equal(currentBlk.Hash().Bytes(), startingBlock.Hash().Bytes()) && !bytes.Equal(currentBlk.ParentHash().Bytes(), gethcommon.HexToHash("").Bytes()); {
		currentBlk, err = e.BlockByHash(currentBlk.ParentHash())
		if err != nil {
			e.logger.Crit(fmt.Sprintf("could not fetch parent block with hash %s.", currentBlk.ParentHash().String()), log.ErrKey, err)
		}
		blocksBetween = append(blocksBetween, currentBlk)
	}

	return blocksBetween
}

func (e *gethRPCClient) IsBlockAncestor(block *types.Block, maybeAncestor common.L1RootHash) bool {
	if bytes.Equal(maybeAncestor.Bytes(), block.Hash().Bytes()) || bytes.Equal(maybeAncestor.Bytes(), (common.L1RootHash{}).Bytes()) {
		return true
	}

	if block.Number().Int64() == int64(common.L1GenesisHeight) {
		return false
	}

	resolvedBlock, err := e.BlockByHash(maybeAncestor)
	if err != nil {
		e.logger.Crit(fmt.Sprintf("could not fetch parent block with hash %s.", maybeAncestor.String()), log.ErrKey, err)
	}
	if resolvedBlock == nil {
		if resolvedBlock.Number().Int64() >= block.Number().Int64() {
			return false
		}
	}

	p, err := e.BlockByHash(block.ParentHash())
	if err != nil {
		e.logger.Crit(fmt.Sprintf("could not fetch parent block with hash %s", block.ParentHash().String()), log.ErrKey, err)
	}
	if p == nil {
		return false
	}

	return e.IsBlockAncestor(p, maybeAncestor)
}

func (e *gethRPCClient) SendTransaction(signedTx *types.Transaction) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.client.SendTransaction(ctx, signedTx)
}

func (e *gethRPCClient) TransactionReceipt(hash gethcommon.Hash) (*types.Receipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.client.TransactionReceipt(ctx, hash)
}

func (e *gethRPCClient) Nonce(account gethcommon.Address) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.client.PendingNonceAt(ctx, account)
}

func (e *gethRPCClient) BlockListener() (chan *types.Header, ethereum.Subscription) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// this channel holds blocks that have been received from the geth network but not yet processed by the host,
	// with more than 1 capacity the buffer provides resilience in case of intermittent RPC or processing issues
	ch := make(chan *types.Header, 100)
	var sub ethereum.Subscription
	var err error
	err = retry.Do(func() error {
		sub, err = e.client.SubscribeNewHead(ctx, ch)
		if err != nil {
			e.logger.Warn("could not subscribe for new head blocks", log.ErrKey, err)
		}
		return err
	}, retry.NewTimeoutStrategy(connRetryMaxWait, connRetryInterval))
	if err != nil {
		// todo: handle this scenario better after refactor of node.go (health monitor report L1 unavailable, be able to recover without restarting host)
		// couldn't connect after timeout period, cannot continue
		e.logger.Crit("could not subscribe for new head blocks.", log.ErrKey, err)
	}

	return ch, sub
}

func (e *gethRPCClient) BlockNumber() (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.client.BlockNumber(ctx)
}

func (e *gethRPCClient) BlockByNumber(n *big.Int) (*types.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.client.BlockByNumber(ctx, n)
}

func (e *gethRPCClient) BlockByHash(hash gethcommon.Hash) (*types.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.client.BlockByHash(ctx, hash)
}

func (e *gethRPCClient) CallContract(msg ethereum.CallMsg) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.client.CallContract(ctx, msg, nil)
}

func (e *gethRPCClient) EthClient() *ethclient.Client {
	return e.client
}

func (e *gethRPCClient) BalanceAt(gethcommon.Address, *big.Int) (*big.Int, error) {
	panic("not implemented")
}

func (e *gethRPCClient) Stop() {
	e.client.Close()
}

// EstimateGasAndGasPrice takes a txData type and overrides the Gas and Gas Price field with estimated values
func (e *gethRPCClient) EstimateGasAndGasPrice(txData types.TxData, from gethcommon.Address) (types.TxData, error) {
	unEstimatedTx := types.NewTx(txData)
	gasPrice, err := e.EthClient().SuggestGasPrice(context.Background())
	if err != nil {
		return nil, err
	}

	gasLimit, err := e.EthClient().EstimateGas(context.Background(), ethereum.CallMsg{
		From:  from,
		To:    unEstimatedTx.To(),
		Value: unEstimatedTx.Value(),
		Data:  unEstimatedTx.Data(),
	})
	if err != nil {
		return nil, err
	}

	return &types.LegacyTx{
		Nonce:    unEstimatedTx.Nonce(),
		GasPrice: gasPrice,
		Gas:      gasLimit,
		To:       unEstimatedTx.To(),
		Value:    unEstimatedTx.Value(),
		Data:     unEstimatedTx.Data(),
	}, nil
}

func connect(ipaddress string, port uint, connectionTimeout time.Duration) (*ethclient.Client, error) {
	var err error
	var c *ethclient.Client
	for start := time.Now(); time.Since(start) < connectionTimeout; time.Sleep(time.Second) {
		c, err = ethclient.Dial(fmt.Sprintf("ws://%s:%d", ipaddress, port))
		if err == nil {
			break
		}
	}

	return c, err
}
