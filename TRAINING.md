# SYMM — Temporal Precursor Learning, Staged Training, and Forward Proof

Implement the next learning cut of SYMM.

The goal is to implement the intended learning experiment faithfully enough that we can determine whether it actually works.

The central hypothesis is:

> By structuring every metric produced by the signal and logic stages into a 2D "grid" and subsequently restructuring that grid into a set of "regions" following a consistent set of rules, we can begin to create a model that can learn to recognize precursors to market excursions.

## Impulse Map

A 2D coordinate space where Signals and Logic Solvers write their Observations. This will naturally create a grid where cells “light up” when reacting with the market tape. 
The next step is to re-organize the Impulse Map to cluster sympathetically. Where sympathy is a ladder of priorities. 
These priorities are additive when they happen at the same time, though hold true stand-alone too. 

- **First:** Values that move together attract 
- **Second:** Values with closest relative magnitude during movement attract
  * Sign is irrelevant if the match is consistent, so A+ and B- can attract only if A- and B+ holds
  * Repel if inconsistent, so either A+ and B- then A- and B-, or A+ and B- then A0-or-Nil and B+ 
- **Third**: Maturity and SNR powers attraction, if A is stronger than B, B moves more to A than A to B. 

This creates natural hot spots, separated by a colder gradient around it. 

### Regions 

The third priority naturally generates regions, the borders being defined by where the weakest cells meet. 

| 1.0 (A)  | 0.5 (A)  | 0.25 (A) | 0.25 (B) | 
|----------|----------|----------|----------| 
| 0.5 (A)  | 0.5 (A)  | 0.25 (A) | 0.25 (B) | 
| 0.25 (A) | 0.25 (A) | 0.25 (B) | 0.5 (B)  | 
| 0.25 (B) | 0.25 (B) | 0.5 (B)  | 1.0 (B)  | 

> Please note: the grid must be "locked" once the regions have formed, so basically we keep running the region "discovery" process, until it "settles".

By taking the N most "lit up" regions when it is reacting with the market tape (whether that is played back or real-time), we generate the "region token" which would be something like: [A, B, C] or [Z, E, F], etc.

Combining this with a sequence of "priors" from previous time steps we can build up a temporal "signature" and use that to generate a prediction for the next time step.

This is where the predictive Radix trie comes in.

All this combined would build up a relatively simple system:

Take a fragment of market tape, where point A represents an unknown/random point along the development of a precursor, point B is the point of "ignition" and point C is the point of exhaustion/stagnation/reversal. Our model would predict the likelyhood that point A is developing into point B, and if that means we should execute action "enter" or if point B (if we have entered at some point) is developing into point C and we should execute action "exit". In all other cases the best action would be "wait".

This is the fundamental idea behind the entire system.

## Training

To train on this successfully, we need to achieve the following:

### Excursion Detection

Monitor the current real-time market tape and determine real ignition and exhaustion/stagnation/reversal fragments, that means detecting B and C as they happen, then adding additional A fragments to the left, and some extra tape to the right, such that the discovered sequence is representative of a market excursion. This should allow for some fluctuation in the signal, and not be on a hair-trigger, so we get realistic scenarios to train on.

We store these fragments, so we can replay them at hardware speed for training.

This will develop the initial model, and by model we mean both the grid/regions, as well as the Radix trie.

Both these elements (grid/trie) should be able to be checkpointed and storable/loadable.

Once the model has gained enough skill from learning on fragments, we start a secondary training layer, which is using the paper trading mode, and using the model as it has developed so far to trade into the real-time market. We use the results from this to further refine the model, and to prove its edge against the real-world conditions.

While we are training, we can use `kraken paper reset` if the model burns through all its cash.

> Please note: The best version of this would not be using the ticker price, but the actual Level3 order book data to determine the profitability of excursions.

Of course we do not just want to train on only upwards excursions, and for variety we should at minimum detect and store:

1. Upwards movement, clearing friction (fees, etc.) (profitable)
2. Upwards movement, not clearing friction (unprofitable)
3. Downwards movement (unprofitable)
4. Choppy/sideways movement that doesn't lead to much of anything (unprofitable)
5. Flat line (unprofitable)

This way the model should also be able to learn what precursors to avoid.

Now, the absolute sweet spot for the model to enter is a little before point B (so there is enough time to execute the trade and fill the order, etc.), and same with exits, a little before point C.

The beauty of storing the excursion fragments is that we know the ground truth, so we can let the model predict the enter and exit points, and evaluate based on that knowledge, then adjust the model.