package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Validators do not send transactions to each other one by one. They
// package everything they have to say in a *block*, and every block points
// to a quorum of blocks of the previous round. The result is a directed
// acyclic graph (DAG) that grows round after round. Two validators may
// observe it with a delay, but they agree on what lies below any block they
// both hold.
//
// The DAG does three jobs at once:
//
//  1. it disseminates transactions (the job of the mempool, session 2);
//  2. it makes the history below a block agreed upon: a validator that
//     holds a block holds its whole causal history, and the identifier of
//     the block commits to that history;
//  3. it *carries the votes*. A validator that references a block has seen
//     everything below it, so it can state its opinion on those
//     transactions in the very block it was going to send anyway. Voting
//     costs no extra message, and a transaction is final after two rounds.

// BlockID is a block identifier: the raw bytes of its hash.
type BlockID string

// Short returns an abbreviated hexadecimal form, for logs.
func (id BlockID) Short() string {
	h := hex.EncodeToString([]byte(id))
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// Vote is the opinion of the block's author about one transaction.
// Accept means "I have seen this transaction and I had not accepted any
// conflicting one before". Reject means "I had already accepted a
// transaction spending the same asset".
type Vote struct {
	Tx     TxID
	Accept bool
}

// Block is a vertex of the DAG. A validator emits at most one block per
// round. A Byzantine validator may emit two different blocks for the same
// round and show each to different validators: this is an equivocation
// (see lecture 4). The fast path survives it, as we will see below.
//
// A real block is signed by its author and serialized on the wire. Here
// validators run in the same process and share blocks by pointer, so a
// block must be treated as immutable once created.
type Block struct {
	ID      BlockID
	Author  int
	Round   uint64
	Parents []BlockID // blocks of round-1 this block acknowledges
	Txs     []*Tx     // transactions proposed by the author
	Votes   []Vote    // opinions on transactions of the causal history
}

// ComputeID returns the SHA-256 hash of the block contents. Hashing the
// parents is what turns the DAG into a Merkle DAG: naming a block names,
// transitively, its whole causal history.
//
// [Task] Serialize the author, the round, the parent identifiers, the
// transaction identifiers and the votes (identifier plus one byte for the
// accept bit), then hash the result with SHA-256. The beginning is given:
// complete it, following the same pattern for the other fields.
func (b *Block) ComputeID() BlockID {
	var data []byte
	data = binary.BigEndian.AppendUint64(data, uint64(b.Author))
	data = binary.BigEndian.AppendUint64(data, b.Round)
	// TODO: append the parents, the transactions and the votes (section 5)
	sum := sha256.Sum256(data)
	_ = sum
	return ""
}

// NewBlock assembles a block and computes its identifier.
func NewBlock(author int, round uint64, parents []BlockID, txs []*Tx, votes []Vote) *Block {
	b := &Block{Author: author, Round: round, Parents: parents, Txs: txs, Votes: votes}
	b.ID = b.ComputeID()
	return b
}

// GenesisBlock is the round-0 block of a validator: the first block of its
// chain, which has no parent and carries no transaction. Every validator
// starts with the genesis blocks of all the others, so the DAG has a
// common root and round-1 blocks have parents to point at.
func GenesisBlock(author int) *Block {
	return NewBlock(author, 0, nil, nil, nil)
}

// String implements fmt.Stringer for logs.
func (b *Block) String() string {
	return fmt.Sprintf("B%d:r%d:%s(%d txs, %d votes)", b.Author, b.Round, b.ID.Short(), len(b.Txs), len(b.Votes))
}
