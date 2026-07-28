# The System

I am going to give you a high-level overview of what this system is, or what it is supposed to be. This is the true definition and original intention of this arctecture. The terminology matters.

## Signals

Signals are not evidence. They are produced in the process of "conditioning" the raw market data into a canonical shape that the rest of the system can key itself to. This shape is the signals output: the Measurement.

But signals are still quite raw, and still potentially somewhat noisy.

So we could say that teh signals are:

1. Conditioners
2. First pass noise reducers
3. Structural "shapers" where we consider the raw market data "unstructured" (in spirit, not in technical terms)

## Measurements

These are the inputs to the logic stage. They should hold the metrics that the signals produce, and some form of quality factor about the moment the measurement was taken. Whether that is called confidence, strength, or signal to noise I don't have an opinion about, as long as this is derived in a principled manner and not some heuristic. This is important, as it should determine how much influence each Measurement is allowed to exert.

Measurements will play different roles inside the logic stage, some of which in combination with other Measurements (meaning other signal sources).

Roles:

1. All measurements contribute to the Category classification process.
2. The Hawkes measurements have a specialized secondary role within the physics fluid sim (manifold), which is the injection of excitation of the oscillators.
3. All signals contribute to the building of the Graph, which is the true final "output" of the logic stage (not one value, or one or more scalars).

## Categories

The process of deriving categories from the Measurements is a dimensionality reduction process, which allows us to feed the dmt.Tree cognitive features (Classifier, Beam Search, REM Sleep, etc.)

They also play a big role within the graph, to help us build a structure the strategy package can use to trace the consequences of any proposals.

## Manifold, Resonance, Causal

This is a sequence in that exact order which can be considered a stabalizing and enrichment sub-stage. Each stage of this sequence feeds into the next one.

All results of these three stages, both intermediate and final output, should feed into the graph as additional context.

## Graph

As said before, the graph is the true, ultimate final output of the logic stage, and the thing that feeds into the final stage, the strategy.

It is a highly structured graph with nodes, connected through directional, weighted edges.

As a reminder, the edges should each have one of the following "types" or "contexts":

- Supports.
- Contradicts.
- Conditions.
- Leads.
- Lags.
- Redundant with.
- Independent of.
- Stale relative to.
- Incomparable with.

These relationships must retain evidence references and temporal context.

1. The Multi-Tier Graph ArchitectureInstead of choosing between 1 global graph or 640+ isolated graphs, implement a Two-Tier Hierarchical Graph System: ┌────────────────────────────────────────────────────────┐
 │            INTER-PAIR COORDINATION GRAPH               │
 │    Nodes = Category Clusters (DeFi, L1, L2, Memes)     │
 └───────────────────────────┬────────────────────────────┘
                             │ (Bridges via Categories)
 ┌───────────────────────────▼────────────────────────────┐
 │                  640+ PER-PAIR GRAPHS                  │
 │    Nodes = Specific Asset Pairs (e.g., BTC/USDT)       │
 └────────────────────────────────────────────────────────┘
Tier 1: The Per-Pair Graphs (What you have now)Role: Captures high-fidelity, hyper-local dynamics for a single trading pair.Input: Local Hawkes measurements inject excitation into this pair's localized physics manifold.Edge Contexts: Tracks local causal relationships (e.g., how the Orderbook Imbalance node Leads or Supports the Price Velocity node for that specific pair).Tier 2: The Inter-Pair Coordination Graph (The missing link)Role: Connects the 640+ individual graphs using your Categories as structural anchors.Nodes: The nodes here are not individual pairs, but the Category Classifications derived from your dmt.Tree dimensionality reduction.Edge Contexts: Focuses heavily on systemic macro relationships: Redundant with (highly correlated pairs), Leads / Lags (sector rotation), and Incomparable with (decoupled assets).

## Strategy

Here is where we make the ultimate decisions, and this is the only place where we would consider using the term "evidence" even though I don't think that is a very useful term to stand behind.

This is why I like the edge types of the graph. Supports, Contradicts. Those are clear terms with clear meanings.

What we want to do here is determine "utility".

1. Defining Utility for the 2 Active Slots (The Gridlock)When your 2 Active Slots are full, the system is in a state of gridlock. To break it, a new opportunity must not just be "good"—its utility score must exceed the remaining utility of an active trade, plus an eviction penalty (transaction friction, spread, and momentum loss).The Local Utility Delta FormulaTo evict an active pair (\(P_{\text{active}}\)) for a new candidate pair (\(P_{\text{candidate}}\)), the strategy must satisfy this condition:\(U(P_{\text{candidate}})>U(P_{\text{active}})+\text{Threshold}_{\text{eviction}}\)Remaining Utility (\(U(P_{\text{active}})\)): You must decay the utility of the active pair based on its temporal context. If its local graph shows its Leads edges are turning into Lags or Stale edges, its utility is actively decaying toward zero.Eviction Threshold: A structural buffer that prevents the system from hyper-switching (churning) back and forth between two similar pairs and burning capital on trading fees.

