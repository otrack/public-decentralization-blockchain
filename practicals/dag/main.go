package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
)

// main runs a small deployment of the fast path: a committee of validators,
// a handful of owners transferring money to each other, and -- unless you
// disable it -- one owner trying to spend the same asset twice.
//
//	go run .
//	go run . -v                     (see the votes and the certifications)
//	go run . -seed=7 -steps=60
//	go run . -crash=2               (one validator is dead)
//	go run . -byzantine             (one validator equivocates)
func main() {
	var validators, txs, steps, delay, crash int
	var seed int64
	var double, verbose, byzantine bool
	flag.IntVar(&validators, "validators", 4, "committee size")
	flag.IntVar(&txs, "txs", 8, "number of honest transfers")
	flag.IntVar(&steps, "steps", 40, "number of simulation steps")
	flag.IntVar(&delay, "delay", 2, "maximum message delay, in steps")
	flag.IntVar(&crash, "crash", -1, "index of a validator to crash, -1 for none")
	flag.Int64Var(&seed, "seed", 1, "rng seed")
	flag.BoolVar(&double, "double", true, "submit a double spend")
	flag.BoolVar(&byzantine, "byzantine", false, "make the last validator equivocate")
	flag.BoolVar(&verbose, "v", false, "log votes and certifications")
	flag.Parse()

	log.SetFlags(0)

	accounts := []string{"alice", "bob", "carol", "dave"}
	balances := make(map[string]uint64)
	for _, a := range accounts {
		balances[a] = 100
	}

	net := NewNetwork(validators, balances, seed)
	net.Delay = delay
	net.SetVerbose(verbose)
	if crash >= 0 && crash < validators {
		net.Crash(crash)
	}
	if byzantine {
		net.Equivocator = validators - 1
	}

	c := net.Committee
	fmt.Printf("committee of %d validators, f=%d, quorum=%d\n", c.Size, c.F(), c.Quorum())

	// Schedule the transfers: transaction i is handed to validator i mod n
	// at step i+1, as its owner would.
	type submission struct {
		at, to int
		tx     *Tx
	}
	var subs []submission
	var honest []*Tx
	seq := make(map[string]uint64)
	for i := 0; i < txs; i++ {
		owner := accounts[i%len(accounts)]
		to := accounts[(i+1)%len(accounts)]
		tx := NewTx(owner, seq[owner], to, 10)
		seq[owner]++
		honest = append(honest, tx)
		subs = append(subs, submission{at: i + 1, to: i % validators, tx: tx})
	}

	// The double spend: two transactions of the same owner consuming the
	// same asset (same sequence number), handed to two different
	// validators at the same time. At most one of them may ever be
	// certified -- that is the property the fast path must guarantee.
	var spendA, spendB *Tx
	if double && validators > 1 {
		owner := accounts[0]
		spendA = NewTx(owner, seq[owner], accounts[1], 25)
		spendB = NewTx(owner, seq[owner], accounts[2], 25)
		seq[owner]++
		subs = append(subs, submission{at: 3, to: 0, tx: spendA})
		subs = append(subs, submission{at: 3, to: 1, tx: spendB})
		fmt.Printf("double spend on asset %s: %s and %s\n", spendA.ConflictKey(), spendA.ID.Short(), spendB.ID.Short())
	}

	for step := 1; step <= steps; step++ {
		for _, s := range subs {
			if s.at == step {
				net.Submit(s.to, s.tx)
			}
		}
		net.Step()
	}

	// Report. Every live validator should agree on everything that
	// matters: the certified transactions and the resulting ledger.
	fmt.Println()
	for i, v := range net.Validators {
		if net.Crashed[i] {
			fmt.Printf("V%d crashed\n", i)
			continue
		}
		fmt.Printf("V%d round=%2d blocks=%3d suspended=%d certified=%2d  %s\n",
			i, v.Round(), v.DAG().Size(), v.DAG().Suspended(), v.Certified(), v.LedgerString(accounts))
	}

	ref := net.Live()[0]
	agree := true
	for _, v := range net.Live() {
		for _, a := range accounts {
			if v.Balances[a] != ref.Balances[a] {
				agree = false
			}
		}
		for _, tx := range honest {
			if v.IsCertified(tx.ID) != ref.IsCertified(tx.ID) {
				agree = false
			}
		}
	}
	fmt.Printf("\nledgers and certifications agree at every live validator: %v\n", agree)

	certified := 0
	var lat []uint64
	for _, tx := range honest {
		if ref.IsCertified(tx.ID) {
			certified++
			if l, ok := ref.Latency(tx.ID); ok {
				lat = append(lat, l)
			}
		}
	}
	fmt.Printf("honest transfers certified: %d/%d\n", certified, len(honest))
	if len(lat) > 0 {
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		var sum uint64
		for _, l := range lat {
			sum += l
		}
		fmt.Printf("fast-path latency: min=%d rounds, median=%d, max=%d (no consensus involved)\n",
			lat[0], lat[len(lat)/2], lat[len(lat)-1])
	}

	if spendA != nil {
		a, b := 0, 0
		for _, v := range net.Live() {
			if v.IsCertified(spendA.ID) {
				a++
			}
			if v.IsCertified(spendB.ID) {
				b++
			}
		}
		fmt.Printf("double spend: %s certified at %d/%d validators, %s at %d/%d\n",
			spendA.ID.Short(), a, len(net.Live()), spendB.ID.Short(), b, len(net.Live()))
		switch {
		case a > 0 && b > 0:
			fmt.Println("SAFETY VIOLATION: both spends were certified")
		case a == 0 && b == 0:
			fmt.Println("no spend was certified: the asset stays locked, and only its owner is to blame")
		default:
			fmt.Println("exactly one spend was certified, as expected")
		}
	}

	var eq []int
	for i := range ref.DAG().Equivocators {
		eq = append(eq, i)
	}
	sort.Ints(eq)
	if len(eq) > 0 {
		fmt.Printf("equivocating validators detected: %v\n", eq)
	}
}
