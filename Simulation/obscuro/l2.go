package obscuro

import (
	"fmt"
	"github.com/google/uuid"
	"time"
)

type L2Cfg struct {
	gossipPeriodMs int
}

type L2Agg struct {
	id      int
	l1      *L1Miner
	cfg     L2Cfg
	network *NetworkCfg

	// control the lifecycle
	runCh1 chan bool
	runCh2 chan bool

	// where rollups and transactions are gossipped from peers
	p2pChRollup chan *Rollup
	p2pChTx     chan *L2Tx

	// where the connected L1 node drops new blocks
	rpcCh chan *Block

	// used for internal communication between the gossi agent and the processing agent
	// todo - probably can use a single channel
	rollupIntCh chan int
	rollupOutCh chan []*Rollup

	// used for internal communication between the gossi agent and the processing agent
	txsIntCh chan bool
	txsOutCh chan []*L2Tx

	// when a new rollup is discovered
	canonicalCh chan *Rollup
}

func NewAgg(id int, cfg L2Cfg, l1 *L1Miner, network *NetworkCfg) L2Agg {
	return L2Agg{
		id:          id,
		cfg:         cfg,
		network:     network,
		l1:          l1,
		runCh1:      make(chan bool),
		runCh2:      make(chan bool),
		p2pChRollup: make(chan *Rollup),
		p2pChTx:     make(chan *L2Tx),
		rpcCh:       make(chan *Block),
		rollupIntCh: make(chan int),
		rollupOutCh: make(chan []*Rollup),
		txsIntCh:    make(chan bool),
		txsOutCh:    make(chan []*L2Tx),
		canonicalCh: make(chan *Rollup),
	}
}

type L2RootHash = uuid.UUID
type StateRoot = string

type Rollup struct {
	height       int
	rootHash     L2RootHash
	agg          *L2Agg
	parent       *Rollup
	creationTime time.Time
	l1Proof      *Block // the L1 block where the parent was published
	nonce        Nonce
	txs          []*L2Tx
	state        StateRoot
}

// Data structure to be used once a rollup was included in a block
type IncludedRollup struct {
	l2 *Rollup // the rollup
	l1 *Block  // the block where it was included
}

// Transfers and Withdrawals for now
type L2TxType int64

const (
	TransferTx L2TxType = iota
	WithdrawalTx
)

type L2TxId = uuid.UUID

// no signing for now
type L2Tx struct {
	id     L2TxId // this is the hash
	txType L2TxType
	amount int
	from   Address
	dest   Address
}

var GenesisRollup = Rollup{-1,
	uuid.New(),
	nil,
	nil,
	time.Now(),
	nil,
	0,
	[]*L2Tx{},
	"",
}

func (a L2Agg) Start() {
	go a.startGossip()

	var l1Head = &GenesisBlock

	for {
		select {
		// Main loop
		// Listen for notifications from the L1 node and process them
		// Note that during processing, more recent notifications can be received
		case b := <-a.rpcCh:
			{
				// make a copy of the head to pass to the processor
				headCopy := *l1Head
				l1Head = b
				go a.processBlock(b, &headCopy)
			}
		case _ = <-a.runCh1:
			return
		}
	}
}

// dumb actor that participates in gossip and responds will all the rollups for a certain height
func (a L2Agg) startGossip() {

	// Rollups grouped by height
	var allRollups = make(map[int][]*Rollup)

	// transactions
	var allTxs = make([]*L2Tx, 0)

	for {
		select {

		case tx := <-a.p2pChTx:
			allTxs = append(allTxs, tx)

		case r := <-a.p2pChRollup:
			val, found := allRollups[r.height]
			if found {
				allRollups[r.height] = append(val, r)
			} else {
				allRollups[r.height] = []*Rollup{r}
			}

		case requestedHeight := <-a.rollupIntCh:
			a.rollupOutCh <- allRollups[requestedHeight]
		case _ = <-a.txsIntCh:
			a.txsOutCh <- allTxs

		case r := <-a.canonicalCh:
			//todo - optimize here the rollup storage
			fmt.Println(r.height)

		case _ = <-a.runCh2:
			return
		}
	}
}

// RPCNewHead Receive notifications from the L1 Node when there's a new block
func (a L2Agg) RPCNewHead(b *Block) {
	a.rpcCh <- b
}

// L2P2PGossipRollup is called by counterparties when there is a Rollup to broadcast
// All it does is drop the Rollups in a channel for processing.
func (a L2Agg) L2P2PGossipRollup(r *Rollup) {
	a.p2pChRollup <- r
}

func (a L2Agg) L2P2PReceiveTx(tx *L2Tx) {
	a.p2pChTx <- tx
}

