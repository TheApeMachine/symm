To visualize the **fluid signal** as a probability distribution across categories, we can map its primary microstructural metrics—**Reynolds Number ($Re$)**, **Divergence ($Div$)**, **Vorticity ($Vort$)**, and **Turbulence ($Turb$)**—against **Viscosity ($Visc$)**.

If the engine were to output a probability for the current "mechanical state" of the market, the categories would likely be:

### 1. Laminar Stability (Orderly Flow)
This category represents a healthy, predictable market environment.
*   **Probability Indicators:** High **Viscosity** (tight bid/ask spreads) coupled with low **Field Activity**.
*   **Semantic Meaning:** The "vapour pipe" of the market is at a constant, manageable diameter. Price moves are smooth, and the book is absorbing updates without churning.

### 2. Turbulent Chaos (Mechanical Breakdown)
This category identifies when the internal mechanics of the market are "shattering," often preceding a major regime shift.
*   **Probability Indicators:** Dominant **Turbulence** readings ($Turb$) and high **Vorticity** ($Vort$).
*   **Semantic Meaning:** This represents **"genuine microstructural chaos"** rather than just price volatility. The fractional differencing filter is detecting that the series is becoming non-stationary and losing its "memory" of previous levels.

### 3. Inertial Displacement (Directional Surge)
This category represents a high-energy move where the market is being forcibly "pushed" by one-sided order flow.
*   **Probability Indicators:** A high **Reynolds Number** ($Re$) and high **Divergence** ($Div$).
*   **Semantic Meaning:** The ratio of inertial forces to viscous forces has exploded. The market has high **information density**, meaning a massive quantum of activity is occurring within a single volume-clocked bar.

### 4. Viscous Resistance (The "Grind")
This category describes a market that is resisting movement, where price is "grinding against a wall."
*   **Probability Indicators:** Low **Viscosity** (wide spreads/high resistance) but moderate **Divergence** and high **Memory** (preserved via fractional differencing).
*   **Semantic Meaning:** The market is "thick" or viscous. Every tick move requires a massive amount of "work" (traded volume), but the signal remembers that price has been exhausted at this level for a long duration.

### Summary of the Probability Set
A visualization would show the probability shifting between these four states based on the **Field Activity** (which takes the maximum absolute value of the four fluid dynamics) and the **Viscosity** (the inverse of the spread):

| Category      | Visc (Spread) | Dominant Metric            | Market "Feel"      |
|:--------------|:--------------|:---------------------------|:-------------------|
| **Laminar**   | High (Tight)  | None (Low Activity)        | Smooth/Consistent  |
| **Turbulent** | Variable      | **Turbulence / Vorticity** | Shattered/Fragile  |
| **Inertial**  | Moderate      | **Reynolds / Divergence**  | Direct/Heavy       |
| **Viscous**   | Low (Wide)    | **Divergence (at walls)**  | Resistant/Grinding |

---

The next signal to map is the **Hawkes signal**, which focuses on the **Trade-Cluster Excitation** perspective. While the Fluid signal looks at the "vapour pipe" of the book, Hawkes looks at the "temperature" and "chain reactions" of the trade arrivals themselves.

### 1. What it measures exactly (in isolation)
The **Hawkes signal** measures the **self-excitation and clustering** of trade arrivals using a bivariate mathematical model. It determines if trades are triggering subsequent trades in a feedback loop, rather than just occurring as isolated, random events.

It isolates the following mathematical components:
*   **Exogenous Base ($\mu$):** The "background" rate of trades—those arriving from outside factors like news or random organic activity.
*   **Branching Ratio ($\alpha$):** The endogenous feedback factor. It measures the "descendant" trades likely to be triggered by a single "parent" trade.
*   **Intensity ($\lambda$):** The current instantaneous rate of trade arrivals for the buy side versus the sell side.
*   **Spectral Radius ($\rho$):** A measure of system stability. As the radius approaches the **"critical branch" (1.0)**, the trade-flow feedback loop becomes explosive and unstable.
*   **Asymmetry:** The net difference between current buy and sell intensities, further confirmed by top-of-book imbalance.

---

### 2. Semantically, what story does it tell?
The Hawkes signal tells the story of **momentum consistency and market "criticality."**

*   **The "Chain Reaction" Story:** Unlike simple volume, Hawkes asks: "Is this trade a lonely event, or the spark of a larger fire?". It distinguishes between a high-volume spike and **genuine momentum ignition**.
*   **The "Boiling Point" Story:** Using the **Spectral Radius**, it identifies when the market is reaching a state of mechanical instability. It tells the story of a market becoming so "hot" that feedback loops are saturating, making a major break imminent.
*   **The "Consensus" Story:** It identifies the difference between a one-sided frenzy and a high-intensity tug-of-war. It can tell when buyers and sellers are both "excited," which signals a high-energy collision of interest.

---

### 3. Probability Visualization Categories
To map this signal into a "perspective," we can visualize the probability across these four mechanical states:

#### **1. Consensus Frenzy (Directional clustering)**
One side of the market has taken complete control of the feedback loop.
*   **Indicators:** High **Asymmetry** and moderate **Spectral Radius**, with one intensity far exceeding its background $\mu$.
*   **Semantic Meaning:** One side is aggressively hitting the book and triggering a chain reaction of subsequent trades.

