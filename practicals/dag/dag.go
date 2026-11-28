package main

import (
	"errors"
	"sort"
)

// Errors returned when a block is not well-formed.
var (
	ErrKnown          = errors.New("block already known")
	ErrBadAuthor      = errors.New("unknown block author")
	ErrBadID          = errors.New("block identifier does not match its contents")
	ErrBadGenesis     = errors.New("a round-0 block must have no parent")
	ErrBadParentRound = errors.New("a parent is not at the previous round")
	ErrWeakSupport    = errors.New("fewer than a quorum of distinct authors among the parents")
	ErrNoOwnParent    = errors.New("a block must extend its author's own previous block")
	ErrMissingParents = errors.New("some parents are unknown")
)

// Committee is the (fixed) set of validators. Validators are numbered
// 0..Size-1 and all have the same weight.
type Committee struct {
	Size int
}

// F is the number of Byzantine validators the committee tolerates, the
// largest f such that Size >= 3f+1.
func (c Committee) F() int { return (c.Size - 1) / 3 }

// Quorum is the size of a quorum, Size-F, i.e. 2f+1 when Size is 3f+1.
// Two quorums always intersect in at least f+1 validators, hence in at
// least one honest validator. Keep this sentence in mind: it is the only
// thing that makes the fast path safe.
func (c Committee) Quorum() int { return c.Size - c.F() }

// DAG stores the blocks a validator knows about, and nothing else: it has
// no opinion on transactions. It is not safe for concurrent use -- the
// whole simulation runs in a single goroutine, unlike the mempool of
// session 2.
type DAG struct {
	committee Committee
	blocks    map[BlockID]*Block
	index     map[int]map[uint64][]BlockID // author -> round -> blocks
	byRound   map[uint64][]BlockID
	suspended map[BlockID]*Block // blocks waiting for a missing parent

	// Equivocators are the authors caught emitting two distinct blocks in
	// the same round. We record them, but we do not drop their blocks:
	// see the comment on Insert.
	Equivocators map[int]bool
}

// NewDAG returns a DAG pre-loaded with the genesis block of every
// validator, so that round-1 blocks have something to point at.
func NewDAG(c Committee) *DAG {
	d := &DAG{
		committee:    c,
		blocks:       make(map[BlockID]*Block),
		index:        make(map[int]map[uint64][]BlockID),
		byRound:      make(map[uint64][]BlockID),
		suspended:    make(map[BlockID]*Block),
		Equivocators: make(map[int]bool),
	}
	for i := 0; i < c.Size; i++ {
		d.store(GenesisBlock(i))
	}
	return d
}

// Size returns the number of blocks in the DAG.
func (d *DAG) Size() int { return len(d.blocks) }

// Get returns a block by identifier, or nil.
func (d *DAG) Get(id BlockID) *Block { return d.blocks[id] }

// Contains reports whether the block is known.
func (d *DAG) Contains(id BlockID) bool { _, ok := d.blocks[id]; return ok }

// MissingParents returns the parents of b that are not (yet) known.
func (d *DAG) MissingParents(b *Block) []BlockID {
	var missing []BlockID
	for _, id := range b.Parents {
		if !d.Contains(id) {
			missing = append(missing, id)
		}
	}
	return missing
}

// BlocksAt returns the blocks of a given round, in a deterministic order.
func (d *DAG) BlocksAt(round uint64) []*Block {
	out := make([]*Block, 0, len(d.byRound[round]))
	for _, id := range d.byRound[round] {
		out = append(out, d.blocks[id])
	}
	sortBlocks(out)
	return out
}

// BlockOf returns the first block known from an author at a round, or nil.
func (d *DAG) BlockOf(author int, round uint64) *Block {
	ids := d.index[author][round]
	if len(ids) == 0 {
		return nil
	}
	return d.blocks[ids[0]]
}

// store records a block and updates the indexes. It assumes the block has
// been validated.
func (d *DAG) store(b *Block) {
	d.blocks[b.ID] = b
	if d.index[b.Author] == nil {
		d.index[b.Author] = make(map[uint64][]BlockID)
	}
	if siblings := d.index[b.Author][b.Round]; len(siblings) > 0 {
		d.Equivocators[b.Author] = true
	}
	d.index[b.Author][b.Round] = append(d.index[b.Author][b.Round], b.ID)
	d.byRound[b.Round] = append(d.byRound[b.Round], b.ID)
}

