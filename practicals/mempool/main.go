package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"time"
)

// main demonstrates the whole picture. It starts a small network of nodes
// on 127.0.0.1, all running the same bank application from the same
// genesis, and wires them together. Clients then submit transfers to
// different nodes, and flooding propagates the valid ones everywhere. One
// node proposes a block made of its pending transactions, and every node
// commits it: as if the network had decided that block by consensus.
func main() {
	var nodes int
	flag.IntVar(&nodes, "nodes", 5, "number of nodes in the network")
	flag.Parse()
	if nodes < 2 {
		log.Fatal("at least 2 nodes")
	}

	log.SetFlags(log.Lmicroseconds)

	genesis := map[string]uint64{"alice": 100, "bob": 50}
	nodesList := make([]*Node, 0, nodes)
	banks := make([]*Bank, 0, nodes)
	base := 8000
	for i := 0; i < nodes; i++ {
		bank := NewBank(genesis)
		n := NewNode(fmt.Sprintf("127.0.0.1:%d", base+i), bank)
		if err := n.Listen(); err != nil {
			log.Fatal(err)
		}
		nodesList = append(nodesList, n)
		banks = append(banks, bank)
	}

	// Fully-connected topology: every node is a peer of every other.
	// A ring or a random graph would be more realistic — try it later.
	for _, n := range nodesList {
		for _, m := range nodesList {
			if n != m {
				n.AddPeer(m.addr)
			}
		}
	}

	// Wait for the listeners to be ready.
	time.Sleep(300 * time.Millisecond)
	first, last := nodesList[0], nodesList[nodes-1]

	step := func(format string, args ...any) {
		time.Sleep(300 * time.Millisecond) // let the gossip settle
		fmt.Printf("\n== "+format+"\n", args...)
	}

	step("bob pays 20 to carol, at %s", first.addr)
	first.Relay(NewTransfer("bob", "carol", 20))

	step("carol pays 30 to dave, at %s: carol holds only 20", nodesList[1].addr)
	nodesList[1].Relay(NewTransfer("carol", "dave", 30))

	step("alice spends her 100 twice, at the same time: 70 to bob at %s, 70 to carol at %s",
		first.addr, last.addr)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); first.Relay(NewTransfer("alice", "bob", 70)) }()
	go func() { defer wg.Done(); last.Relay(NewTransfer("alice", "carol", 70)) }()
	wg.Wait()

	step("the pools before the block")
	for _, n := range nodesList {
		fmt.Printf("%s: %s\n", n.addr, n.pool)
		for _, tx := range n.pool.Reap(-1) {
			fmt.Printf("    %s\n", describe(tx))
		}
	}

	proposer := nodesList[nodes/2]
	block := proposer.pool.Reap(-1)
	step("%s proposes a block of %d transactions; meanwhile, bob pays 10 to dave at %s",
		proposer.addr, len(block), first.addr)
	first.Relay(NewTransfer("bob", "dave", 10))
	for _, n := range nodesList {
		n.Commit(block)
	}

	step("the pools after the block")
	for i, n := range nodesList {
		fmt.Printf("%s: %s, balances alice=%d bob=%d carol=%d dave=%d\n", n.addr, n.pool,
			banks[i].Balance("alice"), banks[i].Balance("bob"),
			banks[i].Balance("carol"), banks[i].Balance("dave"))
	}
}