#### **2. Contested Saturation (Critical instability)**
The market is at its absolute limit of mechanical stability.
*   **Indicators:** Very high **Spectral Radius** ($>0.85$) and high **Intensity** on *both* sides.
*   **Semantic Meaning:** The market is "boiling." Both buyers and sellers are highly active and exciting each other. The system is "super-critical" and likely to break violently once one side exhausts.

#### **3. Exogenous Drift (Orderly flow)**
The default state where trades arrive but do not trigger significant cascades.
*   **Indicators:** Low **Spectral Radius** and intensities staying close to their background **$\mu$** levels.
*   **Semantic Meaning:** Trades are driven by outside factors rather than internal market feedback. The "engine" is running cool and predictably.

#### **4. Flow Exhaustion (Thermal death)**
The trade flow has effectively stalled.
*   **Indicators:** Current intensities falling significantly below historical background **$\mu$**.
*   **Semantic Meaning:** The feedback loops have died out, and even organic interest has slowed. The current move has "run out of steam."

### Summary of Hawkes Categories

| Category       | Spectral Radius   | Asymmetry    | Market "Feel"          |
|:---------------|:------------------|:-------------|:-----------------------|
| **Frenzy**     | Moderate          | High         | Aggressive/Directional |
| **Saturation** | High ($ \to 1.0$) | Low/Moderate | Contested/Unstable     |
| **Organic**    | Low               | Low          | Healthy/Quiet          |
| **Exhaustion** | Very Low          | Low          | Stalled/Dying          |

By mapping Hawkes this way, the engine can distinguish between a move that is **smoothly supported (Frenzy)** and one that is **dangerously overheated (Saturation)**.

---

For our next analysis, let’s map the **Causal signal**. This signal is the engine’s "rationalist," moving beyond simple correlations to identify the true structural drivers of price using Judea Pearl’s "ladder of causation".

### 1. What it measures exactly (in isolation)
The Causal signal measures the **structural relationship** between Macro Momentum, Liquidity, Local Flow, and Price Velocity. It uses a **Directed Acyclic Graph (DAG)** to determine if a price move is an independent event or just a symptom of broader market drift.

It isolates the following causal rungs and metrics:
*   **Rung 1: Association:** Measures simple observational correlation ($P(velocity | flow)$).
*   **Rung 2: Intervention:** Uses **backdoor adjustment** to calculate the effect of "doing" a trade ($P(velocity | do(flow))$) while controlling for macro and liquidity.
*   **Rung 3: Counterfactual Uplift:** Determines what the price move *would have been* if the order flow were different than observed.
*   **Structural Regimes:** It dynamically switches roles based on market health. In **Normal** conditions, "Flow" is the driver; in **Panic** conditions (detected via cross-asset **Contagion** or collinearity), "Liquidity" itself becomes the driver.

---

### 2. Semantically, what story does it tell?
The Causal signal tells the story of **responsibility and origin.**

*   **The "Local vs. Global" Story:** It asks: "Is this asset moving because it's special right now, or because everything is moving?". It filters out "Macro Drift" to find genuine local alpha.
*   **The "Weaponized Liquidity" Story:** It identifies a specific type of market terror where makers pull quotes so aggressively that the **sudden void itself drives price**, while trades merely lag into it.
*   **The "Fragile Equilibrium" Story:** By monitoring the **Condition Number** of the correlation matrix, it tells the story of a market where flow and liquidity have collapsed onto a single axis, meaning the structural edges are no longer identifiable and a regime break is imminent.

---

### 3. Probability Visualization Categories
To map this into a "perspective," we can visualize the probability across these four structural states:

#### **1. Endogenous Alpha (The Leader)**
The price is being driven by local, internal buying or selling pressure.
*   **Indicators:** High **Counterfactual Uplift** within the **Normal (Flow)** regime.
*   **Semantic Meaning:** The move is "authentic." The local order flow is the primary cause of price velocity, independent of the rest of the market.

#### **2. Systemic Beta (The Drifter)**
The price is moving, but it has no internal driver; it is simply following the tide.
*   **Indicators:** High **Association** (Rung 1) but near-zero **Intervention Effect** (Rung 2).
*   **Semantic Meaning:** The asset is just a passenger. The "cause" is **Macro Momentum**, and there is no unique structural reason to favor this specific symbol over the index.

#### **3. Liquidity Shock (The Panic)**
The internal mechanics have inverted; the absence of liquidity is now the dominant force.
*   **Indicators:** **Panic Regime** roles active, triggered by a **Contagion** spike toward 1.0 or an exploding **Condition Number**.
*   **Semantic Meaning:** The market is "hollow." Makers have pulled back, and the resulting void is sucking price in. This is a high-risk state where trade flow is a lagging indicator.

#### **4. Causal Noise (The Equilibrium)**
No single force—local or macro—has a clear grip on price movement.
*   **Indicators:** Low confidence across all causal rungs and high residuals in the **Non-Linear Model**.
*   **Semantic Meaning:** The market is in a state of stochastic equilibrium. Neither buyers, sellers, nor the broader macro environment are providing a statistically significant "push."

### Summary of Causal Categories

