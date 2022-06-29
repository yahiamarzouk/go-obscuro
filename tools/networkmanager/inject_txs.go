package networkmanager

import (
	"fmt"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/obscuronet/obscuro-playground/integration/simulation/params"

	"github.com/obscuronet/obscuro-playground/integration/simulation/stats"

	"github.com/obscuronet/obscuro-playground/go/config"
	"github.com/obscuronet/obscuro-playground/go/ethadapter"
	"github.com/obscuronet/obscuro-playground/go/ethadapter/erc20contractlib"
	"github.com/obscuronet/obscuro-playground/go/ethadapter/mgmtcontractlib"
	"github.com/obscuronet/obscuro-playground/go/rpcclientlib"
	"github.com/obscuronet/obscuro-playground/go/wallet"
	"github.com/obscuronet/obscuro-playground/integration/simulation"
)

func InjectTransactions(cfg Config, args []string) {
	hostConfig := config.HostConfig{
		L1NodeHost:          cfg.l1NodeHost,
		L1NodeWebsocketPort: cfg.l1NodeWebsocketPort,
		L1ConnectionTimeout: cfg.l1ConnectionTimeout,
	}
	l1Client, err := ethadapter.NewEthClient(hostConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create L1 client. Cause: %s", err))
	}
	l2Client := rpcclientlib.NewClient(cfg.obscuroClientAddress)

	txInjector := simulation.NewTransactionInjector(
		time.Second,
		stats.NewStats(1),
		[]ethadapter.EthClient{l1Client},
		createWallets(cfg, l1Client, l2Client),
		&cfg.mgmtContractAddress,
		[]rpcclientlib.Client{l2Client},
		mgmtcontractlib.NewMgmtContractLib(&cfg.mgmtContractAddress),
		erc20contractlib.NewERC20ContractLib(&cfg.mgmtContractAddress, &cfg.erc20ContractAddress),
		parseNumOfTxs(args),
	)

	println("Injecting transactions into network...")
	txInjector.Start()
	reportFinishedInjecting(txInjector)
}

func parseNumOfTxs(args []string) int {
	if len(args) != 1 {
		panic(fmt.Errorf("expected one argument to %s command, got %d", injectTxsName, len(args)))
	}
	numOfTxs, err := strconv.Atoi(args[0])
	if err != nil {
		panic(fmt.Errorf("could not parse number of transactions to inject. Cause: %w", err))
	}
	return numOfTxs
}

func createWallets(nmConfig Config, l1Client ethadapter.EthClient, l2Client rpcclientlib.Client) *params.SimWallets {
	wallets := params.NewSimWallets(len(nmConfig.privateKeys), 0, nmConfig.l1ChainID, nmConfig.obscuroChainID)

	// We override the autogenerated Ethereum wallets with ones using the provided private keys.
	wallets.SimEthWallets = make([]wallet.Wallet, len(nmConfig.privateKeys))
	for idx, privateKeyString := range nmConfig.privateKeys {
		privateKey, err := crypto.HexToECDSA(privateKeyString)
		if err != nil {
			panic(fmt.Errorf("could not recover private key from hex. Cause: %w", err))
		}
		l1Wallet := wallet.NewInMemoryWalletFromPK(big.NewInt(nmConfig.l1ChainID), privateKey)
		wallets.SimEthWallets[idx] = l1Wallet
	}

	// We update the L1 and L2 wallet nonces.
	for _, l1Wallet := range wallets.AllEthWallets() {
		nonce, err := l1Client.Nonce(l1Wallet.Address())
		if err != nil {
			panic(fmt.Errorf("could not set L1 wallet nonce. Cause: %w", err))
		}
		l1Wallet.SetNonce(nonce)
	}
	for _, l2Wallet := range wallets.AllObsWallets() {
		var nonce uint64
		err := l2Client.Call(&nonce, rpcclientlib.RPCNonce, l2Wallet.Address())
		if err != nil {
			panic(fmt.Errorf("could not set L2 wallet nonce. Cause: %w", err))
		}
		l2Wallet.SetNonce(nonce)
	}

	return wallets
}

func reportFinishedInjecting(txInjector *simulation.TransactionInjector) {
	println(fmt.Sprintf(
		"Stopped injecting transactions into network\nInjected %d L1 transactions, %d L2 transfer transactions, and %d L2 withdrawal transactions.",
		len(txInjector.Counter.L1Transactions), len(txInjector.Counter.TransferL2Transactions), len(txInjector.Counter.WithdrawalL2Transactions),
	))
	os.Exit(0)
}
