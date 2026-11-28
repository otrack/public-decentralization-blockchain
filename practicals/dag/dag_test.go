package main

import "testing"

// These tests are the acceptance criteria for sections 4 and 5: the
// transaction identifiers, the block identifiers, and the validity rules of
// the DAG. Run them with
//
//	go test -run 'TestCommittee|TestGenesis|TestBlock|TestValid|TestParent|TestQuorum|TestMissing|TestCausal|TestEquivocation' -v

func testCommittee() Committee { return Committee{Size: 4} }

// allParents returns every block identifier of a round: the natural parent
// set when no validator is silent.
func allParents(d *DAG, round uint64) []BlockID {
	var out []BlockID
	for _, b := range d.BlocksAt(round) {
		out = append(out, b.ID)
	}
	return out
}

func TestCommitteeArithmetic(t *testing.T) {
	for _, c := range []struct{ size, f, quorum int }{
		{1, 0, 1}, {4, 1, 3}, {7, 2, 5}, {10, 3, 7},
	} {
		got := Committee{Size: c.size}
		if got.F() != c.f || got.Quorum() != c.quorum {
			t.Fatalf("size %d: expected f=%d quorum=%d, got f=%d quorum=%d",
				c.size, c.f, c.quorum, got.F(), got.Quorum())
		}
	}
}

func TestTxConflictKey(t *testing.T) {
	a := NewTx("alice", 3, "bob", 10)
	b := NewTx("alice", 3, "carol", 10) // same asset, other recipient
	c := NewTx("alice", 4, "bob", 10)   // next asset
	d := NewTx("bob", 3, "alice", 10)   // another owner

	if a.ID == b.ID {
		t.Fatal("two different transactions must have different identifiers")
	}
	if a.ConflictKey() != b.ConflictKey() {
		t.Fatalf("a double spend must share the conflict key: %q vs %q", a.ConflictKey(), b.ConflictKey())
	}
	if a.ConflictKey() == c.ConflictKey() || a.ConflictKey() == d.ConflictKey() {
		t.Fatal("transactions consuming different assets must have different conflict keys")
	}
	if a.ID != a.ComputeHash() {
		t.Fatal("the identifier must be the hash of the contents")
	}
}

func TestGenesisIsPreloaded(t *testing.T) {
	d := NewDAG(testCommittee())
	if n := len(d.BlocksAt(0)); n != 4 {
		t.Fatalf("expected 4 genesis blocks, got %d (if you got 1, ComputeID does not depend on the contents yet)", n)
	}
	if d.Size() != 4 {
		t.Fatalf("expected a DAG of 4 blocks, got %d", d.Size())
	}
}

func TestBlockIDCoversContents(t *testing.T) {
	d := NewDAG(testCommittee())
	p := allParents(d, 0)
	tx := NewTx("alice", 0, "bob", 1)

	base := NewBlock(0, 1, p, []*Tx{tx}, nil)
	same := NewBlock(0, 1, p, []*Tx{tx}, nil)
	if base.ID != same.ID {
		t.Fatal("two blocks with identical contents must have the same identifier")
	}
	for name, other := range map[string]*Block{
		"author": NewBlock(1, 1, p, []*Tx{tx}, nil),
		"round":  NewBlock(0, 2, p, []*Tx{tx}, nil),
		"txs":    NewBlock(0, 1, p, nil, nil),
		"votes":  NewBlock(0, 1, p, []*Tx{tx}, []Vote{{Tx: tx.ID, Accept: true}}),
		"parent": NewBlock(0, 1, p[:3], []*Tx{tx}, nil),
	} {
		if other.ID == base.ID {
			t.Fatalf("blocks differing by their %s must have different identifiers", name)
		}
	}
}

func TestValidGenesisHasNoParent(t *testing.T) {
	d := NewDAG(testCommittee())
	bad := NewBlock(0, 0, allParents(d, 0), nil, nil)
	if err := d.Add(bad); err != ErrBadGenesis {
		t.Fatalf("expected ErrBadGenesis, got %v", err)
	}
}

func TestQuorumOfParents(t *testing.T) {
	d := NewDAG(testCommittee())
	p := allParents(d, 0)

	if err := d.Add(NewBlock(0, 1, p[:2], nil, nil)); err != ErrWeakSupport {
		t.Fatalf("a block with 2 parents out of 4 must be rejected, got %v", err)
	}
	if err := d.Add(NewBlock(0, 1, p[:3], nil, nil)); err != nil {
		t.Fatalf("a block with a quorum of parents must be accepted, got %v", err)
	}
}