| Category             | Active Regime | Dominant Factor           | Market "Feel"      |
|:---------------------|:--------------|:--------------------------|:-------------------|
| **Endogenous Alpha** | Normal        | **Counterfactual Uplift** | Driven/Independent |
| **Systemic Beta**    | Normal        | **Macro Momentum**        | Drifting/Passive   |
| **Liquidity Shock**  | Panic         | **Liquidity Void**        | Fragile/Inverted   |
| **Causal Noise**     | Variable      | None                      | Stochastic/Unclear |

By combining this with the **Fluid** (mechanical health) and **Hawkes** (thermal excitation) signals, the engine can distinguish between a move that is **excited and healthy (Hawkes Frenzy + Fluid Laminar)** but **causally empty (Systemic Beta)**, versus a move that is **structurally significant (Endogenous Alpha).**

---

The next signal to map is **CVD (Cumulative Volume Delta)**, which provides the "absorption" perspective. While the Fluid and Hawkes signals look at the mechanics and temperature of the book, CVD focuses on the **truth of executed volume** to see if a price move is being supported or secretly resisted.

### 1. CVD Signal: The Absorption Perspective

#### **What it measures exactly (in isolation)**
The **CVD signal** measures the net difference between aggressor-buy volume and aggressor-sell volume over a 15-minute window. It specifically looks for a divergence between **executed flow** and **price drift**.

*   **Net Fraction:** The ratio of net volume (buys minus sells) to gross volume (total trades). A directional read requires a fraction of at least **0.60**.
*   **Price Suppression:** It measures if the price is staying within a "flat band" ($\leq 0.3\%$) despite heavy one-sided buying or selling.
*   **Tick Integrity:** Because it reads the executed trade tape rather than the book, it is immune to spoofing.

#### **Semantically, what story does it tell?**
*   **The "Iceberg" Story:** It identifies when a massive participant is "hidden" in the book, absorbing every market order without letting the price move. It tells us that what looks like a range-bound market is actually a site of **heavy accumulation or distribution**.
*   **The "Authentic Move" Story:** It verifies price trends. If price is rising but CVD is flat or negative, the move is a "trap" or "low-conviction." If price and CVD move together, the trend is **structurally supported**.

#### **Probability Visualization Categories**

| Category               | Net Volume | Price Drift | Market "Feel"                    |
|:-----------------------|:-----------|:------------|:---------------------------------|
| **Hidden Absorption**  | High       | Flat        | **Bullish/Bearish Iceberg**      |
| **Aggressive Drive**   | High       | High        | **Strong Trend Support**         |
| **Stochastic Balance** | Low        | Variable    | **Equilibrium/Choppy**           |
| **Volume Starvation**  | Very Low   | Flat        | **Dying Interest (< 40 trades)** |

---

### 2. DepthFlow Signal: The "Weight of the Book" Perspective

#### **What it measures exactly (in isolation)**
The **DepthFlow signal** measures the **asymmetry of intent** by looking at multiple levels of the order book, weighted by their distance from the mid-price.

*   **Weighted Depth Imbalance (WBI):** Applies an exponential decay kernel ($exp(-\lambda \cdot d)$) to levels. Deep "spoof" walls are down-weighted, while liquidity near the touch is prioritized.
*   **Toxic Filter:** It actively subtracts "toxic" levels—large, young blocks near the touch that are frequently cancelled rather than filled—from the imbalance calculation.
*   **Trade Pressure EMA:** Integrates recent trade sides into a running pressure index to see if the book imbalance is actually resulting in trades.
*   **Spoof Skew:** Specifically flags when deep-book volume contradicts the touch (e.g., a massive buy wall exists while the top-of-book is being sold into).

#### **Semantically, what story does it tell?**
*   **The "Structural Wall" Story:** It identifies when the "gravity" of the book is pulling price in a certain direction. 
*   **The "Spoofing" Story:** Using the **SpoofSkew** metric, it warns the engine when a side is trying to "fake" depth to lure other participants into a trap.
*   **The "Book Decay" Story:** It tells the story of **exhaustion**. By tracking `book_thinning` and `spread_widen` events, it identifies when a side's defensive walls are crumbling.

#### **Probability Visualization Categories**

| Category             | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"            |
|:---------------------|:-------------------------|:------------------|:-------------------------|
| **Loaded Imbalance** | High                     | High (Agrees)     | **Structural Gravity**   |
| **Spoof Trap**       | High                     | Low (Contradicts) | **Manipulated/Fake**     |
| **Book Thinning**    | Rapidly Falling          | Variable          | **Exhaustion/Crumbling** |
| **Dense Neutrality** | Balanced                 | Low               | **Robust Stability**     |

---

### 3. LeadLag Signal: The "Anchor" Perspective

#### **What it measures exactly (in isolation)**
The **LeadLag signal** measures the **temporal correlation** between a "leader" asset (typically BTC/EUR) and the rest of the market.

*   **Cross-Lag Correlation:** It doesn't just look at if they are moving together, but **by how many bars one is leading the other**.
*   **Anchor Threshold:** It only activates when the anchor moves significantly ($\geq 0.05\%$).
*   **Lag Fraction:** Measures what percentage of the leader's move the follower has yet to complete.

