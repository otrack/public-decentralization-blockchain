package main

import (
	"fmt"
	"log"
)

// This is the fast path of Mysticeti, stripped to its bones.
//
// The life of a transaction has three stages:
//
//	proposal   a validator receives tx from its owner and puts it in its
//	           next block;
//	voting     every other validator sees tx in its causal history and,
//	           in the first block it produces after that, says accept
//	           (I had not accepted a conflicting transaction) or reject;
//	certified  as soon as a quorum of *distinct* validators have said
//	           accept, tx is final. Nobody had to agree on an order.
//
// Two rules make it work, and they are both two lines long:
//
//	(1) a validator votes at most once per transaction -- its first
//	    opinion is its only opinion;
//	(2) a validator accepts at most one transaction per asset -- seeing
//	    one locks the asset for good.
//
// Suppose two conflicting transactions were both certified. Each gathered
// accept votes from a quorum, and two quorums intersect in at least f+1
// validators, so at least one *honest* validator accepted both. That
// contradicts rule (2). Therefore, at most one of two conflicting
// transactions is ever certified -- at every validator, with no leader, no
// view change, no consensus.

// Validator runs the fast path. It keeps a DAG of blocks, its own opinions
// on transactions, and the ledger obtained by executing certified
// transfers.
type Validator struct {
	ID        int
	committee Committee
	dag       *DAG
	round     uint64 // the round of the next block this validator will build

	pending []*Tx // submitted by owners, not yet put in one of my blocks
	txs     map[TxID]*Tx
	seenAt  map[TxID]uint64 // round of the first block I saw carrying it

	voted  map[TxID]bool   // transactions I already voted on (rule 1)
	locked map[string]TxID // asset -> the transaction I accepted for it (rule 2)

	accepts     map[TxID]map[int]bool // transaction -> validators that accepted
	certified   map[TxID]bool
	CertifiedAt map[TxID]uint64 // round at which I observed certification

	Balances map[string]uint64
	nextSeq  map[string]uint64
	held     []*Tx // certified, waiting for the owner's earlier transfers

	Verbose bool
}

// NewValidator returns a validator with the given initial balances.
func NewValidator(id int, c Committee, balances map[string]uint64) *Validator {
	v := &Validator{
		ID:          id,
		committee:   c,
		dag:         NewDAG(c),
		round:       1,
		txs:         make(map[TxID]*Tx),
		seenAt:      make(map[TxID]uint64),
		voted:       make(map[TxID]bool),
		locked:      make(map[string]TxID),
		accepts:     make(map[TxID]map[int]bool),
		certified:   make(map[TxID]bool),
		CertifiedAt: make(map[TxID]uint64),
		Balances:    make(map[string]uint64),
		nextSeq:     make(map[string]uint64),
	}
	for account, amount := range balances {
		v.Balances[account] = amount
	}
	return v
}

// Submit is the client-facing entry point: the owner of the transaction
// hands it to this validator, which will propose it in its next block.
func (v *Validator) Submit(tx *Tx) {
	v.pending = append(v.pending, tx)
}

// Round returns the round of the next block this validator will build.
func (v *Validator) Round() uint64 { return v.round }

// DAG returns the block store, for tests and reporting.
func (v *Validator) DAG() *DAG { return v.dag }

// IsCertified reports whether the transaction is final for this validator.
func (v *Validator) IsCertified(id TxID) bool { return v.certified[id] }

// Certified returns the number of certified transactions.
func (v *Validator) Certified() int { return len(v.certified) }

// Latency returns the number of rounds between the block that proposed the
// transaction and the round at which this validator certified it.
func (v *Validator) Latency(id TxID) (uint64, bool) {
	if !v.certified[id] {
		return 0, false
	}
	return v.CertifiedAt[id] - v.seenAt[id], true
}

// selectParents returns the blocks of the previous round this validator
// will acknowledge, or false if it cannot build a block yet. Note that it
// keeps *one* block per author: an equivocating validator gets one parent
// slot, not two, so it can never help a block reach a quorum twice.
func (v *Validator) selectParents() ([]BlockID, bool) {
	byAuthor := make(map[int]BlockID)
	for _, b := range v.dag.BlocksAt(v.round - 1) {
		if _, dup := byAuthor[b.Author]; dup {
			continue
		}
		byAuthor[b.Author] = b.ID
	}
	if len(byAuthor) < v.committee.Quorum() {
		return nil, false // not enough support yet, wait for more blocks
	}
	if _, ok := byAuthor[v.ID]; !ok {
		return nil, false // I must chain my own previous block
	}
	parents := make([]BlockID, 0, len(byAuthor))
	for i := 0; i < v.committee.Size; i++ {
		if id, ok := byAuthor[i]; ok {
			parents = append(parents, id)
		}
	}
	return parents, true
}

