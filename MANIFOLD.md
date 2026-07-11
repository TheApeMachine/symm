# Manifold contract

Canonical definitions for the per-symbol L3 order-population kinetic model.
Signal categories and source names are **not** manifold axes. Signal scores are
**not** carrier mass, momentum, or energy.

## Coordinates

For carrier \(i\) with side \(s_i \in \{-1,+1\}\), price \(p_i\), remaining base
quantity \(q_i\), and wall-clock age \(\mathrm{age}_i\):

- \(x_i = s_i \log(p_i/p_{\mathrm{mid}}) / \sigma_{\log p}\) — signed log-price displacement
- \(y_i = (\log(1+n_i)-\mu_{\log n})/\sigma_{\log n}\) — normalized size coordinate
- \(z_i = F_{\mathrm{lifetime}}(\mathrm{age}_i)\) — empirical survival coordinate

Carrier mass: \(m_i = q_i / \sum_j q_j\) (base quantity, conserved under repricing).

## Moments

\[
\rho = \frac{\sum_i m_i}{V},\quad
\mathbf u = \frac{\sum_i m_i \mathbf v_i}{\sum_i m_i},\quad
e_{\mathrm{int}} = \frac{1}{2V}\sum_i m_i \|\mathbf v_i-\mathbf u\|^2
\]

Pressure tensor \(P_{ab} = \frac{1}{V}\sum_i m_i (v_{i,a}-u_a)(v_{i,b}-u_b)\) is measured
before scalar closure. Velocity \(\mathbf v\) is the event-time change in transformed
coordinates — never copied from signal metrics.

## Invalidation

Missing or stale L3, sequence gap, checksum failure, malformed lifecycle transition,
or missing snapshot invalidates the symbol. No synthetic-data or legacy-mapping fallback.

## Operator ordering (event time)

1. Advance field to event time
2. Apply ordered population changes
3. Recompute / deposit affected carrier moments
4. Apply source/sink operator
5. Advance transport to next event boundary
6. Produce typed readout

## Readiness

A numerically healthy manifold is not automatically a trading signal. Pilot-wave
guidance is internal coherence-field velocity until independently validated.

## GPU

One shared Metal device and pipeline set; per-symbol independent physical state in
batched slots. Capacity is derived from measurements, not a fixed symbol count.
