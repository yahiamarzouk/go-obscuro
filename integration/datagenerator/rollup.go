package datagenerator

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/obscuronet/go-obscuro/go/common"
)

// RandomRollup - block is needed in order to pass the smart contract check
// when submitting cross chain messages.
func RandomRollup(block *types.Block) common.ExtRollup {
	extRollup := common.ExtRollup{
		Header: &common.RollupHeader{
			ParentHash: randomHash(),
			Agg:        RandomAddress(),
			L1Proof:    randomHash(),
			Root:       randomHash(),
			Number:     big.NewInt(int64(RandomUInt64())),
		},
	}

	if block != nil {
		extRollup.Header.LatestInboundCrossChainHeight = block.Number()
		extRollup.Header.LatestInboundCrossChainHash = block.Hash()
	}

	return extRollup
}