#### **Semantically, what story does it tell?**
*   **The "Inefficiency" Story:** It finds "free money" by identifying altcoins that have a high statistical probability of following BTC but haven't "woken up" yet.
*   **The "Beta Drift" Story:** It identifies symbols that have no unique alpha of their own and are simply being dragged along by the market tide.

#### **Probability Visualization Categories**

| Category               | Lead/Lag Correlation | Lag Fraction | Market "Feel"             |
|:-----------------------|:---------------------|:-------------|:--------------------------|
| **Inefficient Lag**    | High                 | High         | **Catch-up Opportunity**  |
| **Synchronized Drift** | High                 | Low          | **Systemic Beta**         |
| **Decoupled Move**     | Low                  | N/A          | **Idiosyncratic Alpha**   |
| **Anchor Stall**       | Low                  | Low          | **Leadership Exhaustion** |

---

The next signals to map are **PumpDump**, **Liquidity (Basis)**, **Sentiment**, and **Toxicity (BookFlow)**. These signals complete the engine’s internal model by looking for explosive moves, cross-sectional scarcity, global market conviction, and "fake" liquidity.

### 4. PumpDump Signal: The Ignition Perspective

#### **What it measures exactly (in isolation)**
The **PumpDump signal** identifies pre-pump microstructure by looking for sudden "verticality" in volume and price.
*   **Volume Lift (RVOL):** Measures fast and medium-term volume spikes against a median hourly baseline.
*   **Precursor Move:** Uses a $PositiveMove$ dynamic to score how much the price has already begun to detach from its recent anchor.
*   **Spread Compression:** Scores how much the bid/ask spread has tightened versus its own baseline.
*   **Move Classifier:** A state-free primitive that maps these metrics into an explicit "Pump" or "Dump" class.

#### **Semantically, what story does it tell?**
*   **The "Ignition" Story:** It identifies the exact moment a move stops being random walk and becomes a vertical event driven by abnormal volume "lift".
*   **The "Coiled Spring" Story:** By tracking spread compression and book-side strength, it identifies when a market is "tightly wound" and ready to snap.

#### **Probability Visualization Categories**

| Category               | Volume Lift | Price Precursor | Market "Feel"            |
|:-----------------------|:------------|:----------------|:-------------------------|
| **Vertical Ignition**  | High Spike  | High            | **Launching / Breakout** |
| **Coiled Compression** | Moderate    | Low             | **Pre-Pump / Loaded**    |
| **Organic Trend**      | Low/Steady  | Moderate        | **Healthy Momentum**     |
| **Faded Exhaustion**   | Falling     | Flat            | **Leg is Dead**          |

---

### 5. Liquidity (Basis) Signal: The Scarcity Perspective

#### **What it measures exactly (in isolation)**
The **Liquidity signal** identifies opportunities in "thin" markets by ranking a symbol’s volume against the broader market.
*   **Cross-Section Ranking:** Ranks the $dailyQuoteVol$ of all subscribed symbols.
*   **Illiquidity Score:** Specifically identifies symbols trading strictly below the cross-section median of their peers.
*   **Peak Scarcity:** Uses a **Peak gate** to find symbols that are currently the most illiquid in the universe.

#### **Semantically, what story does it tell?**
*   **The "Convexity" Story:** It signals where a small amount of order flow will cause the largest price displacement. It finds the "thinnest" pipes in the exchange where price can move most easily.
*   **The "Neglect" Story:** It identifies assets that are being ignored by the broader market, making them prime targets for sudden volatility once a trade actually arrives.

#### **Probability Visualization Categories**

| Category             | Rank vs. Peers   | Volume   | Market "Feel"                |
|:---------------------|:-----------------|:---------|:-----------------------------|
| **Extreme Scarcity** | Peak Illiquidity | Very Low | **High Convexity / Fragile** |
| **Median Depth**     | Middle           | Normal   | **Standard Efficiency**      |
| **Robust Liquidity** | Bottom (Deep)    | High     | **Efficient / Safe**         |

---

### 6. Sentiment Signal: The Bullish Breadth Perspective

#### **What it measures exactly (in isolation)**
The **Sentiment signal** measures global market conviction by looking at the behavior of the entire universe simultaneously.
*   **Market Breadth:** The ratio of symbols with a positive $changePct$ versus the total number of symbols.
*   **Leadership Performance:** Tracks the median performance of the "top" symbols to see if the leaders are actually leading.

#### **Semantically, what story does it tell?**
*   **The "Rising Tide" Story:** It tells you if an asset's move is a solo effort or if it is being carried by a global "risk-on" regime where every asset is moving in unison.
*   **The "Conviction" Story:** It distinguishes between a "fake" leader move (where only one asset is up) and a high-conviction market environment (breadth $> 0.55$).

#### **Probability Visualization Categories**

| Category           | Breadth        | Leader Strength | Market "Feel"                |
|:-------------------|:---------------|:----------------|:-----------------------------|
| **Risk-On Surge**  | High ($>0.55$) | Strong          | **Rising Tide / Global Buy** |
| **Divergent Move** | Low            | Strong          | **Idiosyncratic Alpha**      |
| **Systemic Slump** | Low            | Weak            | **Global Risk-Off**          |

---

### 7. Toxicity (BookFlow) Signal: The Quality Perspective

