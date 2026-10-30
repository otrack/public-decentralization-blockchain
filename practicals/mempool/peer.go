package main

import (
	"errors"
	"log"
	"net"
	"sync"
)

// A mempool is only interesting if it is filled by the rest of the
// network. Transactions must propagate from the node that received them
// to every other node, ideally within seconds: this is *data
// dissemination*.
//
// We implement the simplest gossip strategy, called **flooding**: every
// node that accepts a transaction in its pool relays it to *all* its
// peers. Because a transaction enters a pool at most once (the map of the
// pool), the flood is self-terminating: a node never relays the same
// transaction twice. Because a transaction enters a pool only if it is
// valid, an invalid transaction is never relayed.
//
// The protocol is as simple as it gets: to send a transaction to a peer, a
// node opens a TCP connection, writes the bytes of the transaction (a
// Transfer, encoded with Protocol Buffers), and closes the connection. The
// receiver reads everything until the connection is closed: these are the
// bytes of the transaction. There is no encryption and no authentication
// here.

// Node is a single participant in the network.
type Node struct {
	addr  string     // own TCP address (host:port), used for logging
	pool  *Pool      // the mempool this node maintains
	mu    sync.Mutex // guards peers
	peers map[string]bool
}

// NewNode creates a node listening on addr, with a fresh mempool whose
// transactions are validated by app.
func NewNode(addr string, app Application) *Node {
	return &Node{
		addr:  addr,
		pool:  NewPool(app),
		peers: make(map[string]bool),
	}
}

// AddPeer records a remote address as a neighbor.
func (n *Node) AddPeer(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.peers[addr] = true
}

// Listen starts accepting incoming connections in a background goroutine
// and hands each connection to handle.
//
// [Task] Complete the accept loop: spawn a goroutine that runs
//
//	n.handle(conn) on each accepted connection.
func (n *Node) Listen() error {
	ln, err := net.Listen("tcp", n.addr)
	if err != nil {
		return err
	}
	n.addr = ln.Addr().String()
	log.Printf("%s: listening", n.addr)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("%s: accept: %v", n.addr, err)
				continue
			}
			_ = conn // TODO: go n.handle(conn) (section 8)
		}
	}()
	return nil
}

// handle processes one incoming connection: it reads all the bytes the
// peer sends, until the peer closes the connection, and relays them as a
// transaction.
//
// [Task] Implement the body: read the whole connection with io.ReadAll;
//
//	on error, log it and return; otherwise relay the bytes with n.Relay.
func (n *Node) handle(conn net.Conn) {
	defer conn.Close()
	// TODO: read the whole connection with io.ReadAll, then n.Relay the bytes (section 8)
}

// Broadcast sends a transaction to every peer, each on its own TCP
// connection. A failure with one peer (down, slow, ...) must not prevent
// the others from being reached: log the error and keep looping.
//
// [Task] Implement the loop: dial addr (net.Dial), write tx, close the
//
//	connection.
func (n *Node) Broadcast(tx Tx) {
	for _, addr := range n.peerAddrs() {
		_ = addr // TODO: dial addr, write tx, close; keep going on errors (section 8)
	}
}

// peerAddrs returns a copy of the current set of peers, so that we
// don't hold the lock while doing slow network I/O.
func (n *Node) peerAddrs() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, 0, len(n.peers))
	for addr := range n.peers {
		out = append(out, addr)
	}
	return out
}

// Relay inserts a transaction, received from a peer or from a client, in
// the pool, and re-emits it to all peers if it was new and valid
// (flooding). It returns the error of Insert: ErrTxInPool for a duplicate
// (which is *not* relayed: this is what stops the flood), ErrInvalidTx
// for an invalid transaction (not relayed either).
func (n *Node) Relay(tx Tx) error {
	err := n.pool.Insert(tx)
	if errors.Is(err, ErrTxInPool) {
		return err
	}
	if err != nil {
		log.Printf("%s: rejected %s", n.addr, describe(tx))
		return err
	}
	log.Printf("%s: accepted %s, pool=%s", n.addr, describe(tx), n.pool)
	n.Broadcast(tx)
	return nil
}

// Commit is called once the network has decided a block: the application
// executes it, and the pool forgets its transactions and rechecks the
// others.
func (n *Node) Commit(block []Tx) {
	n.pool.Update(block)
}
