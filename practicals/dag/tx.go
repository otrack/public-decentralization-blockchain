package main

import (
	"encoding/hex"
	"fmt"
)

// A transaction in this practical transfers some amount from one account
// to another. What makes it special is that it has a *single owner*: only
// the account holder can authorize it. Nobody else in the system competes
// for the right to spend that money.
//
// This is the whole point of the fast path. Two transfers signed by two
// different owners are independent: there is no reason to order them, they
// can be applied in any order and every validator will end up with the same
// balances, provided each transfer is funded by money its owner already
// holds. Consensus -- agreeing on a total order -- is only needed when two
// transactions may conflict, and here the only way to create a conflict is
// to spend the same money twice, which only the owner can do.
//
// Wei Dai made this observation in b-money (1998), long before it had a
// name; FastPay and Sui turned it into a protocol.

// TxID is a transaction identifier: the raw bytes of its hash, carried in
// a string so that it can be used as a map key.
type TxID string

// Short returns an abbreviated hexadecimal form, for logs.
func (id TxID) Short() string {
	h := hex.EncodeToString([]byte(id))
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// Tx transfers Amount from Owner to To. Seq is the sequence number of the
// Owner's account: it plays the role of the *version* of the asset being
// spent (an "owned object" in Sui parlance). The pair (Owner, Seq) is what
// the transaction consumes, and consuming it twice is a double spend.
//
// A real transaction would carry the owner's signature. We omit signatures
// entirely: assume the network only ever delivers transactions that the
// owner did sign.
type Tx struct {
	ID     TxID
	Owner  string
	Seq    uint64
	To     string
	Amount uint64
}

// ComputeHash returns the SHA-256 hash of the transaction contents.
//
// [Task] Serialize Owner, Seq (big-endian), To and Amount (big-endian),
// then hash the result with SHA-256. Note that ID itself is never part of
// the hash. Big-endian means that the most significant byte comes first.
// The README explains it, and has a help on concatenating byte slices.
func (t *Tx) ComputeHash() TxID {
	return "" // TODO: hash the serialized contents (section 4)
}

// ConflictKey identifies the asset that this transaction consumes. Two
// transactions conflict -- they are a double spend -- exactly when they
// have the same conflict key but different identifiers.
//
// [Task] Return a string that identifies the (Owner, Seq) pair, for
// instance "alice/3". Use fmt.Sprintf.
func (t *Tx) ConflictKey() string {
	return "" // TODO: identify the asset consumed by this transaction (section 4)
}

// NewTx builds a transaction and computes its identifier.
func NewTx(owner string, seq uint64, to string, amount uint64) *Tx {
	tx := &Tx{Owner: owner, Seq: seq, To: to, Amount: amount}
	tx.ID = tx.ComputeHash()
	return tx
}

// String implements fmt.Stringer for logs.
func (t *Tx) String() string {
	return fmt.Sprintf("%s[%s pays %d to %s, seq %d]", t.ID.Short(), t.Owner, t.Amount, t.To, t.Seq)
}
