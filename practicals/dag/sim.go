package main

import (
	"log"
	"math/rand"
)

// A real validator talks to the others over the network -- you wrote such
// a flooding layer in the mempool practical of session 2. Here we replace
// it by a simulator: all the validators live in the same process, in a
// single goroutine, and messages are delivered after a random delay of one
// or two steps. Nothing in dag.go or validator.go knows about this.
//
// A *step* of the simulation is: deliver whatever is due, then let every
// validator try to build its next block. A validator builds the block of
// round r as soon as it holds blocks of round r-1 from a quorum of authors
// (including itself), so validators are never in lockstep: some run one or
// two rounds ahead of the others, exactly as in a real deployment.

type message struct {
	block *Block
	to    int
	at    int // step at which it is delivered
}

// Network is the simulated deployment.
type Network struct {
	Committee  Committee
	Validators []*Validator

	Delay       int // maximum delivery delay, in steps (>= 1)
	Equivocator int // validator that emits two blocks per round, -1 for none
	Crashed     map[int]bool
	Verbose     bool

	rng   *rand.Rand
	queue []message
	step  int
}

// NewNetwork builds a committee of size validators, each with the given
// initial balances.
func NewNetwork(size int, balances map[string]uint64, seed int64) *Network {
	c := Committee{Size: size}
	n := &Network{
		Committee:   c,
		Delay:       2,
		Equivocator: -1,
		Crashed:     make(map[int]bool),
		rng:         rand.New(rand.NewSource(seed)),
	}
	for i := 0; i < size; i++ {
		n.Validators = append(n.Validators, NewValidator(i, c, balances))
	}
	return n
}

// SetVerbose turns on logging at every validator.
func (n *Network) SetVerbose(v bool) {
	n.Verbose = v
	for _, val := range n.Validators {
		val.Verbose = v
	}
}

// Crash stops a validator: it no longer builds blocks, and no longer
// receives any. The fast path must keep working as long as no more than F
// validators are crashed.
func (n *Network) Crash(i int) { n.Crashed[i] = true }

// Submit hands a transaction to one validator, as its owner would.
func (n *Network) Submit(to int, tx *Tx) { n.Validators[to].Submit(tx) }

// Step runs one step of the simulation.
func (n *Network) Step() {
	n.step++

	var later []message
	for _, m := range n.queue {
		if m.at > n.step {
			later = append(later, m)
			continue
		}
		if n.Crashed[m.to] {
			continue
		}
		if err := n.Validators[m.to].Receive(m.block); err != nil && n.Verbose {
			log.Printf("V%d: %s not inserted: %v", m.to, m.block, err)
		}
	}
	n.queue = later

	for i, v := range n.Validators {
		if n.Crashed[i] {
			continue
		}
		b := v.TryAdvance()
		if b == nil {
			continue
		}
		n.broadcast(i, b)
		if i == n.Equivocator {
			// Same author, same round, different contents: an
			// equivocation, sent to everybody (including its author,
			// so that the DAG stays causally complete) so that it is
			// blatant.
			bogus := NewTx("byzantine", b.Round, "byzantine", 0)
			twin := NewBlock(b.Author, b.Round, b.Parents, []*Tx{bogus}, nil)
			v.Receive(twin)
			n.broadcast(i, twin)
		}
	}
}

// Run executes the given number of steps.
func (n *Network) Run(steps int) {
	for i := 0; i < steps; i++ {
		n.Step()
	}
}

// broadcast schedules a block for delivery at every other validator.
func (n *Network) broadcast(from int, b *Block) {
	for j := range n.Validators {
		if j == from {
			continue // the author inserted its own block already
		}
		n.queue = append(n.queue, message{block: b, to: j, at: n.step + 1 + n.rng.Intn(n.Delay)})
	}
}

// Live returns the validators that have not crashed.
func (n *Network) Live() []*Validator {
	var out []*Validator
	for i, v := range n.Validators {
		if !n.Crashed[i] {
			out = append(out, v)
		}
	}
	return out
}
