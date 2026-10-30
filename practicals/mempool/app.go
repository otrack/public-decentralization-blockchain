package main

import (
	"maps"
	"sync"
)

// Application is the part of a node that gives a meaning to transactions.
// In CometBFT, the application is a separate program, which the node calls
// through an interface named ABCI; we keep the two calls that matter to the
// mempool.
type Application interface {
	// IsValid reports whether tx is valid in the current state (ABCI:
	// CheckTx). A valid transaction is applied to the current state, so
	// that the next call sees it.
	IsValid(tx Tx) bool

	// Commit executes the transactions of a block that the network has
	// decided (ABCI: FinalizeBlock, then Commit). The current state is
	// then the state after the block.
	Commit(block []Tx)
}

// Bank is a toy application: accounts, each with a balance, and transfers
// between them.
//
// It keeps two copies of the balances, as the applications of CometBFT do:
//   - committed: the balances after the last block, the same on every node;
//   - check: committed, plus the transactions that IsValid has accepted
//     since; it is the "current state" against which IsValid validates a
//     transaction, and it is local to the node.
type Bank struct {
	mu        sync.Mutex
	committed map[string]uint64
	check     map[string]uint64
}

// NewBank returns a bank whose accounts hold the given balances.
func NewBank(genesis map[string]uint64) *Bank {
	return &Bank{committed: maps.Clone(genesis), check: maps.Clone(genesis)}
}

// apply executes the transfer tx on the balances in state, and returns
// true; or returns false, leaving state unchanged, if tx is not valid in
// state. A transfer is valid when it decodes, its amount is positive, and
// the sender holds at least that amount. An account that does not appear
// in state holds 0.
//
// [Task] Implement it.
func apply(state map[string]uint64, tx Tx) bool {
	return false // TODO: decode the transfer, check it, then move the amount (section 5)
}

// IsValid validates tx against the check state, and applies it there.
func (b *Bank) IsValid(tx Tx) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return apply(b.check, tx)
}

// Commit applies the block to the committed state, then resets the check
// state to it. A transaction of the block that is not valid is skipped,
// the same way on every node.
func (b *Bank) Commit(block []Tx) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, tx := range block {
		apply(b.committed, tx)
	}
	b.check = maps.Clone(b.committed)
}

// Balance returns the committed balance of an account.
func (b *Bank) Balance(account string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.committed[account]
}