// main block processing logic
func (a L2Agg) processBlock(b *Block, l1Head *Block) {
	// round starts when a new canonical L1 block was produced

	// 1. Find the new canonical L2 chain
	// Note that the previous L1 head is passed in as well, so that the logic can recognize L1 reorgs and replay the state
	// from the forking block
	// Also calculates the state
	bs := a.calculateL2State(b, l1Head)
	newL2Head := bs.head

	a.canonicalCh <- newL2Head

	// determine the transactions to be included
	txs := a.currentTxs(newL2Head)

	// calculate the state after executing them
	stateAfter := a.calculateState(txs, bs.state)

	// Create a new rollup based on the proof of inclusion of the previous, including all new transactions
	r := Rollup{newL2Head.height + 1, uuid.New(), &a, newL2Head, time.Now(), b, generateNonce(), txs, serialize(stateAfter)}
	a.network.broadcastRollupL2(&r)

	// wait to receive rollups from peers
	// todo - make this smarter. e.g: if 90% of the peers have sent rollups, proceed. Or if a nonce is very low and probabilistically there is no chance, etc
	Schedule(a.cfg.gossipPeriodMs, func() {

		// request the new generation rolllups
		a.rollupIntCh <- newL2Head.height + 1
		rollups := <-a.rollupOutCh

		// filter out rollups with a different parent
		var usefulRollups = []*Rollup{&r}
		for _, rol := range rollups {
			if rol.parent.rootHash == newL2Head.rootHash {
				usefulRollups = append(usefulRollups, rol)
			}
		}

		// determine the winner of the round
		winner := a.findRoundWinner(usefulRollups, newL2Head)

		// we are the winner
		if winner.agg.id == a.id {
			a.network.f.WriteString(fmt.Sprintf("-   Agg%d rollup=r%d(height=%d)[r%d] No Txs: %d. State=%s\n", a.id, winner.rootHash.ID(), winner.height, winner.parent.rootHash.ID(), len(winner.txs), winner.state))
			// build a L1 tx with the rollup
			a.network.broadcastL1Tx(&L1Tx{id: uuid.New(), txType: RollupTx, rollup: winner})
		}
	})
}

// Calculate transactions to be included in the current rollup
func (a L2Agg) currentTxs(head *Rollup) []*L2Tx {
	// Requests all l2 transactions received over gossip
	a.txsIntCh <- true
	txs := <-a.txsOutCh
	// and return only the ones not included in any rollup so far
	return FindNotIncludedTxs(head, txs)
}

// Complex logic to determine the new canonical head and to calculate the state
// Uses cache-ing to map the head rollup for each block in case of rollbacks.
func (a L2Agg) calculateL2State(b *Block, l1Head *Block) BlockState {
	dbMutex.RLock()
	val, found := globalDb[b.rootHash]
	dbMutex.RUnlock()
	if found {
		return val
	}

	//  1. The genesis rollup is part of the canonical chain and will be included in an L1 block by the first Aggregator.
	if b.rootHash == GenesisBlock.rootHash {
		bs := BlockState{
			head:  &GenesisRollup,
			state: make(State),
		}
		dbMutex.Lock()
		globalDb[b.rootHash] = bs
		dbMutex.Unlock()
		return bs
	}

	// There was no fork
	if l1Head.parent == nil || b.parent == l1Head {
		var newHead *Rollup = nil
		dbMutex.RLock()
		parentState, parentFound := globalDb[b.parent.rootHash]
		dbMutex.RUnlock()

		if !parentFound {
			// this is called when for some reason the parent was not cached.
			parentState = a.calculateL2State(b.parent, l1Head)
		}

		// find the head rollup according to the rules
		for _, t := range b.txs {
			if t.txType == RollupTx {
				r := t.rollup
				if parentState.head.height < r.height {
					if newHead == nil || r.height > newHead.height || r.l1Proof.height > newHead.l1Proof.height || (r.l1Proof.height == newHead.l1Proof.height && r.nonce < newHead.nonce) {
						newHead = r
					}
				}
			}
		}

		s := copyState(parentState.state)
		s = processDeposits(b, s)

		// only apply transactions if there is a new l2 head
		if newHead == nil {
			newHead = parentState.head
			s = a.calculateState(newHead.txs, s)
		}

		bs := BlockState{
			head:  newHead,
			state: s,
		}

		dbMutex.Lock()
		globalDb[b.rootHash] = bs
		dbMutex.Unlock()

		return bs
	}

	// Reorg
	fork := lca(b, l1Head)

	if !IsAncestor(l1Head, b) {
		statsMu.Lock()
		a.network.stats.noReorgs++
		statsMu.Unlock()

		// There was a fork
		a.network.f.WriteString(fmt.Sprintf("Agg%d :Reorg new=%d(%d), old=%d(%d), fork=%d(%d)\n", a.id, b.rootHash.ID(), b.height, l1Head.rootHash.ID(), l1Head.height, fork.rootHash.ID(), fork.height))
	}

	dbMutex.RLock()
	forkL2, forkFound := globalDb[fork.rootHash]
	dbMutex.RUnlock()

	if !forkFound {
		panic("wtf2")
	}
	// walk back to the fork and find the new canonical chain
	rerun := make([]*Block, 0)
	c := b
	for {
		if c.rootHash == fork.rootHash {
			break
		}
		rerun = append(rerun, c)
		c = c.parent
	}

	var l2 = forkL2
	for i := len(rerun) - 1; i >= 0; i-- {
		l1 := rerun[i]
		l2 = a.calculateL2State(l1, l1.parent)
	}
	dbMutex.Lock()
	globalDb[b.rootHash] = l2
	dbMutex.Unlock()
	return l2
}

func processDeposits(b *Block, s State) State {
	for _, tx := range b.txs {
		if tx.txType == DepositTx {
			_, f := s[tx.dest]
			if !f {
				s[tx.dest] += tx.amount
			} else {
				s[tx.dest] = tx.amount
			}
		}
	}
	return s
}

func (a L2Agg) findRoundWinner(receivedRollups []*Rollup, head *Rollup) *Rollup {
	var win *Rollup
	for _, r := range receivedRollups {
		if r.parent.rootHash != head.rootHash {
			continue
		}
		if win == nil || r.l1Proof.height > win.l1Proof.height || (r.l1Proof.height == win.l1Proof.height && r.nonce < win.nonce) {
			win = r
		}
	}
	return win
}

func (a L2Agg) Stop() {
	a.runCh1 <- false
	a.runCh2 <- false
}
