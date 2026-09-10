# Grid formation and activation

This contract implements the separation in `diagram.pdf` between forming
sympathetic communities and measuring their activation. The user's clarified
lifecycle is: balance the grid, form its regions once, then use those regions.

## Calibration

The existing retained feature window supplies the standardized directional,
sign-consistency and magnitude channels. Once that window is populated, its
pairwise affinities and accumulated maturity/SNR weights become a fixed
calibration problem. An unobserved pair has no edge. A grid without an evidenced
pair cannot claim convergence.

The existing window span and affinity classifier are unchanged by this fix.
They are model inputs, not newly derived confidence guarantees. In particular,
this change does not turn the existing 64-bin default into a statistically
optimal observation horizon.

## Balance

For calibrated pair strength `s_ij`, node evidence `w_i`, target separation
`d_ij` and coordinate `x_i`, the layout minimizes weighted distance stress:

```
E(X) = sum(i < j) w_i w_j s_ij (distance(x_i, x_j) - d_ij)^2
```

The majorization update moves one coordinate per observed quantity. An envelope
containing multiple quantities supplies that many updates, so formation work
is not arbitrarily slowed by the transport bundling many measurements together.
Its own evidence supplies resistance, so weakly evidenced nodes move more than
strongly evidenced nodes. The target graph stays fixed during this solve.
Changing targets during every optimizer step would not establish convergence
of this objective.

Each proposed coordinate update must strictly reduce its incident distance
stress as represented by floating-point arithmetic. A complete sweep without
an improving update ends formation. This rejects numerical coordinate jitter
that does not improve the fitted relationships. There is no chosen elapsed
time, iteration count, percentage improvement or market significance threshold.
Production has no forced-success iteration limit. This is numerical stationarity
of the calibrated layout, not statistical certainty about future market behavior.

## Regions

Let `a_ij` be `w_i w_j s_ij`, positive for a stable direct or inverse
relationship and negative for an inconsistent relationship. A partition `c`
has signed agreement:

```
Q(c) = sum(i < j) a_ij * indicator(c_i == c_j)
```

Starting with separate nodes, move a node only when its incident agreement
strictly improves. Empty communities remain candidates so repulsion can split
a group. Stop when a full pass has no improving move. This is a local optimum
of signed agreement; it is not a claim to solve global clustering optimally.

Membership is then fixed. Each region uses its lowest quantity index as its
stable identity. Inverse members are aligned to that anchor when aggregating
level and change. Current movement energy, maturity and SNR determine current
activation and authority. Otsu selects the active part of this fixed partition;
it does not delete the quieter communities.

## Lifetime

`Impulse.Ready` means formation is complete. Quiet or missing observations can
produce an empty active sequence while readiness remains true. No active
sequence still means no action; it never means that region formation restarted.

Historical fragment changes reset observation-local values and baselines while
retaining the worker's calibration, layout and communities. Workers continue
forming across short fragments instead of restarting on each one.

A previously unknown quantity changes the grid schema and invalidates the
calibration explicitly. Ordinary values, uncertainty changes and absent
producers do not. Process restarts retain the existing checkpoint behavior:
quantity identities and cognition restore, while a new grid instance forms.

This fix does not implement the diagram's optional phase channel or introduce
a top-k policy. It retains the existing all-observed-pairs affinity calculation.