// Add validates a block and inserts it in the DAG.
//
// The first four checks are provided. Note their order: the parents are
// known by the time the structural rules run, so you can dereference them
// without checking for nil.
//
// [Task] Complete the structural rules of a valid block. The skeleton
// already checks that every parent of a block of round r sits at round r-1
// (ErrBadParentRound). Write the other three:
//   - a round-0 block has no parent at all (ErrBadGenesis);
//   - the parents are authored by at least a quorum of *distinct*
//     validators (ErrWeakSupport);
//   - one of the parents is authored by the block's own author, i.e. a
//     validator chains its own blocks (ErrNoOwnParent).
func (d *DAG) Add(b *Block) error {
	if d.Contains(b.ID) {
		return ErrKnown
	}
	if b.Author < 0 || b.Author >= d.committee.Size {
		return ErrBadAuthor
	}
	if b.ID != b.ComputeID() {
		return ErrBadID
	}
	if len(d.MissingParents(b)) > 0 {
		return ErrMissingParents
	}
	if b.Round == 0 {
		// TODO: a genesis block has no parent (section 5)
	} else {
		authors := make(map[int]bool) // the distinct authors of the parents
		for _, id := range b.Parents {
			p := d.Get(id)
			if p.Round != b.Round-1 {
				return ErrBadParentRound
			}
			authors[p.Author] = true
		}
		// TODO: a quorum of distinct authors, one of which is b.Author (section 5)
	}
	d.store(b)
	return nil
}

// Insert is what validators call on reception. It adds b, then retries the
// blocks that were waiting for a missing parent, until no more progress is
// made. It returns every block that entered the DAG, oldest first.
//
// A block whose parents are unknown is *suspended*, not dropped: the DAG
// must stay causally complete, otherwise a validator could vote on a
// transaction while ignoring a conflicting one that sits below it.
//
// Note what Insert does *not* do: reject a second block from an author that
// already emitted one in that round. Equivocated blocks are stored like
// any other. Safety does not come from forbidding equivocation -- which no
// validator can do unilaterally anyway -- but from counting distinct
// authors, both when choosing parents and when counting votes.
func (d *DAG) Insert(b *Block) ([]*Block, error) {
	if err := d.Add(b); err != nil {
		if err == ErrMissingParents {
			d.suspended[b.ID] = b
		}
		return nil, err
	}
	added := []*Block{b}
	for progress := true; progress; {
		progress = false
		for id, s := range d.suspended {
			if err := d.Add(s); err == nil {
				delete(d.suspended, id)
				added = append(added, s)
				progress = true
			} else if err != ErrMissingParents {
				delete(d.suspended, id) // permanently invalid, forget it
			}
		}
	}
	return added, nil
}

// Suspended returns the number of blocks waiting for a missing parent.
func (d *DAG) Suspended() int { return len(d.suspended) }

// CausalHistory returns all the *strict* ancestors of b, i.e. every block
// reachable from its parents, in a deterministic order.
//
// This set is the reason the DAG can carry votes for free: by referencing
// its parents, a block implicitly says "I have seen all of this".
//
// [Task] Implement the traversal. Beware of two things: the DAG is not a
// tree, so the same block is reachable by many paths and must be visited
// once; and the result must be sorted with sortBlocks, so that two
// validators observing the same blocks process them in the same order.
// The README has a step-by-step help.
func (d *DAG) CausalHistory(b *Block) []*Block {
	return nil // TODO: collect every ancestor of b, once (section 5)
}

// HistoryOf returns the given blocks together with all their ancestors,
// oldest first. This is the causal history of a block that would have the
// given blocks as parents.
func (d *DAG) HistoryOf(ids []BlockID) []*Block {
	out := []*Block{}
	seen := make(map[BlockID]bool)
	for _, id := range ids {
		b := d.Get(id)
		if b == nil {
			continue
		}
		if !seen[b.ID] {
			seen[b.ID] = true
			out = append(out, b)
		}
		for _, a := range d.CausalHistory(b) {
			if !seen[a.ID] {
				seen[a.ID] = true
				out = append(out, a)
			}
		}
	}
	sortBlocks(out)
	return out
}

// sortBlocks orders blocks by round, then author, then identifier. Any
// total order would do, as long as every validator uses the same one.
func sortBlocks(bs []*Block) {
	sort.Slice(bs, func(i, j int) bool {
		if bs[i].Round != bs[j].Round {
			return bs[i].Round < bs[j].Round
		}
		if bs[i].Author != bs[j].Author {
			return bs[i].Author < bs[j].Author
		}
		return bs[i].ID < bs[j].ID
	})
}