#### **What it measures exactly (in isolation)**
The **Toxicity signal** analyzes the "honesty" of the book by tracking how makers behave when a trade approaches.
*   **Cancel-to-Fill Asymmetry:** Measures the ratio of liquidity being "pulled" (cancelled) versus liquidity being "hit" (filled).
*   **Toxic Level Detection:** Flags large, young, near-touch blocks that disappear rather than fill—this is the signature of a bluff.
*   **Directional BookFlow:** Emits a directional read based on which side of the book is "retreating" (vacuum effect).

#### **Semantically, what story does it tell?**
*   **The "Bluffing" Story:** It exposes makers who are "fake-bidding" to create an illusion of support, warning the engine that a wall is not "real" and will crumble upon contact.
*   **The "Vacuum" Story:** It identifies a "liquidity vacuum" where one side pulls away so aggressively that the resulting void "sucks" the price in that direction.

#### **Probability Visualization Categories**

| Category             | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:---------------------|:------------------|:----------------|:-----------------------|
| **Liquidity Vacuum** | High Asymmetry    | One Side        | **Vacuum Surcharge**   |
| **Toxic Bluff**      | High              | Near-Touch      | **Manipulated / Fake** |
| **Hard Support**     | Low (High Fill)   | None            | **Robust / Sincere**   |

---

That’s right—we are down to the final two specialized perspectives: **Correlation** and **Exhaust**. While the others focus on "Why" and "When" to enter, these two focus on **"Systemic Health"** and **"The Exit Strategy."**

### 8. Correlation Signal: The "Herd Behavior" Perspective

#### **What it measures exactly (in isolation)**
The **Correlation signal** measures **synchronized return correlation** across the subscribed universe using a rolling window of log-returns. It determines if the market is moving as a single, indistinguishable block or if individual assets are exhibiting unique behavior.

*   **Synchronized Log-Returns:** It aligns price windows onto a shared time grid (e.g., 10-second bars) to calculate the Pearson correlation between pairs.
*   **Peak Score:** It identifies symbols that are hitting a "peak" in their correlation to the broader market, using an adaptive peak gate.
*   **Hayashi-Yoshida Fallback:** For high-frequency, asynchronous data where trades don't align perfectly on time bars, it uses the H-Y estimator to capture overlapping return intervals.

#### **Semantically, what story does it tell?**
*   **The "Rising Tide" Story:** It asks: "Is this asset special, or is it just being dragged along by the herd?". High correlation indicates that macro-systemic forces are dominant.
*   **The "De-coupling" Story:** It identifies "alpha" opportunities by spotting when an asset stops following its peers, suggesting a local catalyst is at play.
*   **The "Liquidation" Story:** Sudden spikes in cross-asset correlation toward **1.0** often signal systemic panics or liquidation cascades where everything is sold at once.

#### **Probability Visualization Categories**

| Category             | Correlation Level | Variance | Market "Feel"                           |
|:---------------------|:------------------|:---------|:----------------------------------------|
| **Systemic Herd**    | High ($> 0.85$)   | High     | **Global Beta / Momentum Drift**        |
| **Decoupled Alpha**  | Low               | High     | **Unique Driver / Leading Move**        |
| **Stochastic Noise** | Low               | Low      | **Quiet / Indecisive**                  |
| **Divergent Stress** | Negative          | High     | **Contrarian Move / Relative Weakness** |

---

### 9. Exhaust Signal: The "Exit Thesis" Perspective

#### **What it measures exactly (in isolation)**
The **Exhaust signal** tracks **microstructure decay** to advise on the urgency of closing an open position. Unlike entry signals that look for momentum ignition, Exhaust looks for **momentum rot**.

*   **Book Thinning:** Measures the trend of bid/ask depth; if depth is disappearing as price moves, the move is "hollow".
*   **Pressure Fade:** Tracks the decay in trade pressure EMA; it signals when the aggressive "hitters" have run out of ammunition.
*   **Spread Widening:** Monitors the bid/ask spread; widening spreads during a trend indicate increasing mechanical resistance and risk.
*   **Imbalance Flip:** Detects when the "weight" of the book flips from the support side to the resistance side.

#### **Semantically, what story does it tell?**
*   **The "Party is Over" Story:** It identifies the exact moment a trend stops being supported by fresh liquidity and begins to rely on "fumes."
*   **The "Trap" Story:** By spotting **Book Thinning** while price is still rising, it warns that the bid wall is crumbling and a sharp reversal is imminent.
*   **The "Thesis Fade" Story:** It provides a "Thesis-decay" exit—if the reason you entered (imbalance and pressure) is gone, it closes the trade even if the stop-loss hasn't been hit yet.

#### **Probability Visualization Categories**

| Category                | Primary Metric     | Urgency  | Market "Feel"                        |
|:------------------------|:-------------------|:---------|:-------------------------------------|
| **Mechanical Collapse** | **Book Thinning**  | High     | **Crumbling Walls / Flash-Risk**     |
| **Thermal Exhaustion**  | **Pressure Fade**  | Moderate | **Dying Momentum / Topping Out**     |
| **Fragile Expansion**   | **Spread Widen**   | Moderate | **Increasing Friction / Risky Hold** |
| **Active Reversal**     | **Imbalance Flip** | High     | **Sentiment Flip / Counter-Attack**  |