func TestOwnParentIsRequired(t *testing.T) {
	d := NewDAG(testCommittee())
	p := allParents(d, 0)
	// parents of authors 1, 2, 3: a quorum, but not the author's own block
	if err := d.Add(NewBlock(0, 1, p[1:], nil, nil)); err != ErrNoOwnParent {
		t.Fatalf("expected ErrNoOwnParent, got %v", err)
	}
}

func TestParentsAreAtThePreviousRound(t *testing.T) {
	d := NewDAG(testCommittee())
	p := allParents(d, 0)
	for i := 0; i < 4; i++ {
		if err := d.Add(NewBlock(i, 1, p, nil, nil)); err != nil {
			t.Fatalf("round-1 block of validator %d: %v", i, err)
		}
	}
	// a round-2 block pointing at the genesis blocks instead of round 1
	if err := d.Add(NewBlock(0, 2, p, nil, nil)); err != ErrBadParentRound {
		t.Fatalf("expected ErrBadParentRound, got %v", err)
	}
}

func TestMissingParentIsSuspended(t *testing.T) {
	d := NewDAG(testCommittee())
	p := allParents(d, 0)

	var round1 []*Block
	for i := 0; i < 4; i++ {
		round1 = append(round1, NewBlock(i, 1, p, nil, nil))
	}
	// everybody but validator 2 is inserted
	for i, b := range round1 {
		if i == 2 {
			continue
		}
		if _, err := d.Insert(b); err != nil {
			t.Fatal(err)
		}
	}

	top := NewBlock(0, 2, []BlockID{round1[0].ID, round1[1].ID, round1[2].ID}, nil, nil)
	if _, err := d.Insert(top); err != ErrMissingParents {
		t.Fatalf("expected ErrMissingParents, got %v", err)
	}
	if d.Suspended() != 1 {
		t.Fatalf("the block should be suspended, %d suspended", d.Suspended())
	}
	if d.Contains(top.ID) {
		t.Fatal("a block with a missing parent must not enter the DAG")
	}

	added, err := d.Insert(round1[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 {
		t.Fatalf("inserting the missing parent should also unblock the suspended block, got %d blocks", len(added))
	}
	if d.Suspended() != 0 || !d.Contains(top.ID) {
		t.Fatal("the suspended block should have entered the DAG")
	}
}

func TestCausalHistory(t *testing.T) {
	d := NewDAG(testCommittee())
	p := allParents(d, 0)
	for i := 0; i < 4; i++ {
		if err := d.Add(NewBlock(i, 1, p, nil, nil)); err != nil {
			t.Fatal(err)
		}
	}
	top := NewBlock(0, 2, allParents(d, 1)[:3], nil, nil)
	if err := d.Add(top); err != nil {
		t.Fatal(err)
	}

	history := d.CausalHistory(top)
	// 3 blocks of round 1, and the 4 genesis blocks below them
	if len(history) != 7 {
		t.Fatalf("expected 7 ancestors, got %d", len(history))
	}
	seen := make(map[BlockID]bool)
	var round uint64
	for _, b := range history {
		if seen[b.ID] {
			t.Fatalf("%s appears twice in the causal history", b)
		}
		seen[b.ID] = true
		if b.Round < round {
			t.Fatal("the causal history must be sorted, oldest round first")
		}
		round = b.Round
		if b.ID == top.ID {
			t.Fatal("the causal history must contain the strict ancestors only")
		}
	}
	if len(d.CausalHistory(d.BlocksAt(0)[0])) != 0 {
		t.Fatal("a genesis block has no ancestor")
	}
}

func TestEquivocationIsStoredAndFlagged(t *testing.T) {
	d := NewDAG(testCommittee())
	p := allParents(d, 0)
	one := NewBlock(0, 1, p, []*Tx{NewTx("alice", 0, "bob", 1)}, nil)
	two := NewBlock(0, 1, p, []*Tx{NewTx("alice", 0, "carol", 1)}, nil)

	if err := d.Add(one); err != nil {
		t.Fatal(err)
	}
	if err := d.Add(two); err != nil {
		t.Fatalf("an equivocating block is stored like any other, got %v", err)
	}
	if !d.Equivocators[0] {
		t.Fatal("validator 0 should have been flagged as an equivocator")
	}
	if !d.Contains(one.ID) || !d.Contains(two.ID) {
		t.Fatal("both equivocating blocks must be in the DAG")
	}
}
