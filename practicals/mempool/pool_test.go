package main

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// The mempool has two hard invariants that all tests rely on: no
// duplicate transaction, whatever happens; and no transaction that the
// application rejects.

// newTestPool returns a pool backed by a bank where alice holds 100 and
// bob holds 50.
func newTestPool() *Pool {
	return NewPool(NewBank(map[string]uint64{"alice": 100, "bob": 50}))
}

// TestTransferRoundTrip checks that DecodeTransfer recovers what
// NewTransfer encoded, and rejects bytes that are not a Transfer.
func TestTransferRoundTrip(t *testing.T) {
	tx := NewTransfer("alice", "bob", 42)
	if len(tx) == 0 {
		t.Fatal("NewTransfer returned no bytes")
	}
	got, err := DecodeTransfer(tx)
	if err != nil {
		t.Fatal(err)
	}
	if got.From != "alice" || got.To != "bob" || got.Amount != 42 {
		t.Fatalf("round-trip changed the transfer: %v", got)
	}
	if _, err := DecodeTransfer(Tx{0xff}); err == nil {
		t.Fatal("decoding garbage must fail")
	}
}

// TestKey checks that the key of a transaction depends on its content
// only.
func TestKey(t *testing.T) {
	a := NewTransfer("alice", "bob", 10)
	b := NewTransfer("alice", "bob", 10)
	c := NewTransfer("alice", "bob", 11)
	if a.Key() != b.Key() {
		t.Fatal("the same transfer must have the same key")
	}
	if a.Key() == c.Key() {
		t.Fatal("two different transfers must have different keys")
	}
}

// TestIsValid checks the validity rules of the bank, and that a valid
// transfer changes the check state, but not the committed one.
func TestIsValid(t *testing.T) {
	bank := NewBank(map[string]uint64{"alice": 100})
	if !bank.IsValid(NewTransfer("alice", "bob", 60)) {
		t.Fatal("alice holds 100, she can send 60")
	}
	if bank.IsValid(NewTransfer("alice", "carol", 60)) {
		t.Fatal("alice holds 40 after her first transfer, she cannot send 60")
	}
	if !bank.IsValid(NewTransfer("bob", "carol", 60)) {
		t.Fatal("bob received 60 from alice, he can send them")
	}
	if bank.IsValid(NewTransfer("dave", "carol", 1)) {
		t.Fatal("dave holds nothing, he cannot send 1")
	}
	if bank.IsValid(NewTransfer("alice", "bob", 0)) {
		t.Fatal("a zero amount is not valid")
	}
	if bank.IsValid(Tx{0xff}) {
		t.Fatal("a malformed transaction is not valid")
	}
	if bank.Balance("alice") != 100 || bank.Balance("bob") != 0 {
		t.Fatal("IsValid must not change the committed state")
	}
}

// TestDedup checks that inserting the same transaction twice is a no-op,
// also for the application: the amount is debited once.
func TestDedup(t *testing.T) {
	pool := newTestPool()
	tx := NewTransfer("alice", "bob", 60)

	if err := pool.Insert(tx); err != nil {
		t.Fatalf("first insert should succeed: %v", err)
	}
	if err := pool.Insert(tx); !errors.Is(err, ErrTxInPool) {
		t.Fatalf("second insert of the same tx must return ErrTxInPool, got %v", err)
	}
	if size := pool.Size(); size != 1 {
		t.Fatalf("expected size 1, got %d", size)
	}
	if err := pool.Insert(NewTransfer("alice", "carol", 40)); err != nil {
		t.Fatalf("alice still holds 40, the duplicate must not be debited: %v", err)
	}
}

// TestInvalid checks that a transaction the application rejects does not
// enter the pool.
func TestInvalid(t *testing.T) {
	pool := newTestPool()
	if err := pool.Insert(NewTransfer("alice", "bob", 1000)); !errors.Is(err, ErrInvalidTx) {
		t.Fatalf("an invalid transfer must return ErrInvalidTx, got %v", err)
	}
	if size := pool.Size(); size != 0 {
		t.Fatalf("expected size 0, got %d", size)
	}
}