1. Categorizing the Graph Nodes for Strategy LogicTo compute utility without building manual heuristics, classify your CategoryType nodes into functional groups that interact with your edge contexts (Supports, Contradicts, etc.):Group A: High-Velocity Excitation (Reserved Slot Targets)These categories signify the sudden, violent injections of market energy typical of high-value pump-and-dumps.VerticalIgnitionFrenzyLiquidityShockAggressiveDriveLoadedImbalanceGroup B: Structural Compressors (Active Slot Setup Targets)These nodes represent high-probability setups where energy is storing up, ideal for your 2 standard active trading slots.CoiledCompressionLaminarResonanceOrganicTrendHiddenAbsorptionHardSupportGroup C: Fluid Sim & Systemic States (Context Anchors)These define the baseline medium through which the edges propagate their weights. They dictate how long a trade can safely mature before rotting.Laminar / TurbulentInertial / ViscousSystemicBeta / EndogenousAlpha / DecoupledAlphaEquilibriumGroup D: Structural Friction & Trap States (Utility Inhibitors)The presence of these nodes connected via Supports or Conditions edges acts as a massive penalty to the candidate pair's final utility score.SpoofTrapToxicBluffExhaustion / ThermalExhaustion / FadedExhaustionMechanicalCollapseBookThinning / LiquidityVacuum2. Calculating Utility for the 2 Active SlotsFor standard slots, utility relies on structural sustainability and alpha generation. You want high Endogenous Alpha or Coiled Compression moving through a Laminar state, with low systemic drag.Step 1: Compute Positive Structural MomentumSum the weights (\(W\)) of edges originating from target setup categories that connect to a favorable price-direction node via Supports or Leads:\(U_{\text{positive}}=W(\text{CoiledCompression})+W(\text{OrganicTrend})+W(\text{LaminarResonance})+W(\text{DecoupledAlpha})\)Step 2: Compute Structural FrictionSum the weights of edges pointing to decay or trap states, or noise categories that degrade signal quality factor (\(Q\)):\(U_{\text{negative}}=W(\text{Exhaustion})+W(\text{ToxicBluff})+W(\text{SpoofTrap})+W(\text{StochasticNoise})+W(\text{CausalNoise})\)Step 3: Active Slot Utility Score\(U_{\text{Active}}=U_{\text{positive}}-U_{\text{negative}}\)Admission Condition: If Active Slots are full, Candidate Pair \(A\) evicts Active Pair \(B\) if and only if \(U_{\text{Active}}(A) > U_{\text{Active}}(B) + \text{EvictionPenalty}\). Pair \(B\) will naturally see its score tank as its graph begins encoding nodes like FadedExhaustion, ThermalExhaustion, or AnchorStall.3. Calculating Utility for the 2 Reserved SlotsFor high-value/pump opportunities, your bouncer logic changes. You are not looking for clean trends; you are measuring violent structural imbalances where buyers are tearing through liquidity.Step 1: Measure Velocity and IgnitionThe utility score scales linearly with the strength of the volume and ignition nodes:\(U_{\text{Pump}}=W(\text{VerticalIgnition})+W(\text{Frenzy})+W(\text{LiquidityShock})+W(\text{ExtremeScarcity})\)Step 2: Check for Critical Structural FailureInstead of calculating a complex balance, look for immediate structural red flags. If any edge context points to these specific nodes, apply a massive penalty or drop the candidate entirely:An edge connecting VerticalIgnition to MechanicalCollapse via Leads.A strong Supports edge on Saturation or LiquidityVacuum.Admission & Eviction Condition: Reserved slots ignore macro data. If a new pair registers a massive \(W(\text{VerticalIgnition})\) within a localized TurbulentResonance and contains no MechanicalCollapse or SpoofTrap nodes, it is admitted. If slots are full, it evicts the active reserved pair whose graph shows its VerticalIgnition node has transitioned to a Lags relationship relative to FadedExhaustion or VolumeStarvation.4. Example: Processing an Admission RequestImagine your strategy is evaluating an incoming candidate pair graph while your 2 Active Slots are full:Read Topology: The strategy scans the candidate graph and finds strong Supports edges on HiddenAbsorption and CoiledCompression.Evaluate Context: The graph shows these nodes are occurring during a state of SynchronizedDrift with low StochasticNoise. This yields a high \(U_{\text{Active}}\) score.Scan Active Slots: Active Slot 1 is riding an OrganicTrend. Active Slot 2's graph just encoded an AnchorStall node which Conditions its primary trend, dragging its remaining utility down.Execute Eviction: Candidate Utility > Active Slot 2 Utility + Friction. Active Slot 2 is evicted; the candidate is admitted.