// unvoted returns the transactions of the causal history of the given
// parents that this validator has not voted on yet, oldest block first.
func (v *Validator) unvoted(parents []BlockID) []*Tx {
	var out []*Tx
	seen := make(map[TxID]bool)
	for _, b := range v.dag.HistoryOf(parents) {
		for _, tx := range b.Txs {
			if v.voted[tx.ID] || seen[tx.ID] {
				continue
			}
			seen[tx.ID] = true
			out = append(out, tx)
		}
	}
	return out
}

// TryAdvance builds the next block of this validator, or returns nil if it
// does not have enough blocks of the previous round yet.
//
// [Task] Complete the body, once the parents are known:
//  1. take the pending transactions submitted by owners (and empty the
//     pending list -- they are about to be proposed);
//  2. vote, with voteOn, on those transactions *and* on the ones of the
//     causal history this validator has not voted on yet (helper unvoted).
//     Beware of the order: a validator must vote on its own proposals too,
//     and it should consider them after what it has already heard from
//     others;
//  3. assemble the block with NewBlock, move to the next round, hand the
//     block to Receive so that it enters the local DAG, and return it.
func (v *Validator) TryAdvance() *Block {
	parents, ok := v.selectParents()
	if !ok {
		return nil
	}
	_ = parents // TODO: propose, vote, assemble (section 6)
	return nil
}

// voteOn returns this validator's opinion on the given transactions, and
// records it so that it is never contradicted later. This is where the two
// rules of the fast path live.
//
// [Task] For each transaction, in order:
//   - if this validator already voted on it (rule 1), return no vote for
//     it: the result must not contain a second opinion on the same
//     transaction;
//   - otherwise mark it as voted, and look up its asset in v.locked;
//   - if the asset is already locked by a *different* transaction, this is
//     a double spend: vote reject;
//   - otherwise lock the asset with this transaction and vote accept.
func (v *Validator) voteOn(txs []*Tx) []Vote {
	return nil // TODO: vote once per transaction, accept once per asset (section 6)
}

// Receive inserts a block in the local DAG and counts the votes it carries.
// A block whose parents are missing is suspended by the DAG and counted
// later, when they arrive.
func (v *Validator) Receive(b *Block) error {
	added, err := v.dag.Insert(b)
	if err != nil {
		return err
	}
	for _, nb := range added {
		for _, tx := range nb.Txs {
			if _, ok := v.txs[tx.ID]; !ok {
				v.txs[tx.ID] = tx
				v.seenAt[tx.ID] = nb.Round
			}
		}
	}
	for _, nb := range added {
		v.tally(nb)
	}
	return nil
}

// tally counts the votes carried by a freshly inserted block and certifies
// the transactions that reached a quorum.
//
// [Task] For every accept vote of the block:
//   - record that the *author of the block* accepted that transaction (a
//     set per transaction, so that a validator that votes twice -- in two
//     equivocated blocks -- is still counted once);
//   - when the number of distinct accepting validators reaches
//     v.committee.Quorum() and the transaction is not certified yet, mark
//     it certified, remember the current round in v.CertifiedAt, and call
//     v.execute on it. The transaction v.txs[vote.Tx] may legitimately be
//     unknown only if the DAG is incomplete, so skip it in that case.
//
// Reject votes need no bookkeeping here: a transaction that does not gather
// a quorum of accepts is simply never certified.
func (v *Validator) tally(b *Block) {
	// TODO: count accept votes per transaction, certify at a quorum (section 7)
}

// execute applies a certified transfer to the local ledger.
//
// Certified transactions reach validators in different orders, and that is
// fine as long as each transfer is funded: it only debits its owner and
// credits its recipient, and additions commute. The one thing we do enforce
// is that an owner's own transfers are applied in sequence-number order; a
// transfer that arrives early waits in v.held.
//
// Known limit (see the question on funds in the README): the test on the
// balance below is made against the *local* ledger, so a transfer that spends
// money received through another owner's transfer may be skipped at one
// validator and applied at another. The specification asks the owner to
// decide, from its own history, and to name the transfers it relies on.
func (v *Validator) execute(tx *Tx) {
	v.held = append(v.held, tx)
	for progress := true; progress; {
		progress = false
		for i, held := range v.held {
			if held.Seq != v.nextSeq[held.Owner] {
				continue
			}
			v.held = append(v.held[:i], v.held[i+1:]...)
			v.nextSeq[held.Owner] = held.Seq + 1
			if v.Balances[held.Owner] >= held.Amount {
				v.Balances[held.Owner] -= held.Amount
				v.Balances[held.To] += held.Amount
			} else if v.Verbose {
				log.Printf("V%d: %s has insufficient funds, skipped", v.ID, held)
			}
			progress = true
			break
		}
	}
}

// LedgerString renders the balances in a stable order, for reporting.
func (v *Validator) LedgerString(accounts []string) string {
	s := ""
	for _, a := range accounts {
		s += fmt.Sprintf("%s=%d ", a, v.Balances[a])
	}
	return s
}