---

### Summary of the Complete Engine
With these mapped, the engine now has a full "360-degree" view of every symbol:
1.  **Fluid & Hawkes:** How is the "engine" of the book running? (Mechanical/Thermal)
2.  **Causal & LeadLag:** Who is responsible for this move? (Source/Anchor)
3.  **CVD & Toxicity:** Is this move "honest"? (Absorption/Bluffing)
4.  **PumpDump & Liquidity:** Is this an explosive opportunity? (Ignition/Scarcity)
5.  **Sentiment & Correlation:** Is the rest of the world helping? (Breadth/Beta)
6.  **Exhaust:** Is it time to leave? (Decay)

---

### 1. Fluid Signal: The Mechanical Perspective

| Category      | Visc (Spread) | Dominant Metric            | Market "Feel"      |
|:--------------|:--------------|:---------------------------|:-------------------|
| **Laminar**   | High (Tight)  | None (Low Activity)        | Smooth/Consistent  |
| **Turbulent** | Variable      | **Turbulence / Vorticity** | Shattered/Fragile  |
| **Inertial**  | Moderate      | **Reynolds / Divergence**  | Direct/Heavy       |
| **Viscous**   | Low (Wide)    | **Divergence (at walls)**  | Resistant/Grinding |

---

### 2. Hawkes Signal: The Thermal Perspective

| Category       | Spectral Radius   | Asymmetry    | Market "Feel"          |
|:---------------|:------------------|:-------------|:-----------------------|
| **Frenzy**     | Moderate          | High         | Aggressive/Directional |
| **Saturation** | High ($ \to 1.0$) | Low/Moderate | Contested/Unstable     |
| **Organic**    | Low               | Low          | Healthy/Quiet          |
| **Exhaustion** | Very Low          | Low          | Stalled/Dying          |

---

### 3. CVD Signal: The Absorption Perspective

| Category             | Active Regime | Dominant Factor           | Market "Feel"      |
|:---------------------|:--------------|:--------------------------|:-------------------|
| **Endogenous Alpha** | Normal        | **Counterfactual Uplift** | Driven/Independent |
| **Systemic Beta**    | Normal        | **Macro Momentum**        | Drifting/Passive   |
| **Liquidity Shock**  | Panic         | **Liquidity Void**        | Fragile/Inverted   |
| **Causal Noise**     | Variable      | None                      | Stochastic/Unclear |

---

### 4. CVD Signal: The Absorption Perspective

| Category               | Net Volume | Price Drift | Market "Feel"                    |
|:-----------------------|:-----------|:------------|:---------------------------------|
| **Hidden Absorption**  | High       | Flat        | **Bullish/Bearish Iceberg**      |
| **Aggressive Drive**   | High       | High        | **Strong Trend Support**         |
| **Stochastic Balance** | Low        | Variable    | **Equilibrium/Choppy**           |
| **Volume Starvation**  | Very Low   | Flat        | **Dying Interest (< 40 trades)** |

---

### 5. DepthFlow Signal: The "Weight of the Book" Perspective

| Category             | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"            |
|:---------------------|:-------------------------|:------------------|:-------------------------|
| **Loaded Imbalance** | High                     | High (Agrees)     | **Structural Gravity**   |
| **Spoof Trap**       | High                     | Low (Contradicts) | **Manipulated/Fake**     |
| **Book Thinning**    | Rapidly Falling          | Variable          | **Exhaustion/Crumbling** |
| **Dense Neutrality** | Balanced                 | Low               | **Robust Stability**     |

---

### 6. LeadLag Signal: The "Anchor" Perspective

| Category               | Lead/Lag Correlation | Lag Fraction | Market "Feel"             |
|:-----------------------|:---------------------|:-------------|:--------------------------|
| **Inefficient Lag**    | High                 | High         | **Catch-up Opportunity**  |
| **Synchronized Drift** | High                 | Low          | **Systemic Beta**         |
| **Decoupled Move**     | Low                  | N/A          | **Idiosyncratic Alpha**   |
| **Anchor Stall**       | Low                  | Low          | **Leadership Exhaustion** |

---

### 7. PumpDump Signal: The Ignition Perspective

| Category               | Volume Lift | Price Precursor | Market "Feel"            |
|:-----------------------|:------------|:----------------|:-------------------------|
| **Vertical Ignition**  | High Spike  | High            | **Launching / Breakout** |
| **Coiled Compression** | Moderate    | Low             | **Pre-Pump / Loaded**    |
| **Organic Trend**      | Low/Steady  | Moderate        | **Healthy Momentum**     |
| **Faded Exhaustion**   | Falling     | Flat            | **Leg is Dead**          |

---

### 8. Liquidity (Basis) Signal: The Scarcity Perspective

| Category             | Rank vs. Peers   | Volume   | Market "Feel"                |
|:---------------------|:-----------------|:---------|:-----------------------------|
| **Extreme Scarcity** | Peak Illiquidity | Very Low | **High Convexity / Fragile** |
| **Median Depth**     | Middle           | Normal   | **Standard Efficiency**      |
| **Robust Liquidity** | Bottom (Deep)    | High     | **Efficient / Safe**         |

---

### 9. Sentiment Signal: The Bullish Breadth Perspective