// TestFIFO checks that Reap returns the transactions in the order they
// arrived, and leaves them in the pool.
func TestFIFO(t *testing.T) {
	pool := newTestPool()
	want := []Tx{
		NewTransfer("bob", "carol", 30),
		NewTransfer("alice", "bob", 1),
		NewTransfer("alice", "dave", 2),
	}
	for _, tx := range want {
		if err := pool.Insert(tx); err != nil {
			t.Fatal(err)
		}
	}
	if got := pool.Reap(2); len(got) != 2 || !bytes.Equal(got[0], want[0]) || !bytes.Equal(got[1], want[1]) {
		t.Fatalf("Reap(2) must return the two oldest transactions")
	}
	got := pool.Reap(-1)
	if len(got) != len(want) {
		t.Fatalf("Reap(-1) returned %d transactions, expected %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("Reap: transaction #%d is %s, expected %s", i, describe(got[i]), describe(want[i]))
		}
	}
	if size := pool.Size(); size != len(want) {
		t.Fatalf("Reap must not remove transactions: size %d", size)
	}
}

// TestUpdate checks that committing a block removes its transactions from
// the pool, and the transactions that are no longer valid after it.
func TestUpdate(t *testing.T) {
	pool := newTestPool()
	toBob := NewTransfer("alice", "bob", 70)
	toCarol := NewTransfer("alice", "carol", 70) // the double spend, decided elsewhere
	committed := NewTransfer("bob", "carol", 5)
	fromBob := NewTransfer("bob", "dave", 10)
	for _, tx := range []Tx{toBob, committed, fromBob} {
		if err := pool.Insert(tx); err != nil {
			t.Fatal(err)
		}
	}

	pool.Update([]Tx{toCarol, committed})

	if pool.Contains(committed.Key()) {
		t.Fatal("a transaction of the block must leave the pool")
	}
	if pool.Contains(toBob.Key()) {
		t.Fatal("alice holds 30 after the block: her transfer of 70 is no longer valid")
	}
	if !pool.Contains(fromBob.Key()) {
		t.Fatal("bob still holds 45 after the block: his transfer of 10 is still valid")
	}
	if size := pool.Size(); size != 1 {
		t.Fatalf("expected size 1, got %d", size)
	}
	if err := pool.Insert(NewTransfer("bob", "dave", 40)); err == nil {
		t.Fatal("bob holds 45, minus 10 pending: he cannot send 40 more")
	}
}

// TestConcurrentInsert hammers the pool from many goroutines, each
// inserting a distinct transfer of 1 from alice, who holds only 100: the
// pool must end with exactly 100 of them. Run with `go test -race` to be
// sure the locks are doing their job.
func TestConcurrentInsert(t *testing.T) {
	const n = 1000
	pool := newTestPool()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pool.Insert(NewTransfer("alice", fmt.Sprint("account", i), 1))
		}(i)
	}
	wg.Wait()

	if size := pool.Size(); size != 100 {
		t.Fatalf("expected 100 transactions, got %d", size)
	}
}

// TestFlooding starts three nodes on the local machine, connected to each
// other, and checks that a valid transaction submitted to one of them
// reaches the other two, and that an invalid one stays where it was sent.
func TestFlooding(t *testing.T) {
	genesis := map[string]uint64{"alice": 100}
	var nodes []*Node
	for i := 0; i < 3; i++ {
		n := NewNode("127.0.0.1:0", NewBank(genesis)) // port 0: any free port
		if err := n.Listen(); err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
	}
	for _, n := range nodes {
		for _, m := range nodes {
			if n != m {
				n.AddPeer(m.addr)
			}
		}
	}

	valid := NewTransfer("alice", "bob", 60)
	if err := nodes[0].Relay(valid); err != nil {
		t.Fatalf("alice holds 100, she can send 60: %v", err)
	}
	for _, n := range nodes {
		deadline := time.Now().Add(2 * time.Second)
		for !n.pool.Contains(valid.Key()) {
			if time.Now().After(deadline) {
				t.Fatalf("%s did not receive the transaction", n.addr)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	invalid := NewTransfer("alice", "carol", 60)
	if err := nodes[0].Relay(invalid); !errors.Is(err, ErrInvalidTx) {
		t.Fatalf("alice holds only 40 now, her transfer of 60 must be rejected, got %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	for _, n := range nodes[1:] {
		if n.pool.Contains(invalid.Key()) {
			t.Fatalf("%s received a transaction that was not valid", n.addr)
		}
	}
}
