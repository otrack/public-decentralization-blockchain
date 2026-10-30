package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Tx is a transaction, as the mempool sees it, that is an opaque sequence of bytes.
// Only the application (app.go) knows what the bytes mean.
type Tx []byte

// One identifies a transaction with the SHA-256 digest of its bytes.
// A key is never received from the network.
// Each node simply computes it from the bytes it holds.
type TxKey [sha256.Size]byte

// Key returns the key of the transaction.
func (tx Tx) Key() TxKey {
	return sha256.Sum256(tx)
}

// String prints the first bytes of the key, enough to tell transactions apart in the logs.
func (k TxKey) String() string {
	return hex.EncodeToString(k[:4])
}

// NewTransfer builds the transaction that moves 'amount' units from the
// account 'from' to the account 'to'.
//
// [Task] Fill a Transfer and encode it with proto.Marshal. Encoding such a simple message cannot fail, Call panic() with the error if it does.
func NewTransfer(from, to string, amount uint64) Tx {
	return nil // TODO: encode a Transfer with proto.Marshal (section 4)
}

// DecodeTransfer decodes the Transfer carried by a transaction. The bytes
// come from the network: anybody can send anything, so a malformed
// transaction must give an error, never a crash.
//
// [Task] Decode tx with proto.Unmarshal.
func DecodeTransfer(tx Tx) (*Transfer, error) {
	return nil, fmt.Errorf("TODO: decode a Transfer with proto.Unmarshal (section 4)")
}

// describe returns a readable form of a transaction, for the logs.
func describe(tx Tx) string {
	t, err := DecodeTransfer(tx)
	if err != nil {
		return fmt.Sprintf("%v (malformed)", tx.Key())
	}
	return fmt.Sprintf("%v (%s->%s: %d)", tx.Key(), t.From, t.To, t.Amount)
}