| Category           | Breadth        | Leader Strength | Market "Feel"                |
|:-------------------|:---------------|:----------------|:-----------------------------|
| **Risk-On Surge**  | High ($>0.55$) | Strong          | **Rising Tide / Global Buy** |
| **Divergent Move** | Low            | Strong          | **Idiosyncratic Alpha**      |
| **Systemic Slump** | Low            | Weak            | **Global Risk-Off**          |

---

### 10. Toxicity (BookFlow) Signal: The Quality Perspective

| Category             | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:---------------------|:------------------|:----------------|:-----------------------|
| **Liquidity Vacuum** | High Asymmetry    | One Side        | **Vacuum Surcharge**   |
| **Toxic Bluff**      | High              | Near-Touch      | **Manipulated / Fake** |
| **Hard Support**     | Low (High Fill)   | None            | **Robust / Sincere**   |

---

### 11. Correlation Signal: The "Herd Behavior" Perspective

| Category             | Correlation Level | Variance | Market "Feel"                           |
|:---------------------|:------------------|:---------|:----------------------------------------|
| **Systemic Herd**    | High ($> 0.85$)   | High     | **Global Beta / Momentum Drift**        |
| **Decoupled Alpha**  | Low               | High     | **Unique Driver / Leading Move**        |
| **Stochastic Noise** | Low               | Low      | **Quiet / Indecisive**                  |
| **Divergent Stress** | Negative          | High     | **Contrarian Move / Relative Weakness** |

---

### 12. Exhaust Signal: The "Exit Thesis" Perspective

| Category                | Primary Metric     | Urgency  | Market "Feel"                        |
|:------------------------|:-------------------|:---------|:-------------------------------------|
| **Mechanical Collapse** | **Book Thinning**  | High     | **Crumbling Walls / Flash-Risk**     |
| **Thermal Exhaustion**  | **Pressure Fade**  | Moderate | **Dying Momentum / Topping Out**     |
| **Fragile Expansion**   | **Spread Widen**   | Moderate | **Increasing Friction / Risky Hold** |
| **Active Reversal**     | **Imbalance Flip** | High     | **Sentiment Flip / Counter-Attack**  |

---

Alright, so I put these tables underneath each other, and I think that what "perspectives" should probably be is like a desicion making chart, how do you call these? You know where you have nodes and branches, and you just follow along based on the questions in the nodes, and the branch that has the currently most correct answer. These branches should then terminate with the most logical action to take (or no action if that is the most logical). It could be that we may have to use the same signal category in multiple places somewhere, this is really just about what do we need to know at which point, or better, which question should we be asking at which point and which answers (categories) could answer those questions.

---

This design makes perfect sense; in the SYMM architecture, "perspectives" are effectively the layer that fuses raw microstructural measurements into a unified market view to authorize a prediction. By treating these as a **decision-making chart** (often called a **Decision Tree** or **Logic Flow**), you can move from noisy signal categories to a concrete "Verdict" of **Allow**, **Deny**, or **Wait**.

Below is a design for such a flow, structured by the questions the engine must answer at each stage of the trade lifecycle.

### Stage 1: The Systemic Filter (Is the world helping?)
Before looking at a specific symbol, the engine asks if the global environment is conducive to the current strategy.

*   **Node 1: Is there global conviction?**
    *   **Branch [Sentiment: Risk-On Surge]:** Continue to Local Alpha.
    *   **Branch [Sentiment: Systemic Slump]:** **Action: No Action (Wait for Breadth)**.
*   **Node 2: Is the market moving as a single block?**
    *   **Branch [Correlation: Decoupled Alpha]:** High conviction for idiosyncratic moves; continue.
    *   **Branch [Correlation: Systemic Herd]:** Increase risk dampening (Systemic Beta risk); proceed with caution.

### Stage 2: The Origin Check (Who is driving this?)
This stage uses the "rationalist" signals to ensure the move is authentic and not just macro noise.

*   **Node 3: Is this move independent or a passenger?**
    *   **Branch [Causal: Endogenous Alpha]:** Valid local driver; continue.
    *   **Branch [Causal: Systemic Beta]:** **Action: No Action (Asset is a Drifter)**.
*   **Node 4: Is there a leadership lag?**
    *   **Branch [LeadLag: Inefficient Lag]:** High-probability "Catch-up" setup; prioritize entry.
    *   **Branch [LeadLag: Anchor Stall]:** Move may be exhausted; **Action: Wait**.

### Stage 3: The Quality Check (Is the liquidity sincere?)
This stage filters out manipulation and hidden barriers.

*   **Node 5: Are the walls real?**
    *   **Branch [Toxicity: Hard Support] + [DepthFlow: Loaded Imbalance]:** Strong structural support; continue.
    *   **Branch [Toxicity: Toxic Bluff] or [DepthFlow: Spoof Trap]:** **Action: Deny (Manipulation Detected)**.
*   **Node 6: Is there hidden resistance?**
    *   **Branch [CVD: Hidden Absorption]:** A massive "Iceberg" is being exhausted; prepare for breakout.
    *   **Branch [CVD: Aggressive Drive]:** Trend is confirmed by tape; continue.

### Stage 4: The Timing Node (Is the engine running hot?)
The final gate checks the mechanical and thermal state of the book for the exact entry moment.

