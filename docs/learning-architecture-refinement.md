# SYMM learning architecture refinement

This report describes the current implementation, its economic contracts, and the verification evidence. It makes no claim that historical or simulated profitability grants current trading authority.

## Ownership and decision lifecycle

`Agent` has only construction, `Step`, and `Error`. Its step serializes composed owners and forwards the original envelope. It no longer owns scope lookup, lane transitions, wallet settlement, execution admission, candidate competition, account rewards, snapshot construction, or retrospective policy review.

| Owner | Responsibility |
| --- | --- |
| `LocalLearning` | Advance independent spot experiments against the guarded resident Level3 book and deliver their journal facts after releasing that book. |
| `learningLane` | Execute its own wallet, retain prospective decisions, settle delayed targets, and recycle only an exhausted independent experiment. |
| `Knowledge` | Recall alternative global/symbol evidence, bind both to one experience, attribute original Regions, and reconstruct retained local experience. |
| `CandidateBook` / `EntryCandidate` | Own current prospective claims, immutable inputs, revocation, and separately appended outcomes. |
| `CapitalLearner` | Compare WAIT and currently viable claims using the existing numerical selection mechanism. |
| `VirtualPortfolio` | Maintain one finite exact-rational shared cash balance, displayed-depth holdings, and delayed fills. |
| `AccountTeacher` | Assign causal, funding-adjusted, time-normalized account outcomes only to capital decisions. |
| `CapitalHistory` | Reconstruct complete portfolio experiences without restoring a portfolio, pending action, or authority. |
| `Execution` | Separate increase authority from liquidation, propose local buy claims, and select reductions from actual inventory. |
| `learningDesk`, `accountFunds`, `executionFeedback` | Queue venue work; atomically reserve current funds; keep pre-venue refusals distinct from actual execution feedback. |
| `LotIncrease` | Serialize increases, cumulative fills, and cancellation-before-exit on the existing position guardian. |
| `LearningInspector` / `PolicyReview` | Produce coherent operator snapshots / compare delayed spot episodes with observed policy exposure. Neither retrospective review nor the UI teaches live action selection. |

The former `agent_transition.go`, `agent_execution.go`, and `agent_view.go` are removed. These responsibilities belong to the owners above; they are not additional implementation files full of Agent methods.

## Required implementation report

1. **Conceptual owners.** The table above gives each substantial owner one responsibility. The numerical Model, Grid, Regions, Skill, Realization, existing broker lifecycle, and existing ordered SQLite writer remain canonical.

2. **What Agent lost.** Per-market initialization/transition, lane issue/settle/recycle, hierarchical recall, account dispatch and gating, exposure reconciliation, review state, and snapshot construction moved to composed owners. Agent retains the serialized orchestration boundary and its error.

3. **Continuous global training.** One local issue binds `{symbol,"virtual"}` and `{"","virtual"}` in the same Model ticket. Resolution advances the Model clock once, observes each scope's original ordered prefixes once, and releases that ticket once. Policy and exploratory lanes teach the same knowledge identities.

4. **Cross-symbol use.** Recall obtains a global and a symbol reading for the requested action/context. A symbol with absent or insufficient local evidence consumes the global reading, including that reading's matched depth, authority, samples, and pending count.

5. **Specialization.** Local evidence must be defined, must define variance whenever global evidence does, and must retain at least the global reading's `EvidenceAuthority * Maturity`. `Maturity = (Support-1)/Support`; Support is Kish effective sample size. Specificity wins ties. Outcome sign does not choose the scope. Both scopes age on the common resolution clock, so a dormant local exception can yield to fresh global experience.

6. **No double counting.** Recall selects one alternative reading. It never adds global and local sample counts, weights, predictions, or confidence. Each scope separately summarizes the same observation. `PriorReading.Pending` comes directly from the same interned prior that supplies the recalled evidence and depth; selection has no separate inflight traversal. Original issue paths still retain their original ordered training identity. Greedy permutation/subset recovery remains a read policy, with a dynamically sized used-token slice, including contexts longer than 32 tokens.

7. **Prospective candidates.** A non-exploring local positive buy action produces an immutable candidate after quantity/depth/fee validation. The record includes its local ticket, capture, clock times, named Regions, context, scope readings, authority, quantities/cost/reference, venue rules, horizon, Grid version, and observable account facts. Publication precedes allocation. Local virtual execution continues whether or not the actual account can fund the claim.

8. **Staleness.** Replacement, originating Grid-row updates, changed local policy, expiry of the frozen measured horizon, and changed effective increase authority revoke candidates. Upstream insufficient funds or blocked authorization retires the old claim. A queued worker checks current validity again. Final book/rule/fee changes that violate the original economics produce a specific refusal; the old claim is never silently resized or repriced into a different opportunity.

