# SYMM — Temporal Precursor Learning, Staged Training, and Forward Proof

Implement the learning system of SYMM.

The goal is to implement the intended learning experiment faithfully so we can determine whether it actually works.

The central hypothesis is:

> By structuring every metric produced by the signal and logic stages into a 2D "grid" and subsequently restructuring that grid into a set of sympathetic "regions" following a consistent set of rules, we can begin to create a model that can learn to recognize precursors to market excursions.

## Impulse Map

A 2D coordinate space where Signals and Logic Solvers write their Observations. This will naturally create a grid where cells “light up” when reacting with the market tape. 
The next step is to re-organize the Impulse Map to cluster sympathetically. Where sympathy is a ladder of priorities. 
These priorities are additive when they happen at the same time, though hold true stand-alone too. 

- **First:** Values that move together attract, this helps correlated metrics to cluster together, reducing fragmentation in the signals.
- **Second:** Values with closest relative magnitude during movement attract
  * Sign is irrelevant if the match is consistent, so A+ and B- can attract only if A- and B+ holds
  * Repel if inconsistent, so either A+ and B- then A- and B-, or A+ and B- then A0-or-Nil and B+ 
- **Third**: Maturity and SNR powers attraction, if A is stronger than B, B moves more to A than A to B. If SNR or maturity is not available, you can use "separation" which means grouping the Metrics within a Measurement into "buckets" based on their opposition towards each other, and determining how much the strongest "bucket" stands out above the mean of the other bucketed metrics. This will require using the standardizer, obviously, which requires all metrics to properly set center and scale.
- **Forth**: Once two metrics are attracted closely enough, they "bind" into a developing region. That region will now add additional power to each metric attracting other metrics. This is an attempt to prevent one big region from forming. A bound metric can no longer be "stolen".

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

Combining this with a sequence of "priors" from previous time steps we can build up a temporal "signature" and use that to generate a prediction for the next time step. This is the core behind detecting developing precursors for all types of market events.

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

## Orchestration

In training.go there is a certain orchestration that must be followed.

1. The Step method is part of the LMAX Disruptor pipeline, which means it receives the real-time market tape cut on each call.
2. The Step method first develops the grid, settling into regions, it does not do anything else.
3. Once the grid has settled, is frozen, and the checkpoint is saved to storage, the Run method starts a background goroutine, and starts training on historical tape fragments stored in Iceberg Tables. Training means running the Measurements that belong to the tape fragment through the frozen grid, and retrieving the region token for each tick, and using the predictive radix trie to build up a signature of past region tokens to determine the most likely next region token, and using that to predict the likely next action (enter/exit/wait). This training process MUST be visualized for debugging purposes, so we can see if something goes wrong. The exact visualization must be modeled on ./frontend/tmp/learning/ the model training part, but obviously using the current styling already used for the rest of the UI. But it must show the tape fragment line, the A, B, and C markers, and the ENTER and EXIT prediction markers.
4. You can keep looping over the historical tape fragments, and train on them multiple iterations, just by offsetting the A marker at a random point, as long as it is before the B marker. This will help make the model more robust, since it will not just learn to recognize the precursor development from a specific offset to B only.
5. Once the model has built up enough skill (and is regularly checkpointed to storage), a secondary process starts in parallel. Now we go back to the Step method, but instead of manipulating the grid (which remains frozen) we now start using the real-time tape to predict real entry and exit points, against the live market. We will use trader.go to talk to broker.Desk, and use the paper trading engine to "paper trade" using the model. This means we will be actually validating the model, in preparation for the real thing. Of course this secondary process should also be refining the model, using the outcome of the paper trading positions.

## Important files and directories

- ./hindsight/store_tee.go
- ./nomagique/store/grid.go
- ./nomagique/cognition/
- ./strategy/training.go
- ./strategy/trader.go

## CRITICAL

- Do not fake anything
- Do not use fallbacks, ever
- If things are not as expected, throw an error
- Do not "invent" or work around things
- Do not use "best guess" approaches, only do things when you know what you are doing
- Do not believe tests that are green, they were likely mostly written to always be green, or are way too weak to be trusted

AND AT ALL TIMES, USE CRITICAL REASONING, FIRST-PRINCIPLES, AND COMMON SENSE. NEVER JUST GET LOST IN CHASING LINT ERRORS, OR START BLINDLY "SOLVING" ISSUES. ALWAYS TAKE A STEP BACK, AND LOOK AT THE BIGGER PICTURE. WE HAVE TO GET THIS RIGHT!

AND POTENTIALLY EVEN THE MOST CRITICAL POINT OF ALL: NEVER BE AFRAID TO REMOVE CODE. DO NOT JUST KEEP PILING ON MORE AND MORE COMPLEXITY TO TRY AND MAKE THINGS WORK. SCRATCH THE CODE, DELETE IT, START OVER. THIS IS ESSENTIALLY NOT A COMPLEX SYSTEM, SO DO NOT DROWN YOURSELF, OR ME, IN COMPLEXITY THAT IS NOT EARNED.

Read AGENTS.md for general project guidelines.