*   **Node 7: Is the flow orderly?**
    *   **Branch [Fluid: Laminar Stability] or [Inertial Displacement]:** Mechanically healthy; continue.
    *   **Branch [Fluid: Turbulent Chaos]:** **Action: Wait (Mechanical Breakdown)**.
*   **Node 8: Is the chain reaction ignited?**
    *   **Branch [Hawkes: Frenzy]:** Momentum is consistent; **Action: ALLOW ENTRY**.
    *   **Branch [Hawkes: Saturation]:** **Action: Deny (Dangerously Overheated)**.

### Stage 5: The Exit Thesis (Is it time to go?)
Once in a position, the perspective shifts to monitoring for "momentum rot".

*   **Node 9: Is the thesis still valid?**
    *   **Branch [Exhaust: Mechanical Collapse] or [Active Reversal]:** **Action: URGENT EXIT**.
    *   **Branch [Exhaust: Thermal Exhaustion]:** **Action: SOFT EXIT (Harvest Profits)**.
    *   **Branch [Fluid: Viscous Resistance]:** Thesis is "grinding"; **Action: Hold (Monitor Runway)**.

---

Let's also consider that entries and exits are not fundamentally different strategies from each other. Sure, before entering a position we must of course ask if there is enough upwards momentum sustained to get us past the point of slippage, spread, and fees to make the trade worth entering, and for exits we are likely more trying to see if momentum indeed persists (though we should of course not exit at the first sign of a bit of drop, since the market fluctuates, so historical context matters here), but ultimately all the signals are just as relevant for the exit strategy as they are for the enter strategy. And about Stage 1, you mentioned "the current strategy" but that is something we are still determining at that point. The strategy is both dynamic, and running multiples in parallel. While one coin might be pump and dumping at the moment, another one might be legitimately rising in value because of some event. We want to obviously capitalize on whatever opportunities we are presented with.

---

That is a profound insight into the engine's design—the idea that **entry and exit are not separate strategies**, but rather a continuous re-evaluation of the same "thesis." In the SYMM architecture, this is explicitly handled by the **`Perspective`** and the **`ExitReasonEdgeFaded`** logic.

Here is how we can refine the decision-making flow to reflect this unified, multi-strategy approach:

### 1. Strategic Pluralism (Stage 1 Refinement)
Instead of filtering for a single strategy, the **Systemic Filter** identifies the "menu of opportunities" available in the current environment. Because the engine uses **atomic state containers (`Cell`)**, it can process dozens of symbols in true parallel.
*   **The "Rising Tide" (Sentiment > 0.55):** Authorizes "Organic Trend" and "LeadLag Catch-up" strategies across the universe.
*   **The "Localized Ignition" (PumpDump Spikes):** Operates independently of market breadth, allowing the engine to capitalize on a solo "Pump" even during a systemic slump.
*   **Parallel Regimes:** A single symbol can transition from a **Causal "Flow" regime** to a **"Liquidity" (Panic) regime** instantly if structural edges collapse.

### 2. The Unified Thesis: "Allow" vs. "Persist"
As you noted, the signals don't stop once you enter. The engine moves from asking **"Should I Allow Entry?"** to **"Does the Thesis Persist?"** using the same 360-degree view:

*   **Momentum vs. Friction:** For entries, the **`entryReturnRequirement`** calculates if the predicted return clears $2\times$ the round-trip friction (slippage + fees).
*   **Thesis Decay (`ExitReasonEdgeFaded`):** Once in a position, the engine performs a **live re-read of the `Perspective`**. If the direction flips or the forward-return estimate goes negative, the "reason we entered is gone" and the position is closed immediately.
*   **Historical Buffering:** To prevent exiting on minor fluctuations, the engine uses **`MinExhaustHold`**, which suppresses "soft" microstructure exits (like minor spread widening) for the first few seconds to let the trade clear its entry fee.

### 3. The "Persistence" Decision Tree
We can visualize the logic as a loop where the "Terminator" nodes (Actions) feed back into the start:

| Phase | Question (The Signal Check) | Decision (Entry Logic) | Decision (Exit Logic) |
| :--- | :--- | :--- | :--- |
| **Mechanical** | Is the book flow laminar? [Fluid] | **Wait** if turbulent chaos | **Hold** if viscous "grind" |
| **Thermal** | Is the excitement sustained? [Hawkes] | **Enter** on Frenzy | **Exit** on Saturation ($ \to 1.0$) |
| **Causal** | Is the local driver still active? [Causal] | **Allow** if Endogenous | **Exit** if it becomes Systemic Beta |
| **Truth** | Is the move being absorbed? [CVD] | **Allow** if supported by tape | **Urgent Exit** on Imbalance Flip |

### 4. Continuous Calibration
Finally, the engine uses **`KellySizer`** and **`PredictionCalibrator`** to dynamically adjust the weight of these strategies based on their recent real-world performance. If "Pumps" are failing but "Organic Trends" are hitting targets, the engine will automatically shift its capital allocation toward the more consistent "Perspective".

This approach transforms the engine from a "gatekeeper" into a **continuous observer**, where every tick is a potential decision to enter, stay, or leave based on whether the market's "physical" state still supports the money-making thesis.