9. **Capital competition.** Each allocation compares WAIT with all currently viable, fundable claims through the existing Model selector. The exploration account uses the existing exploratory sampling semantics; the actual account uses its non-exploring authority-weighted evidence. Symbol sorting supplies reproducible tie order only. There is no top-K, arrival-order ranking, Sharpe score, correlation penalty, or fixed deployment quota.

10. **Learned versus physical behavior.** Preference among claims and WAIT is learned. Cash, paid fees, displayed depth, quantity increments/minimums, exact candidate budgets, and one outstanding virtual commitment are explicit economic/execution constraints. Local ENTER and SCALE UP both mean a buy claim. At actual allocation time, current account holdings choose whether the broker opens a lot or increases its existing lot; the original local action and its independent context remain unchanged in the candidate record. Virtual and actual inventory need not match.

11. **WAIT.** WAIT is an ordinary action with its own prospective capital ticket, account context, frozen baseline, measured horizon, and delayed outcome. A funded positive candidate does not force deployment. An account with no entries can still teach the return from retaining cash.

12. **Actual top-down reward.** Broker observations carry producer time/version, total spot value, quote cash, venue-available cash, inventory, completeness, and cumulative external funding. Kraken real spot valuation uses equivalent balance across currencies (`eb`), rather than collateral equity (`e`). Extended balance distinguishes owned cash from exchange holds and used credit; unused credit is not owned spot capital. Paper reads its authoritative CLI valuation and available wallet balance. The interpretation follows the [TradeBalance](https://docs.kraken.com/api-reference/account-data/get-trade-balance) and [BalanceEx](https://docs.kraken.com/api-reference/account-data/get-extended-balance) contracts.

13. **Elapsed time.** For a capital decision issued at `t0` and resolved at `t1`, define `growth = (equity1-equity0) - (funding1-funding0)`. Its target is `(growth / elapsedSeconds - issueBaselineRate) / initialAccountEquity`. The baseline is frozen at issue. Local action advantages use the same rate units; Skill separately observes the absolute interval return normalized by initial capital.

14. **Fast versus slow growth.** Equal gains over different observed durations yield different rates. Tests exercise a $2 gain on $200 over 0.5 seconds and 20 seconds. No fixed holding penalty, profit deadline, stop, or take-profit rule implements that distinction. Resolution occurs on the first available complete account observation after the measured window; actual feedback cannot have finer temporal resolution than the account producer.

15. **No selection contamination.** Actual and shared virtual account teachers reference only the capital Model. They cannot update local Knowledge, independent lane wallets, Skill, or Realization. Those models answer different causal questions. The capital Model shares virtual and actual evidence about finite-wallet allocation; it does not claim those experiences are independent market opportunities.

16. **Three separate concepts.** Skill measures disjoint absolute policy profitability and retains its existing promotion/demotion hysteresis. Capital reward measures allocation outcomes. Realization measures submission defects and observed fill slippage. Profits, losses, waiting, and upstream cash refusals do not change the Realization failure counter.

17. **Entry versus liquidation.** New/increased exposure requires current Skill and Realization authorization plus final feasible capital. Actual holdings supply reduction quantity and exposure context, including when the isolated policy is flat or demoted. Reductions/exits do not consult the increase gate. An exit during an outstanding increase cancels that buy, waits for its terminal cumulative fill fact, then sells the reconciled full lot. Ordinary stale/busy preconditions are execution refusals; actual venue errors remain failures.

18. **Cash admission.** The market workspace reads immutable account state and queues work. The venue worker refreshes authoritative account data, rechecks the candidate against the current guarded spot book, and atomically reserves the frozen fee-inclusive maximum cost. A terminal order retains its reservation until a balance request begun after that terminal fact returns. A proven pre-submission refusal releases it immediately. Exchange holds and local reservations may temporarily overlap; until terminal reconciliation establishes the overlap, availability conservatively charges both. This is visible in committed/available capital and is not a portfolio preference.

19. **Futures information boundary.** Derivatives Measurements still reach the spot-labelled Grid row. Only a registered spot symbol on a Level3 envelope enters local wallet initialization/execution. Futures ticker/trade envelopes cannot issue actions, create wallets, or publish capital candidates. Execution resolves the actual spot instrument and book again.

20. **Forward policy universe.** Raw Hindsight captures preserve their original spot/futures domain and original product identity. The policy reviewer filters the observation domain before episode discovery and accepts only the learner's actual spot markets. It never aliases a futures excursion into a spot profit coordinate.

21. **Prospective audit and later labels.** Candidate UUIDs link original local issues, candidate status facts, capital decisions/alternatives, later local and portfolio resolutions, and broker lifecycle correlation. Later labels are separate records. The candidate journal endpoint retrieves these records across local and account symbols. UI review labels the local continuing policy outcome as such, without calling it an isolated executable buy-and-hold counterfactual. Live selection has no retrospective review-result reader.

22. **Warmup.** SQLite joins complete issue/resolution pairs within the same run and mode before applying the retained-experience limit. Named quantity identities remap Regions into the current Grid registry. Legacy local events lacking quantity names train only unconditioned knowledge and report that limitation. Legacy interval targets are divided by their positive issue-to-resolution time; unknown target units are rejected. Old local experience trains both scopes. Portfolio history requires real prospective account/funding/identity fields and rate units. Knowledge is reconstructed; Skill, Realization, accounts, candidates, and pending actions remain cold.

23. **Constants and numerical integrity.** The existing 2048-resolution memory, eight measured impulse epochs per horizon, execution queue capacity, and bounded operator-history sizes remain existing operating choices. Warmup's experience count is derived from the geometric Kish limit `ceil((1+decay)/(1-decay))`, not a new raw-row cap. Dyadic exposure uses the existing bisection radix. API cursor overlap follows Unix-second protocol resolution. No new trading-score coefficient or risk threshold is introduced. Prior now retains effective support and normalized central moments directly: uniform aging leaves both invariant, avoiding squared-weight underflow after long dormant gaps. There is no non-finite sanitizer. Broker cumulative-fill arithmetic uses supplied decimal precision for costs and stable venue/SDK precision for displayed VWAPs.

24. **Explicit constraints and remaining limits.** This implementation adds no conventional portfolio strategy. It preserves the existing selector's empirical Gaussian approximation and its authority-weighted non-exploring preference; it is not a formal calibrated posterior. Non-quote external funding lacks historical conversion data and explicitly disables actual-account learning/admission rather than inventing a conversion. Full restoration of such funding periods requires authoritative historical valuations. Account marks are continuous at the producer's existing refresh cadence, not per market tick. Actual venue marks include already paid fees but are not a proof that all inventory can be liquidated through current displayed depth. Candidate local labels describe the continuing independent policy, not isolated causal profits. Forward Review still performs a run-wide delayed scan outside the workspace; its cost grows with retained tape and should be addressed as storage scales.

## Deterministic behavior coverage

| Requested behaviors | Direct coverage |
| --- | --- |
| 1–9: transfer, sparse/mature/stale specialization, no doubled recall, depth/backoff/recovery | `TestKnowledgeReading`, `TestKnowledgeWarmup`, `TestModelRecall`, `TestModelSelect`, `TestPriorReading` |
| 10–18: competition, WAIT, finite funds, staleness, repricing, independent local experiment | `TestCapitalLearnerAllocate`, `TestVirtualPortfolioAllocate`, `TestAccountFundsReserve`, `TestCandidateBookPublish`, `TestEntryCandidateReprice`, `TestAgentStep` |
| 19–26: marked reward, frozen input, elapsed time, separate local/realization state | `TestAccountTeacherObserve`, `TestAccountTeacherIssue`, `TestCapitalLearnerAllocate`, account/reward ledger tests |
| 27–34: derivatives inform spot, cannot trade, raw futures retained, no futures policy episode | `TestGridNodeStep`, `TestAgentStep`, `TestDecodeObservations`, `TestForwardReviewJudgesAgainstActualExposure` |
| 35–42: separate increase/liquidation gates, upstream refusal, rejection and slippage | `TestExecutionSubmit`, `TestExecutionReduce`, `TestExecutionRefresh`, `TestLotIncreasePlace`, `TestLotIncreaseApply`, `TestLotIncreaseCancel`, `TestRecordLifecycle`, `TestAccountFundsReserve` |
| 43–52: complete-pair warmup, cold authority, immutable candidates, separate labels, provenance | `TestKnowledgeWarmup`, `TestCapitalHistoryWarmup`, `TestAgentWarmup`, `TestSQLiteLearningExperiences`, `TestSQLiteLearningEvents`, `TestWriterWriteLearning`, existing Hindsight integrity suites |

The million-resolution aging regression failed with a NaN support value before the numerical representation change. Repeated partial fills, external exchange holds, changed authority, and local/actual inventory divergence have dedicated regression cases. Frontend tests render actual JSON field casing, separate gates, retained evidence, unavailable account inputs, unresolved targets, and exact-fraction display formatting.

## Verification evidence

The companion `learning-architecture-verification.md` records exact commands and their unmodified stdout, with stderr kept separately. Runtime observations and screenshots identify whether they used a live feed or a fixture. A startup/listen check, a live UI response, deterministic tests, race tests, and benchmarks are different forms of evidence and are reported separately.
