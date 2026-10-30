package main

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
)

// A node's mempool (short for "memory pool") is the set of transactions
// that the node has heard of, that are valid, and that are not yet in a
// block. It is a cache of pending transactions, not a ledger.
//
// This one follows the mempool of CometBFT:
//   - a hash map from the key of each transaction to its entry, which
//     answers "have I already seen it?" in O(1);
//   - a list of the transactions, in the order they arrived.
//     A block proposer takes the transactions from the front of the list:
//     first come, first served.
//
// Before a transaction enters the pool, the pool asks the application
// whether it is valid in the current state (p.app.IsValid). An invalid
// transaction is rejected, and therefore never relayed to the peers.

// ErrTxInPool is returned by Insert for a transaction already in the pool.
var ErrTxInPool = errors.New("transaction already in the mempool")

// ErrInvalidTx is returned by Insert for a transaction that the
// application rejects.
var ErrInvalidTx = errors.New("transaction not valid")

// Pool is a thread-safe mempool.
type Pool struct {
	mu   sync.Mutex
	app  Application
	txs  map[TxKey]*list.Element // index: the entry of each transaction in fifo
	fifo *list.List              // the transactions (type Tx), oldest first
}

// NewPool returns an empty mempool, whose transactions are validated by app.
func NewPool(app Application) *Pool {
	return &Pool{
		app:  app,
		txs:  make(map[TxKey]*list.Element),
		fifo: list.New(),
	}
}

// String implements fmt.Stringer for quick debugging.
func (p *Pool) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("mempool{%d pending}", len(p.txs))
}

// Size returns the number of pending transactions.
func (p *Pool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.txs)
}

// Contains reports whether a transaction with the given key is pending.
func (p *Pool) Contains(key TxKey) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.txs[key]
	return ok
}

// Insert adds a transaction at the end of the pool, if it is new and valid.
// It returns nil if the transaction was added,
// ErrTxInPool if it was already there, and ErrInvalidTx if it is not valid.
//
// [Task] Implement it:
//  1. return ErrTxInPool if the key of tx is in the map, *before* asking
//     the application (why does the order matter?);
//  2. return ErrInvalidTx if p.app.IsValid rejects tx;
//  3. add tx at the back of the list using PushBack(tx), and its entry to the map.
func (p *Pool) Insert(tx Tx) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return nil // TODO: dedup with the map, p.app.IsValid, then store in the list and the map (section 6)
}

// Remove deletes a transaction from the pool. It returns true if the pool
// changed, false if the key is unknown.
func (p *Pool) Remove(key TxKey) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.remove(key)
}

// remove is Remove without the lock: the caller must hold p.mu.
func (p *Pool) remove(key TxKey) bool {
	e, ok := p.txs[key]
	if !ok {
		return false
	}
	p.fifo.Remove(e)
	delete(p.txs, key)
	return true
}

// Reap returns, without removing them, the max oldest transactions of the
// pool, or all of them if max is negative. This is what a block proposer puts in its block.
func (p *Pool) Reap(max int) []Tx {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []Tx
	for e := p.fifo.Front(); e != nil && len(out) != max; e = e.Next() {
		out = append(out, e.Value.(Tx))
	}
	return out
}

// Update is called once the network has decided a block (CometBFT:
// Update). The application executes the block, then the pool removes the
// transactions of the block, and *rechecks* the others: a transaction that
// was valid before the block may no longer be valid after it.
//
// [Task] Implement the two loops: remove each transaction of the block,
// then go through the list, oldest first, and remove each transaction that
// the application (p.app.IsValid) now rejects.
func (p *Pool) Update(block []Tx) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.app.Commit(block)
	// TODO: remove the transactions of the block, then recheck the others (section 7)
}
