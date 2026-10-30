# The waiting room of a blockchain [3h]

**Outline.** You will implement a simplified mempool, the component of a blockchain node that holds the transactions which have been broadcast to the network but are not yet in a block.
Its design follows the mempool of [CometBFT](https://docs.cometbft.com/latest/core/mempool), the consensus engine of Cosmos:
a hash map to deduplicate the transactions, 
a list that keeps them in their order of arrival, and 
a question to the *application* before any transaction gets in: is it valid in the current state?
You will write it in Go, together with a small application (a bank), a flooding protocol that disseminates the transactions between nodes over TCP, with messages encoded in Protocol Buffers, and the update of the pool once a block is decided.
The mempool is the practical counterpart of the lecture on *data dissemination*.

You will have to complete all the items marked by **[Task]**.
Items marked **[Opt]** are optional, and questions marked **[Q]** are to be answered in the report.

## 1. Context [15']

A blockchain is a *replicated state machine*: every node must (eventually) apply the same transactions, in the same order, to its local copy of the state.
Where do these transactions come from?
Users create them constantly, and send them to any node.
Transactions are **not** put straight into a block.
Instead, each node keeps them in a staging area called the **memory pool** (or *mempool*), until a block proposer picks some of them to form the next block.

The mempool has three jobs:

- **Dissemination.** A transaction sent to one node must eventually reach every node, so that whoever compute the next block can include it.
- **Filtering.** Anybody can send anything to a node.
  A transaction that cannot be executed (a malformed one or one whose sender cannot pay) must not take room in the pool. 
  Otherwise, a single client could flood the whole network for free, wasting resources.
  So, before a transaction enters the pool, the node asks the application whether it is valid *in the current state*.
- **Selection.** The block proposer takes valid transactions from the pool and add them to the next block.
  In Bitcoin or Ethereum, it takes those that pay the highest fees; CometBFT, by default, takes them first come, first served, and so do we: 
  there are no fees in this practical.

Lecture 2 (slide *When is a transaction valid?*) explains what valid means in Ethereum: among other things, the sender must be able to pay, that is, its balance must cover the value sent plus the maximum gas fee.
Our application keeps only this rule, without gas: a transfer is valid when its sender holds the amount.

The state changes with every block, so a transaction that was valid when it entered the pool may no longer be valid after the next block.
For instance, Alice holds 100 coins, and sends 70 to Bob through one node and, at the same time, 70 to Carol through another node.
Each node accepts whichever of the two transactions it hears first, and rejects the other one, as Alice holds only 30 after the first: 
alone, the mempools cannot agree on which transfer wins.
The next block decides; then every node *rechecks* its pool against the new state, and drops the transfer that lost.

## 2. Getting started [15']

Install Go (>= 1.21) following the official instructions, <https://go.dev/dl/>, then check the installation and run the tests:

```sh
go version
go test
```

The first run downloads the Protocol Buffers library for Go, `google.golang.org/protobuf`, which the file `go.mod` lists as a dependency (and `go.sum` pins, with its hash).
Everything should then *compile*, and the tests should fail.
Your job is to fill the `// TODO` gaps, in the order of the sections below, until they all pass.

`go test` is the test runner of Go.
It compiles the package, together with its files whose name ends in `_test.go` (here, `pool_test.go`), and runs every function whose name starts with `Test`, such as `TestIsValid`.
A test fails when it calls `t.Error` or `t.Fatal`.
Three options are used throughout this practical:

- `-v` (verbose) prints the name and the outcome of every test, `PASS` or `FAIL`, together with the messages of the failing ones; without it, `go test` prints only the failures and a one-line summary;
- `-run 'TestA|TestB'` runs only the tests whose name matches the given regular expression (here, `TestA` or `TestB`), so that each section checks the functions it asks you to write and nothing else;
- *(NEW)* `-race` enables the *race detector*: it watches the memory accesses of the program while the tests run, and makes a test fail if two goroutines (Go's lightweight threads) access the same variable at the same time, one of them writing, without a lock to order them.

## 3. Tour of the provided code [15']

	mempool.proto   the message of a transaction, in Protocol Buffers: a Transfer
	mempool.pb.go   the Go code generated from mempool.proto (do not edit it)
	tx.go           a transaction: opaque bytes, and its key
	app.go          the application: a bank, which validates and executes transfers
	pool.go         the pool: the dedup map, the FIFO list, the validity check, update
	peer.go         the node: TCP flooding so that nodes share transactions
	main.go         a demo: 5 nodes, a few transfers, a double spend, a block
	pool_test.go    the acceptance tests
	go.mod, go.sum  module metadata, and the dependency on Protocol Buffers

Read the opening comment of `pool.go` to understand how the pool is organized.
Then, start reading `tx.go` and move to the next section once it is done.

## 4. Transactions are bytes [25']

For the mempool, a transaction is an opaque sequence of bytes, `type Tx []byte`, and its *key* is the SHA-256 digest of these bytes (`Tx.Key`, in `tx.go`).
The mempool never looks inside a transaction.
Only the application knows what the bytes mean.

Our application is the running example of [Lecture 1](../../lectures/lecture1.pdf), a bank whose transactions are transfers.
A transfer is described in the file `mempool.proto`, in the language of **Protocol Buffers** (protobuf, for short).
Protobuf is a format to encode structured data in a compact, language-neutral binary form.
In some sense, it is similar to JSON.
Below, there is the protobuf declaration of a transfer for our banking application.
It includes the sender of the transfer (`from`), its destination (`to`), and the amount being transferred.

```proto
message Transfer {
  string from = 1;
  string to = 2;
  uint64 amount = 3;
}
```

In protobuf, each field has a type, a name, and a *number*: the encoding writes the number of a field, not its name, followed by its value.
The compiler `protoc` turns this description into Go code, the file `mempool.pb.go`: a struct `Transfer`, with the fields `From`, `To` and `Amount`.
The library `google.golang.org/protobuf/proto` then encodes such a struct into bytes, and decodes it back.
Protobuf is a communication standard in many distributed systems, from cloud infrastructure to blockchain.

**[Task]** Implement `NewTransfer` and `DecodeTransfer` in `tx.go`.

<details>
<summary><b>help me</b></summary>

**Encoding.** The function [`proto.Marshal`](https://pkg.go.dev/google.golang.org/protobuf/proto#Marshal) takes a *pointer* to a message and returns its encoding, and an error:

```go
data, err := proto.Marshal(m)
```

To build the message, write a [composite literal](https://go.dev/ref/spec#Composite_literals), the struct type followed by the values of its fields, and take its address with [`&`](https://go.dev/ref/spec#Address_operators):

```go
m := &Transfer{From: from, To: to, Amount: amount}
```

The names and the Go types of the fields of `Transfer` follow the rules of the generated code: a field `amount` of type `uint64` becomes a field `Amount` of type `uint64` (see [singular fields](https://protobuf.dev/reference/go/go-generated/#singular-implicit)).
If `err` is not `nil`, call `panic(err)`; otherwise return `data`.
A `[]byte` can be returned where a `Tx` is expected, since `Tx` is defined as a `[]byte`.

**Decoding.** The function [`proto.Unmarshal`](https://pkg.go.dev/google.golang.org/protobuf/proto#Unmarshal) does the converse: it takes the bytes and a pointer to an (empty) message, which it fills.
Declare a `Transfer` with `var t Transfer`, pass `&t` to `proto.Unmarshal`, and return `&t` if the returned error is `nil`.
Return the error otherwise.

</details>

At any time, you can validate `NewTransfer` and `DecodeTransfer` by running the following command:

```sh
go test -run 'TestTransferRoundTrip|TestKey' -v
```

## 5. The application: is a transaction valid? [20']

The bank, in `app.go`, holds the balance of each account.
It accepts a transfer when its amount is positive, and the sender holds at least that amount.
Executing the transfer moves the amount from the sender to the receiver.
The bank keeps two copies of the balances, as the applications of CometBFT do:

- `committed`, the balances after the last block, the same on every node;
- `check`, the balances after the last block *and* after the transactions that the mempool has accepted since.
  This is the *current state* against which `IsValid` validates a transaction; it is local to the node.

`IsValid` (already written) validates a transaction and, if it is valid, applies it to `check`, so that the next transaction is validated after it.
`Commit` (already written) applies a block to `committed`, then resets `check` to a copy of `committed`.
Both rely on the function `apply`, which executes a transfer on a set of balances.

**[Task]** Implement `apply` in `app.go`.

<details>
<summary><b>help me</b></summary>

The balances are a *map* from the name of an account to its balance, `map[string]uint64`.
Reading `state[name]` returns the balance, and `0` if the account is not in the map: this is exactly what we want for an account that never received anything.
Writing `state[name] = v` adds or updates an entry (see [mutating maps](https://go.dev/tour/moretypes/22), in the tour of Go).

1. Decode the transfer with your `DecodeTransfer`.
   If it fails, return `false`.
2. Check the amount and the balance of the sender, and return `false` if one of them is wrong.
   Several conditions can be combined with `||` (or), as in `if err != nil || t.Amount == 0 {`.
3. Only once every check has passed, move the amount: subtract it from the sender, add it to the receiver, and return `true`.
   Checking first matters: `apply` must leave `state` unchanged when the transfer is not valid.

The amounts are of type `uint64`, which cannot be negative: if you subtract more than the balance, the result wraps around to a huge number!
Another thing to check before moving funds.

</details>

At any time, you can validate `apply` by running the following command:

```sh
go test -run TestIsValid -v
```

## 6. The pool: deduplication, validity, order [30']

The `Pool`, in `pool.go`, holds two structures, as the mempool of CometBFT:

- `txs map[TxKey]*list.Element`, the index that answers the question: have I already seen this transaction?
- `fifo *list.List`, a doubly linked list of the transactions, oldest first.
  `Reap` (already written) returns its first transactions: those of the next block.

Why a list, and not just the map?
A Go map does not remember the order of insertion: going through it returns its entries in an arbitrary order.
Why a map, and not just the list?
Finding a transaction in the list takes a scan.
Each entry of the map points to its element of the list, so the pair gives both, and a removal in O(1).
Keeping them consistent is the whole game.

Before a transaction enters the pool, the pool asks the application whether it is valid in the current state, by calling `p.app.IsValid(tx)`.

The pool is accessed concurrently: each incoming connection is served by its own goroutine (section 8), and all of them insert into the same pool.
So every operation of `Pool` is protected by a lock, `p.mu`: it starts with `p.mu.Lock()`, immediately followed by `defer p.mu.Unlock()`.
To understand how `defer` releases the lock when the function returns, read the [page on `defer` of the tour of Go](https://go.dev/tour/flowcontrol/12).

**[Task]** Implement `Pool.Insert` in `pool.go`.

<details>
<summary><b>help me</b></summary>

**The map.** To tell "absent" from "present", read the map with two variables, the second one a boolean (see [mutating maps](https://go.dev/tour/moretypes/22)):

```go
e, ok := p.txs[key]
```

The key of a transaction is `tx.Key()`.

**The list.** The package `container/list` provides the linked list.
[`p.fifo.PushBack(tx)`](https://pkg.go.dev/container/list#List.PushBack) adds `tx` at the end of the list, and returns its new [element](https://pkg.go.dev/container/list#Element): this is what the map stores.

**Insert** therefore: compute the key; if it is in the map, return `ErrTxInPool`; if `p.app.IsValid(tx)` returns `false`, return `ErrInvalidTx`; otherwise push `tx` at the back of the list, store the element in the map, and return `nil`.
The lock is already taken for you at the top of the function, and released by `defer` when it returns.

</details>

At any time, you can validate `Insert` by running the following command:

```sh
go test -race -run 'TestInvalid|TestFIFO|TestConcurrentInsert' -v
```

**[Q1]** `IsValid` does not only check a transfer: it also applies it to the check state.
Suppose that `Insert` called `IsValid` *before* looking the transaction up in the map.
Alice holds 100, and the node receives her transfer of 30 to Bob 4 times.
What does the node then believe Alice holds?
What happens when she then sends 20 to Carol?
The test `TestDedup` checks a similar scenario (a transfer of 60 received twice, then one of 40); you can run it with the following command:

```sh
go test -run TestDedup -v
```

## 7. Committing a block [25']

Once the network has decided a block (by consensus, which is not our concern here), every node calls `Pool.Update` with it.
The application executes the block, and resets its check state to the new committed state.
Then the pool does two things, as CometBFT's `Update`:

1. it removes the transactions of the block, which are no longer pending;
2. it *rechecks* every other transaction, oldest first, against the new state, and removes those that are no longer valid.
   Rechecking in order rebuilds the check state of the application, one transaction after the other.

**[Task]** Complete `Pool.Update` in `pool.go`.

<details>
<summary><b>help me</b></summary>

The first loop goes through `block` with `for _, tx := range block`, and calls `p.remove` on the key of each transaction (not `p.Remove`, which would try to take the lock that `Update` already holds, and block forever).
In Go, a mutex is not re-entrant: a goroutine that already holds `p.mu` and calls `p.mu.Lock()` again waits for itself forever; hence the pair `Remove` (which takes the lock) and `remove` (which expects the caller to hold it).

The second loop goes through the list.
The [example of the package](https://pkg.go.dev/container/list#example-package) shows the classic loop:

```go
for e := p.fifo.Front(); e != nil; e = e.Next() {
	// e.Value is the element
}
```

There is a trap: here the loop removes elements, and `p.fifo.Remove(e)` erases the link from `e` to the next element, so `e.Next()` returns `nil` after it, and the loop stops early.
Read `next := e.Next()` *before* removing `e`, and move on with `e = next` at the end of the body.
In the body, ask the application again with `p.app.IsValid`, and remove the transaction if it returns `false`.

`e.Value` has the type `any` (it can hold anything); to get the `Tx` back, write the [type assertion](https://go.dev/ref/spec#Type_assertions) `e.Value.(Tx)`.

</details>

At any time, you can validate `Update` by running the following command:

```sh
go test -run TestUpdate -v
```

**[Q2]** Rechecking costs one call to the application per pending transaction, at every block.
CometBFT lets the operator of a node turn it off (`recheck = false`, in the `[mempool]` section of its configuration).
For which kind of application would this be safe?
Hint: what must hold for a transaction that was valid when it entered the pool?

## 8. Dissemination: flooding over TCP [25']

It is now time to connect the blockchain nodes!
We use the simplest way to disseminate a message, **flooding** (lecture 2, slide *A possible solution: flooding*): each node relays every transaction that enters its pool to all its neighbors, and every node that receives it does the same.

The protocol between nodes is as simple as it gets: one transaction per TCP connection.
To send a transaction to a peer, a node opens a connection, writes the bytes of the transaction, and closes the connection.
These bytes are those of section 4, a `Transfer` encoded with Protocol Buffers: there is nothing else to encode.
On the other side, the peer reads everything until the connection is closed: these are the bytes of the transaction, which it hands to `Relay`.

**[Task]** Complete the accept loop of `Node.Listen`: spawn a goroutine per connection that runs `n.handle(conn)`.
A *goroutine* is a lightweight thread, managed by the Go runtime rather than by the operating system: it costs a few kilobytes, so a program can run thousands of them (Java has the same idea with its *virtual threads*).
The tour of Go explains them on its page [goroutines](https://go.dev/tour/concurrency/1).

<details>
<summary><b>help me</b></summary>

The keyword `go` before a function call runs the call in a new goroutine, and moves on at once: `go n.handle(conn)`.
Without `go`, the node would serve one connection at a time.

</details>

**[Task]** Complete `Node.handle`: read all the bytes of the connection, and relay them with `n.Relay`.
If the read fails, log the error and return.

<details>
<summary><b>help me</b></summary>

[`io.ReadAll(conn)`](https://pkg.go.dev/io#ReadAll) reads from the connection until the peer closes it, and returns all the bytes read, with an error.
A `[]byte` can be passed where a `Tx` is expected, since `Tx` is defined as a `[]byte`.

</details>

**[Task]** Implement `Node.Broadcast`: for each peer, open a connection, write the transaction, and close the connection.
If a peer is down, log and *keep going*: resilience to a dead peer is the point of gossip.
`peerAddrs()` already returns a copy of the peer set, so you never hold the lock during I/O.

<details>
<summary><b>help me</b></summary>

[`net.Dial("tcp", addr)`](https://pkg.go.dev/net#Dial) opens a connection to `addr`, and returns it with an error.
On error, log it with `log.Printf` and `continue` with the next peer.
Otherwise, `conn.Write(tx)` sends the transaction, and `conn.Close()` closes the connection (see the [`Conn`](https://pkg.go.dev/net#Conn) interface).

</details>

At any time, you can validate your node by running the following command, which starts three nodes on your machine and submits transactions to one of them:

```sh
go test -run TestFlooding -v
```

Now read `Relay`, in `peer.go`: this is where the mempool meets the network.
It inserts the transaction in the pool, and broadcasts it only if `Insert` accepted it.
So a node relays a transaction only the first time it receives it: the map of the pool plays the role of the `received` set of the flooding algorithm of lecture 2, and makes the flood stop by itself.
There is one difference, though: since an invalid transaction never enters a pool, it is never relayed.

**[Q3]** Compare `Relay` with the flooding algorithm of lecture 2 (slide *A possible solution: flooding*).
Lines 5 and 6 of the algorithm play the same role as the map of the pool, in `Insert`.
The algorithm relays a message (line 8) *before* delivering it (line 9), so that a process that crashes right after delivering has already passed the message on.
`Relay` does the opposite: it inserts the transaction in the pool (our "delivery"), then broadcasts it.
If a node crashes in between, is any harm done?
Then, think of Alice's double spend (section 1): which property of reliable broadcast (integrity, validity, termination, agreement) no longer holds, once nodes drop the transactions they find invalid?

**[Q4]** Lecture 2 also presents *rumor mongering*, a gossip protocol where a node that receives a message propagates it to a few other nodes.
Our flooding is a form of rumor mongering: what is its fanout, and which stop condition among those of the lecture (slides *Rumor mongering: stop conditions*) makes it terminate?
What would change if each node relayed a transaction to a few peers chosen at random, instead of all of them?
This is closer to what blockchains do in practice: go-ethereum sends a new transaction in full to only the square root of its peers, and announces its hash to the others; a CometBFT node relays it to all its peers, but these are only a small part of the network (by default, at most 10 outgoing and 40 incoming connections).

## 9. Running the demo [10']

`go run .` compiles the package of the current directory (`.`) into a temporary program and runs it at once, with the words that follow as its arguments.
Here, the program is the demo of `main.go`.
It starts 5 nodes on `127.0.0.1`, connected to each other, all running the bank from the same genesis (Alice holds 100, Bob 50); then, a step every 300 ms:

1. Bob pays 20 to Carol, at the first node: the transfer floods everywhere;
2. Carol pays 30 to Dave: she holds only 20, so the node rejects the transfer, which goes nowhere;
3. Alice sends 70 to Bob at the first node and, at the same time, 70 to Carol at the last one;
4. the middle node proposes a block with all its pending transactions, every node commits it, and meanwhile Bob pays 10 to Dave.

Each node logs every transaction it accepts or rejects, and the demo prints the pools before and after the block:

```sh
go run .
go run . -nodes=8   # the option -nodes sets the number of nodes
```

**[Q5]** Run the demo several times.
Before the block, do all the nodes hold the same pool?
After the block?
Who decides which of Alice's two transfers wins, and is it always the same one?

**[Q6]** The rejections of step 3 are logged several times by the same node.
Why?

## 10. Going further (optional)

**[Opt]** **Nonces.** Protect the bank against replay attacks with a nonce, as Ethereum does (lecture 2, slides *Sending a transaction again?* and *The nonce*).

1. Add a field `uint64 nonce = 4;` to `Transfer` in `mempool.proto`, then regenerate `mempool.pb.go`.
   This needs the compiler `protoc` (from <https://github.com/protocolbuffers/protobuf/releases>) and its Go plugin, which `go install` downloads, compiles, and installs in `~/go/bin`:

   ```sh
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   export PATH="$PATH:$HOME/go/bin"   # so that protoc finds the plugin
   protoc --go_out=. --go_opt=paths=source_relative mempool.proto
   ```

2. Give `NewTransfer` a `nonce` argument.
3. Make the bank remember, for each account, the number of transfers it has executed, and change `apply`: a transfer is valid only if its nonce is equal to that number, and executing it increments the number.
4. Add a step at the end of `main.go`, where somebody sends again the bytes of Bob's transfer to Carol, which is already in the block: the node must reject it.

Suppose that Bob sends two transfers in a row, with nonces 0 and 1, through the same node.
Since our nodes send each message on its own TCP connection, another node may receive the second one first: what happens to it there?

**[Opt]** **Random topology.** Replace the complete graph in `main.go` with a ring (each node peers only with its predecessor and its successor).
Does every transaction still reach every node?
What about the double spend?

**[Opt]** **Malleability.** The key of a transaction is the hash of its *bytes*, not of the transfer they encode.
Read the section on [field order](https://protobuf.dev/programming-guides/encoding/#order) of the protobuf encoding: can two different sequences of bytes decode to the same `Transfer`?
If an attacker re-encodes Alice's transfer that way, and sends it before Alice's own version reaches the other nodes, what do the mempool and the bank do with the two versions?
Alice follows her transfer by its key: what does she see?
(This problem has a name in Bitcoin, *transaction malleability*; look it up.)

## What we simplified

- **no signatures**: anybody can send a transfer on behalf of Alice.
  A real application first checks that the transaction is signed by its sender;
- **no nonce**: once a transfer is in a block, anybody can send its bytes again, and the bank executes it again; Ethereum prevents this replay with the nonce of the sender (lecture 2, slide *The nonce*, and the first optional item of section 10);
- **no gas**: a sender only needs to hold the amount it sends, while in Ethereum it must also hold the maximum gas fee of the transaction;
- **no fees, no priority**: the pool is first come, first served, as CometBFT's default mempool.
  Ethereum's mempool sorts the transactions by fee, and evicts the cheapest ones when it is full;
- **no bound** on the size of the pool, or of a transaction;
- **no consensus**: in the demo, the block is decided by fiat, and handed to every node.

## Validation checklist

Hand the practical in when:

- [ ] all the `[Task]` gaps in `tx.go`, `app.go`, `pool.go` and `peer.go` are filled;
- [ ] `go test -race -v` passes;
- [ ] `go run .` shows the transfer of Carol rejected and never relayed, and, after the block, the same balances on every node and a single pending transaction, Bob's transfer to Dave;
- [ ] your report answers the `[Q]` questions (a few sentences each), and is handed in with your code.

Congratulations: you have written, in miniature, the component that every blockchain node runs between "I hear a transaction" and "it is in a block".
The pieces you assembled today (a hash map to deduplicate, a list to keep the order, a question to the application before anything gets in, and a recheck after every block) are those of CometBFT.
