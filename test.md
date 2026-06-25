### The Unified Architecture: From Market Arrivals to Counterfactuals

If we map how these components interact, they form a structured cascade:

```
[Arrival Stream / Order Book] (Hawkes Point Processes)
         ↓  (Extract self-excitation, branching & cross-excitation)
[Physical Oscillators] (Mass, Phase, Velocity, Heat)
         ↓  (Project into 3D Space-Filling Torus)
[3D Pilot-Wave / Fluid Manifold] (Navier-Stokes + GPE in Metal)
         ↓  (Generate Coherence, Divergence, Pressure Gradients)
[Cognitive Manifold / Latent State] (Batched Predictive Coding)
         ↓  (Compress state & minimize free energy)
[Causal Structural Model] (Pearl's Ladder of Causation)
         ↓  (Abduct noise, Intervene via Do, compute counterfactuals)
[Actionable Policy / Staged Outputs]
```

#### 1. The Ingestion Layer: Point Processes
At the very beginning, you have raw events (such as high-frequency exchange feeds or transaction arrivals). A standard system treats these as a flat time-series. Instead, your [arrival.go](file:///hawkes/arrival.go) and [estimator.go](file:///hawkes/estimator.go) model them using a bivariate [BivariateEstimator](file:///hawkes/estimator.go).
* By calculating the branching matrices and cross-excitation parameters ($\alpha_{xy}, \alpha_{yx}$), you capture the microscopic "volatility" and "inertia" of the system before any smoothing occurs.

#### 2. The Field Layer: Toroidal Fluid Dynamics on the GPU
Rather than mapping these Hawkes parameters to a conventional feature vector, they are mapped to physical [ManifoldOscillator](file:///physics/manifold/bridge.h) structures—carrying mass, phase, velocity, and heat—and injected into a 3D toroidal grid [manifold.metal](file:///physics/manifold/manifold.metal).
* On the GPU, you run a dual-energy Eulerian gas simulation (compressible Navier-Stokes) coupled with a Gross-Pitaevskii (GPE) quantum fluid solver. 
* The particles are guided by a **probability current guidance equation** (Pilot-Wave mechanics):
  $$v(x) = \frac{\hbar}{m} \frac{\text{Im}(\psi^* \nabla \psi)}{|\psi|^2 + \epsilon}$$
* This translates noisy, high-frequency transactional data into smooth, macroscopic wave-coherence fields ($\psi_{re}, \psi_{im}$) and pressure gradients.

#### 3. The Cognitive Layer: Predictive Coding
Once the physical manifold settles, its state represents the raw "sensory" environment. To compress this and make it legible for decision-making, your [resonance.metal](file:///learning/manifold/resonance.metal) engine runs batched predictive coding on the GPU.
* By minimizing free-energy (reconstruction error + state/weight decay) across a deep hierarchy, the network extracts the latent modes of the physical field, outputting structured [Eigenmode](file:///geometry/eigenmode.go) vectors.

#### 4. The Reasoning Layer: Pearl’s Causal Ladder
With the compressed, low-dimensional latent states in hand, you finally climb Pearl's Causal Ladder [causal/ladder.go](file:///causal/ladder.go).
* In [abduction.go](file:///causal/abduction.go) and [do.go](file:///causal/do.go), you use these states to run structural causal models (SCMs).
* Because the system is modeled as a physical fluid, **abduction** is the extraction of the physical fluid's residual noise, and **intervention ($do(x)$)** represents shifting the boundary conditions of the fluid (e.g., adding localized mass or energy) to observe how the entire thermodynamic grid re-equilibrates.

---

### The Most Fascinating Cross-Section: "Physical" Counterfactuals

The most compelling aspect of this pipeline is how it redefines **Abductive Counterfactual Reasoning**. 

In classical causal inference, running a counterfactual query on tabular data is highly abstract and reliant on restrictive linear assumptions. In your system, the Structural Causal Model is replaced by a physical simulation:
1. **Abduction:** You observe a market state, reconstruct it in the predictive-coding manifold, and isolate the exact "noise" (residual error) required to make the physical 3D gas simulation match reality.
2. **Action ($do$):** You intervene by forcing a parameter (e.g., setting liquidity flow to a specific level).
3. **Prediction:** You let the GPU fluid run, and the pilot waves guide the particles to a new, physically consistent equilibrium. The resulting state is your counterfactual.
