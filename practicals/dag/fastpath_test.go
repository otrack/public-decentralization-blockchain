package main

import "testing"

// These tests are the acceptance criteria for sections 6 and 7: voting,
// certification, and the property the whole practical is about -- a double
// spend is never certified twice.

func testBalances() map[string]uint64 {
	return map[string]uint64{"alice": 100, "bob": 100, "carol": 100}
}

func newTestNetwork(size int, seed int64) *Network {
	return NewNetwork(size, testBalances(), seed)
}

// TestVoteOncePerAsset checks the two rules of the fast path directly on a
// validator, without any network: a validator votes once per transaction,
// and accepts at most one transaction per asset.
func TestVoteOncePerAsset(t *testing.T) {
	v := NewValidator(0, testCommittee(), testBalances())
	first := NewTx("alice", 0, "bob", 10)
	second := NewTx("alice", 0, "carol", 10) // conflicts with first
	other := NewTx("bob", 0, "alice", 10)    // independent

	votes := v.voteOn([]*Tx{first, second, other})
	if len(votes) != 3 {
		t.Fatalf("expected one vote per transaction, got %d", len(votes))
	}
	if votes[0].Tx != first.ID || !votes[0].Accept {
		t.Fatal("the first transaction seen for an asset must be accepted")
	}
	if votes[1].Tx != second.ID || votes[1].Accept {
		t.Fatal("a transaction conflicting with an accepted one must be rejected")
	}
	if votes[2].Tx != other.ID || !votes[2].Accept {
		t.Fatal("a transaction on another asset must be accepted")
	}

	// Voting again on the same transactions must not produce a second
	// opinion: the first vote is the only one.
	if again := v.voteOn([]*Tx{first, second, other}); len(again) != 0 {
		t.Fatalf("a validator votes at most once per transaction, got %d extra votes", len(again))
	}
}

// TestProposerVotesForItsOwnTransaction checks that a validator that
// proposes a transaction also states its opinion on it: the proposer is one
// of the voters of the quorum.
func TestProposerVotesForItsOwnTransaction(t *testing.T) {
	net := newTestNetwork(4, 1)
	tx := NewTx("alice", 0, "bob", 10)
	net.Submit(0, tx)
	net.Step()

	b := net.Validators[0].DAG().BlockOf(0, 1)
	if b == nil {
		t.Fatal("validator 0 should have built its round-1 block")
	}
	if len(b.Txs) != 1 || b.Txs[0].ID != tx.ID {
		t.Fatalf("the block should propose the submitted transaction, got %v", b.Txs)
	}
	found := false
	for _, v := range b.Votes {
		if v.Tx == tx.ID && v.Accept {
			found = true
		}
	}
	if !found {
		t.Fatal("the proposer must also vote for the transaction it proposes")
	}
}

// TestSingleTransferIsCertified is the happy path: one transfer, no
// conflict, and the whole committee certifies and executes it.
func TestSingleTransferIsCertified(t *testing.T) {
	net := newTestNetwork(4, 2)
	tx := NewTx("alice", 0, "bob", 10)
	net.Submit(0, tx)
	net.Run(20)

	for _, v := range net.Validators {
		if !v.IsCertified(tx.ID) {
			t.Fatalf("V%d did not certify %s", v.ID, tx)
		}
		if v.Balances["alice"] != 90 || v.Balances["bob"] != 110 {
			t.Fatalf("V%d has a wrong ledger: alice=%d bob=%d", v.ID, v.Balances["alice"], v.Balances["bob"])
		}
		if l, _ := v.Latency(tx.ID); l > 4 {
			t.Fatalf("V%d certified after %d rounds, the fast path should take 2 or 3", v.ID, l)
		}
	}
}

// TestCrashedValidatorDoesNotBlockTheFastPath checks that a committee of
// four keeps certifying when one validator is dead, since a quorum of three
// remains. Note that the transfers submitted to the dead validator are
// simply never proposed: the fast path is not censorship-resistant by
// itself.
func TestCrashedValidatorDoesNotBlockTheFastPath(t *testing.T) {
	net := newTestNetwork(4, 3)
	net.Crash(3)
	tx := NewTx("alice", 0, "bob", 10)
	net.Submit(0, tx)
	net.Run(30)

	for _, v := range net.Live() {
		if !v.IsCertified(tx.ID) {
			t.Fatalf("V%d did not certify %s with one validator down", v.ID, tx)
		}
	}
}

// TestDoubleSpendIsNeverCertifiedTwice is the safety property of the fast
// path. It must hold whatever the seed, i.e. whatever the order in which
// the validators discover the two conflicting transactions.
//
// [Task] Write this test:
//   - build a network of four validators;
//   - create two transactions of the same owner consuming the same asset
//     (same sequence number) with different recipients, and hand them to
//     two different validators;
//   - run the simulation long enough (30 steps is plenty);
//   - assert that no validator certified both of them, and that all the
//     validators agree on the outcome;
//   - if one of them was certified, assert that the owner was debited
//     exactly once.
//
// Try several seeds: sometimes one of the two transactions wins, sometimes
// neither does. Never both.
func TestDoubleSpendIsNeverCertifiedTwice(t *testing.T) {
	t.Skip("TODO: write this test (section 8)")
}

// TestBalancesConverge checks that validators that never agreed on an order
// nevertheless agree on the ledger.
//
// [Task] Write this test:
//   - build a network of four validators;
//   - create four transfers from different owners (watch the sequence
//     numbers: an owner's transfers are numbered 0, 1, ...), and hand each
//     of them to a different validator;
//   - run the simulation;
//   - assert that every validator certified every transfer, that they all
//     report the same balances, and that the total amount of money is
//     unchanged.
func TestBalancesConverge(t *testing.T) {
	t.Skip("TODO: write this test (section 8)")
}
