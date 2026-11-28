# Spending without agreeing — the fast path of Mysticeti [4h]

**Outline.** You will implement the *fast path* of
[Mysticeti](https://arxiv.org/abs/2310.14821), the BFT protocol based on a directed acyclic
graph (DAG) of blocks, that runs the Sui blockchain, restricted to transfers that have a **single owner**.
You will write the DAG that validators build round after round, the voting
rule that rides on it for free, and the certification rule that makes a
transfer final — all of it without ever running consensus. On the way you will
see, in your own test output, why a double spend can never be certified twice,
and why the account of whoever attempts one gets stuck.

This practical is the counterpart of the lecture on *replicated state
machines*. It uses the transaction dissemination you built in session 2 and the
quorum reasoning of sessions 3 and 4.

You will have to complete all the items marked by **[Task]** to successfully
fulfill this practical. Items marked **[Opt]** are optional, and questions
marked **[Q]** are to be answered in writing.

## 1. Context: what needs consensus, and what does not [35']

A blockchain is a replicated state machine: every validator applies the same
transactions, in the same order, to the same state. The "in the same order"
part is the expensive one — that is consensus, and sessions 3 and 4 told you
what it costs (FLP, leaders, view changes, `3f+1` validators, several rounds of
messages).

Here is the question that this practical is about: **when is that order
actually needed?**

Take a payment: *alice pays 10 to bob*. Take another: *carol pays 5 to dave*.
As long as each of them is funded, applying them in either order gives the
same balances — crediting an account is an addition, and additions commute.
There is nothing to agree upon. The *only* way to create a genuine conflict is for alice to spend the same money twice,
and a well-formed transfer is signed by its owner, so **only alice can create a
conflict on alice's money**.

Wei Dai sketched exactly this in [b-money](http://www.weidai.com/bmoney.txt)
(1998), well before Bitcoin. It was proved twenty years later: asset transfer
has [consensus number
1](https://arxiv.org/abs/1906.05574) — it can be implemented in an asynchronous
system with faulty processes, where consensus cannot. Modern systems
industrialised it:
[FastPay](https://arxiv.org/abs/2003.11506), Astro, and then Sui, which splits
its objects in two:

- **owned objects**, whose only writer is their owner. These take the *fast
  path*: two message delays, no consensus, no leader, no ordering;
- **shared objects** (e.g., a lock, an order book), where several users compete
  for the same state. These need a total order, hence consensus.

Mysticeti implements both over a *single* structure, a DAG of blocks:
`Mysticeti-C` derives a total order from the DAG for shared objects, and
`Mysticeti-FPC` certifies owned-object transactions directly on the DAG. **You
are implementing a simplified `Mysticeti-FPC`.**

**[Q1]** Give two examples of blockchain operations that cannot be put on the
fast path, and say which shared state they compete for.

### 1.1 What object are we building? [25']

Before writing code, fix *what* the validators implement together. This is the
**asset transfer object** of lecture 5 (the slide "A base state machine: the
asset transfer object"), after ["The consensus number of a
cryptocurrency"](https://arxiv.org/abs/1906.05574) (Guerraoui et al., PODC
2019): a balance per account, one **owner** per account, and a *total*
`transfer(x, y, a)` that returns `true` and moves the money if its caller owns
`x` and `balance(x) >= a`, and otherwise returns `false` and changes nothing.
Reread that slide before going on.

To reason about what such an object costs, we use two notions about pairs of
operations. Two operations *commute* if applying them in either order leaves the
same state and the same responses. One *overwrites* the other if applying it
after the other leaves the same state and returns the same response as
applying it alone. If every pair of operations of an object either commutes or
overwrites, the object has consensus number 1: it can be implemented without
consensus. A finer criterion is the *indistinguishability graph* of
["Adjusted Objects: An Efficient and Principled Approach to Scalable
Programming"](https://doi.org/10.1145/3721462.3730953) (Kane et al., Middleware
2025): take one operation per process and all the orders in which
they can run; two orders are linked when some operation, say `c`, obtains the
same response in both and leaves the same state right after `c`; the object
needs consensus when these orders split into at least two connected components.

**[Q2]** Take the operations `transfer` and `read`, with one owner per
account. For each kind of pair (a credit and a debit of the same account, debits
of two different accounts, two debits of the same account, a `read` and a
`transfer`), say whether the two operations commute or overwrite each other.
Which pairs may be genuinely in conflict, and who can issue them?

**[Q3]** Now let two processes, alice and bob, *both* own an account that
holds 1. Each tries to move everything to a private account. Show that this
solves consensus between the two processes: how does each of them learn the
winner? What does this tell you about the role of the *single owner* in the
specification above?

**[Q4]** Back to one owner per account. Alice has 5 and bob has 10. Alice
invokes `transfer(alice, bob, 10)` while bob invokes `transfer(bob, alice, 5)`.
Write the two orders, the responses and the states they produce. Is there a
winner that each process can recognise on its own? Draw the indistinguishability
graph of these two operations. What does the result of Kane et al. tell us in
this case? Hint: which of the two operations overwrites the other?

**[Q5]** Forget Byzantine failures for a moment: validators may crash, the network
is asynchronous, and every validator keeps a copy of the ledger. A validator that owns an
account `x` executes `transfer(x, y, a)` as follows: if its local balance of `x`
is at least `a`, it sends the transfer to all validators and returns `true`;
otherwise it returns `false`. Every validator applies a transfer when it receives
it.

(a) Alice pays 10 to bob, who has 0, and bob then pays 10 to carol. Show an
execution in which a third validator ends with a balance that differs from the
others'.

(b) Which guarantee must the broadcast give so that this cannot happen? Does the
protocol need consensus anywhere?

(c) What changes if alice is Byzantine and signs two different transfers of her
whole balance? (One sentence: the optional exercises at the end of the practical
go further.)

## 2. Getting started [10']

Install Go (>= 1.21) following the official instructions:
<https://go.dev/dl/>, then check the installation and run the tests:

```sh
go version
go test
```

Everything should *compile* at this point and the tests should fail. Your job
is to fill the `// TODO` gaps and turn the failures green, in the order of the
sections below. Two tests are skipped: those you have to write yourself.

## 3. Tour of the provided code [10']

	tx.go             transfers, and the asset a transfer consumes
	block.go          the vertex of the DAG: parents, transactions, votes
	dag.go            the block store, its validity rules, causal history
	validator.go      the fast path: propose, vote, certify, execute
	sim.go            a simulated network, provided, nothing to do here
	main.go           a runnable demo, provided
	dag-animation.html  an animation of the DAG, to open in a browser
	dag_test.go       acceptance tests for sections 4 and 5
	fastpath_test.go  acceptance tests for sections 6 and 7

Read `tx.go`, `block.go` and the head comment of `validator.go` now; they carry
the model. Two remarks before you start:

- Unlike the mempool of session 2, **there are no locks here**: the whole
  simulation runs in one goroutine, and blocks are immutable once built. Do not
  add concurrency, it would only get in your way.
- Validators are numbered `0..n-1`, and all count the same when we count them.

## 4. Transactions, and the asset they consume [20']

A transaction here is `owner → recipient, amount`, plus a **sequence number**
`Seq`. The pair `(Owner, Seq)` is the asset being consumed: think of it as the
*version* of the owner's account, or as the identifier of an owned object in
Sui. Spending it twice is a double spend, and it is the only conflict that
exists in this system.

**[Task]** Complete `Tx.ComputeHash` in `tx.go`: serialize the owner, the
sequence number, the recipient and the amount, then hash the result with
SHA-256. The two integers are written in **big-endian** order: an integer of
64 bits is written on 8 bytes, the most significant byte first. For instance,
`Seq = 3` is the 8 bytes `00 00 00 00 00 00 00 03`. Fixing the byte order and
the width is what makes all the validators, whatever their machine, compute
the same bytes, hence the same identifier.

<details>
<summary><b>help me</b></summary>

As in `NewTree` of the Merkle practical, you build the bytes to hash by
*concatenating* slices with `append`. A `[]byte` is a slice of bytes, and
`append(data, other...)` returns `data` followed by the bytes of `other`: the
three dots *spread* the elements of `other`, instead of appending the slice as
one element. Start from an empty slice, and add the fields one after the
other:

```go
var data []byte                   // empty for now
data = append(data, t.Owner...)   // a string can be spread as bytes
```

For an integer, the package `encoding/binary` writes it in big-endian order
at the end of a slice:

```go
data = binary.BigEndian.AppendUint64(data, t.Seq)
```

Do the same for `To` and `Amount`. Then `sha256.Sum256(data)` returns an
array of 32 bytes; `sum[:]` turns it into a slice, and `TxID(sum[:])` converts
it into a `TxID`. Add the two imports that you use (`crypto/sha256` and
`encoding/binary`) to the `import` block of `tx.go`.

</details>

**[Task]** Complete `Tx.ConflictKey`: return a string identifying `(Owner,
Seq)`, for instance `alice/3`. Two transactions conflict when they share a
conflict key but have different identifiers.

At any time, you can validate these two functions by running the following
command (`-run` selects the tests whose name matches the pattern):

```sh
go test -run 'TestTx' -v
```

## 5. The DAG [55']

Validators do not exchange transactions one at a time. Each of them emits **one
block per round**, and every block points to blocks of the previous round. The
result is a DAG that each validator builds locally, from the blocks it
receives: two validators may see it with a delay, but they agree on everything
below a block that they both hold.

To see how blocks are built, how a transaction travels and how votes ride on
the next blocks, open [`dag-animation.html`](dag-animation.html) in a browser
and step through it, with and without a slow validator.

The DAG does three jobs at once, and this is why it is worth the trouble:

1. it **disseminates** transactions, which is the mempool of session 2;
2. it makes the **past agreed upon**: holding a block means holding its whole
   causal history, and the identifier of a block commits to that history, so
   two validators never disagree about what lies below a given block. (It does
   not promise that every validator receives every block: getting a missing
   block from the others is not implemented, see "What we simplified".);
3. it **carries the votes**. Referencing a block proves you have seen
   everything below it, so you can state your opinion on those transactions in
   the block you were going to send anyway. Voting costs *zero* extra messages,
   which is why a transaction is final after only two rounds.

A block is valid when it is properly named and properly supported:

	round 0                 the genesis block of each validator, no parents
	round r > 0             parents are blocks of round r-1, authored by at
	                        least a quorum of distinct validators, one of
	                        which is the author itself

The **genesis block** is the first block of a chain: it is fixed in advance,
it carries no transaction, and it points to no parent. Every validator knows
the genesis blocks of all the others from the start; they are the common root
of the DAG, and what the blocks of round 1 point to.

**[Task]** Complete `Block.ComputeID` in `block.go`: hash the author, the
round, the parent identifiers, the transaction identifiers and the votes (a
vote is the identifier of its transaction, followed by one byte for the accept
bit). The beginning is given, in the same style as `Tx.ComputeHash`: finish it.
Hashing the parents is what turns the DAG into a Merkle DAG — naming a block
names its entire causal history.

**[Task]** Complete the structural rules in `DAG.Add` (`dag.go`), returning the
provided errors `ErrBadGenesis`, `ErrBadParentRound`, `ErrWeakSupport` and
`ErrNoOwnParent`. The skeleton already contains the check that every parent of
a block of round `r` sits at round `r-1`; you write the three others.

**[Task]** Complete `DAG.CausalHistory`: all the *strict* ancestors of a block,
each one exactly once, sorted with `sortBlocks`.

<details>
<summary><b>help me</b></summary>

The ancestors of `b` are its parents, the parents of its parents, and so on
until the genesis blocks. This is a graph traversal; the only trap is that the
DAG is not a tree, so a block can be reached by several paths and must be
collected only once. Use two variables:

```go
out := []*Block{}               // the ancestors found so far
seen := make(map[BlockID]bool)  // the identifiers already visited
stack := append([]BlockID{}, b.Parents...)  // the identifiers still to visit
```

`append([]BlockID{}, b.Parents...)` copies the parents, so that you do not
modify the slice of the block (a block is immutable). Then loop while the stack
is not empty (`for len(stack) > 0 { ... }`). At each iteration:

1. pop the last element: `id := stack[len(stack)-1]`, then
   `stack = stack[:len(stack)-1]`;
2. if `seen[id]`, do nothing (`continue`); otherwise set `seen[id] = true`;
3. get the block with `d.Get(id)` (it is never `nil` for a block that was
   accepted by `Add`, but test it anyway), add it to `out`, and push its
   parents on the stack: `stack = append(stack, p.Parents...)`.

When the stack is empty, call `sortBlocks(out)` and return `out`: sorting makes
the result the same for all the validators, whatever the order of the visit.

</details>

At any time, you can validate these three functions by running the following
command:

```sh
go test -run 'TestGenesis|TestBlock|TestValid|TestQuorum|TestOwn|TestParents|TestMissing|TestCausal|TestEquivocation' -v
```

**[Q6]** Why must a block reference a *quorum* of blocks of the previous round,
and not just one? What would break if a validator could build on a single
parent?

**[Q7]** Read `DAG.Insert`. A block whose parents are unknown is *suspended*
rather than dropped. What could a validator conclude wrongly if it processed a
block without its causal history?

**[Q8]** `Insert` does **not** reject a second block from an author that
already emitted one in the same round: equivocated blocks are stored like any
other. (An *equivocation* is what a Byzantine validator does when it signs two
different blocks for the same round, and shows each of them to different
validators; recall lecture 4.) Find the two places in the code where
equivocation is actually neutralised.

## 6. Voting, for free, on the DAG [35']

Now the protocol itself. When a validator builds its block for round `r`, it
looks at everything new in the causal history of its parents and states an
opinion on each transaction it has not voted on yet:

- **accept** — "I have seen this transaction, and I had not accepted any
  transaction consuming the same asset before";
- **reject** — "I had already accepted a conflicting one".

An opinion is a `Vote`, defined in `block.go`: the identifier of a transaction
and a boolean `Accept`. The votes of a block are listed in its `Votes` field,
next to its `Txs` (the transactions it proposes).

Two rules, and the entire safety of the fast path rests on them:

> **(1)** a validator votes **at most once** per transaction: its first opinion
> is its only opinion;
> **(2)** a validator accepts **at most one** transaction per asset: seeing one
> locks that asset for good.

**[Task]** Complete `Validator.voteOn` in `validator.go`, following the two
rules above (`v.voted` implements the first, `v.locked` the second).

**[Task]** Complete `Validator.TryAdvance`: take the pending transactions,
vote on them *and* on the unvoted transactions of the causal history (helper
`unvoted`), assemble the block, move to the next round and hand the block to
`Receive`.

At any time, you can validate these two functions by running the following
command:

```sh
go test -run 'TestVote|TestProposer' -v
```

**[Q9]** We never verify a signature in this practical. Where exactly would the
owner's signature be checked, and which property of the protocol collapses
without it? (Hint: who is allowed to create a conflict?)

**[Q10]** Our blocks carry an explicit `accept` vote for every transaction.
Mysticeti does better: a block *implicitly* accepts everything in its causal
history, and carries only explicit rejects. What does that change in the size
of a block, and why is it correct?

## 7. Certification: finality without consensus [35']

A transaction is **certified** — final, executable, irreversible — as soon as a
quorum of *distinct* validators have accepted it. Nothing else. No leader, no
ordering, no second phase.

	           round r              round r+1                    round r+2
	V0    [ proposes tx ] ────► [ accept tx ] ───┐
	V1    [             ] ────► [ accept tx ] ───┼──► [ V0 counts 3 accepts:
	V2    [             ] ────► [ accept tx ] ───┤       tx is certified ]
	V3    [             ] ────► [ reject tx ] ───┘   (V3 had seen a conflict)

**[Task]** Complete `Validator.tally`: count the accept votes carried by a
freshly inserted block, **per transaction and per author**, and certify a
transaction when it reaches `Quorum()` distinct accepting validators. Then
execute it (`v.execute`, provided).

```sh
go test -run 'TestSingle|TestCrashed' -v
```

**[Q11]** Prove the safety of the fast path, in three lines. Suppose two
conflicting transactions are both certified. Each of them gathered accept votes
from a quorum. What do you know about the intersection of two quorums in a
committee of `3f+1`? Conclude, using rule (2) of section 6.

**[Q12]** Sketch why, if a correct validator certifies a transaction, every
correct validator eventually certifies it. Which facts about the DAG do you use
(where do the accept votes live, who eventually receives which blocks)? Which
assumption does the code *not* implement?

**[Q13]** Now the other side of the coin. Suppose alice submits two conflicting
transfers to two different validators, and the committee splits 2-2. Neither
transaction is ever certified. What happens to alice's account, forever? Is
that a problem — and for whom? Compare with what a leader-based protocol would
have done.

**[Q14]** `execute` applies certified transfers in the order in which *this*
validator certified them, which differs from one validator to the next, and yet
all the ledgers agree. Why? Which single ordering constraint did we have to
keep, and why that one? Is it enough? Try to break it in the next question.

**[Q15]** Take bob with a balance of 0, and two certified transfers: alice
pays 10 to bob, then bob pays 10 to carol. Can two validators end with different
ledgers (look at `execute`)? Who decides that a transfer is unfunded in the
specification of section 1.1 (the owner, from its own history), and who decides
it in `execute`? What should a transfer carry so that every validator reaches
the same decision?

## 8. Proving it with tests [20']

The two properties that matter are not tested yet. Write them.

**[Task]** Complete `TestDoubleSpendIsNeverCertifiedTwice` in
`fastpath_test.go`: hand two conflicting transactions to two different
validators, run the simulation, and assert that no validator certified both and
that all the validators agree on the outcome. Run it over several seeds — the
delivery order changes with the seed, and so does the winner.

**[Task]** Complete `TestBalancesConverge`: a handful of transfers from
different owners, handed to different validators; assert that every validator
certifies every transfer, that they all report the same balances, and that the
total amount of money is unchanged.

At any time, you can run all the tests, with the name of each one, by running
the following command:

```sh
go test -v
```

## 9. Running the whole thing [20']

`go run .` compiles the program of the directory and runs it; the flags that
follow it select a variant:

```sh
go run .
go run . -v                 # votes and certifications, as they happen
go run . -seed=4            # another delivery order
go run . -crash=2           # one validator is dead
go run . -byzantine         # one validator emits two blocks per round
```

The demo runs a committee of four, transfers money between four accounts, and
has alice attempt a double spend. It then checks that every live validator
agrees on the ledger and reports the fast-path latency, in rounds.

**[Q16]** Run the demo with several seeds and look at the last line. Sometimes
one spend wins, sometimes neither does, but never both, and all the validators
always agree on which. Which part of the code decides the winner?

**[Q17]** Run `go run . -crash=2`. Why are some transfers never certified, and
why can the double spend no longer be resolved? Give the quorum arithmetic.

**[Q18]** Run `go run . -byzantine`. Validator 3 emits two different blocks in
every round, and everything still works. Which line of `selectParents` and
which line of `tally` keep you safe?

## 10. Going further (optional)

**[Opt]** State and sketch a proof of *convergence*: two correct validators that
certified the same set of transfers have the same ledger. Hint: express the
balance of an account as a function of the credits it received and of its
`Seq`-ordered debits. Where does the proof break with the current `execute`?

**[Opt]** Sketch why a funded transfer of a correct owner is
certified within 2 to 3 rounds when at least `2f+1` validators are correct and the
network is synchronous. Which rule of block building makes it so?

**[Opt] Specification.** Specify the abstraction that the validators implement together, in the
style of the replicated state machine of lecture 5, but ordering only
*conflicting* commands. Give (a) the commands and their conflict relation, from
your answer to question **[Q2]**; (b) the properties that correct validators
must satisfy, in the presence of Byzantine validators **and** Byzantine owners: at
least *integrity*, *consistency* (what must be ordered, what need not),
*convergence* and *liveness*; (c) what is **not** guaranteed when an owner is
Byzantine. Compare with the leader-based generalized SMR of lecture 5: why can
the dependency graph of this object never contain a cycle?

**[Opt]** List the properties that the DAG gives you as a *causal-order
broadcast* (what is delivered, in which order, what happens to a block whose
history is incomplete). Point at the code that provides each one. Which property
is *not* provided by the DAG alone, and could a Byzantine owner exploit it?
(Hint: reread what a double spend is.)

## What we simplified

Worth knowing before you quote this code in an exam:

- **no cryptography beyond hashing**: no signatures on transactions or blocks,
  no serialization — validators share blocks in memory. Equivocation is
  therefore detectable here but not *provable* to a third party;
- **explicit accept votes**, where Mysticeti keeps them implicit (see
  **[Q10]**);
- **stricter DAG**: our parents must sit exactly at round `r-1`, real
  implementations also link older blocks;
- **no synchronization, no garbage collection**: we suspend blocks with missing
  parents and wait, and we never forget anything;
- **funds are not checked**: a certified transfer is applied or skipped by
  `execute` against the local balance, see question **[Q15]**;
- **no consensus path**: shared objects, and the assets locked by a double
  spend, need `Mysticeti-C`, which we do not implement.

## Validation checklist

Hand the practical in when:

- [ ] all the `[Task]` gaps in `tx.go`, `block.go`, `dag.go`, `validator.go`
      and `fastpath_test.go` are filled;
- [ ] `go test -v` passes, with no skipped test;
- [ ] `go run .` reports agreeing ledgers, a latency of 2 to 3 rounds, and
      never certifies two conflicting spends — for any seed, with `-crash` and
      with `-byzantine`;
- [ ] your report answers the `[Q]` questions (a few sentences each;
      **[Q11]** and **[Q12]** (the proof
      sketches) and **[Q13]** are the ones that matter), and is handed in with
      your code.

Congratulations: you have implemented a payment system that is final in two
message delays, tolerates `f` Byzantine validators out of `3f+1`, and never
runs consensus. The insight is thirty years old and the protocol is three years
old — and it is what makes Sui settle a transfer faster than the block time of
almost every chain that runs consensus for everything.
