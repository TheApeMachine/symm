import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { actionStore } from "#/collections/actions";
import { appStore } from "#/collections/app";
import { causalStore } from "#/collections/causal";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { cn } from "#/lib/utils";
import { fixed } from "./decision-format";
import { DecisionSideRail } from "./decision-side";

const finite = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const ratio = (value: unknown): number =>
	Math.min(1, Math.max(0, finite(value)));

export const DecisionsSurface = () => {
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const actionsBySymbol = useSelector(actionStore, (state) => state.actions);
	const causalBySymbol = useSelector(causalStore, (state) => state.causal);
	const manifoldBySymbol = useSelector(
		manifoldStore,
		(state) => state.manifold,
	);
	const resonanceBySymbol = useSelector(
		resonanceStore,
		(state) => state.resonance,
	);
	const measurementsBySymbol = useSelector(
		measurementsStore,
		(state) => state.measurements,
	);
	const instrumentSymbols = useSelector(
		instrumentsStore,
		(state) => state.symbols,
	);
	const allowedActions = Object.values(actionsBySymbol).flatMap((history) =>
		history.values(),
	);
	const allowedBySymbol = allowedActions.reduce(
		(out, action) => {
			out[action.symbol] = action;

			return out;
		},
		{} as Record<string, (typeof allowedActions)[number]>,
	);
	const symbolSet = new Set(
		[
			...Object.keys(causalBySymbol),
			...Object.keys(resonanceBySymbol),
			...Object.keys(manifoldBySymbol),
			...Object.keys(measurementsBySymbol),
			...Object.keys(actionsBySymbol),
		].filter((symbol) => symbol.includes("/")),
	);
	const symbols = [
		...instrumentSymbols.filter((symbol) => symbolSet.has(symbol)),
		...[...symbolSet]
			.filter((symbol) => !instrumentSymbols.includes(symbol))
			.sort(),
	];
	const candidates = symbols
		.map((symbol) => {
			const action = allowedBySymbol[symbol];
			const causal = causalBySymbol[symbol]?.values().at(-1);
			const resonance = resonanceBySymbol[symbol]?.values().at(-1);
			const manifold = manifoldBySymbol[symbol]?.values().at(-1);
			const causalStrength = finite(causal?.strength);
			const causalBaseline = finite(causal?.baseline);
			const resonanceConfidence = ratio(resonance?.confidence);
			const causalConfidence = ratio(causal?.confidence);
			const score = action?.score ?? Math.min(resonanceConfidence, causalConfidence);
			const support = [causal, resonance, manifold].filter(Boolean).length;
			const inPlay = support >= 2 && causalStrength >= causalBaseline;
			const verdict = action?.verdict ?? (inPlay ? "blocked" : "below");
			const why =
				action?.reason ??
				(causal === undefined
					? "waiting causal"
					: resonance === undefined
						? "waiting resonance"
						: manifold === undefined
							? "waiting manifold"
							: inPlay
								? "not admitted"
								: "below line");

			return {
				action,
				causal,
				manifold,
				resonance,
				symbol,
				support,
				score,
				inPlay,
				verdict,
				why,
				bars: [
					{ src: "causal", value: causalStrength },
					{ src: "predict", value: finite(resonance?.confidence) },
					{ src: "manifold", value: finite(manifold?.momentum) },
				].filter((bar) => bar.value !== 0),
				waterfall: [
					{
						src: "causal",
						delta: causalStrength - causalBaseline,
					},
					{
						src: "predict",
						delta: finite(resonance?.flow) - finite(resonance?.baseline),
					},
					{
						src: "field",
						delta: finite(manifold?.momentum),
					},
				],
				probes: [
					{ label: "beta", value: finite(causal?.beta) },
					{ label: "panic", value: finite(causal?.panic) },
					{ label: "residual", value: finite(causal?.residual) },
					{ label: "intervention", value: finite(causal?.intervention) },
				],
			};
		});
	const current =
		candidates.find((candidate) => candidate.symbol === selectedSymbol) ??
		candidates.find((candidate) => candidate.symbol === focusSymbol) ??
		candidates[0];
	const scanned = instrumentSymbols.length || symbols.length;
	const quoted = Object.keys(measurementsBySymbol).length;
	const inPlay = candidates.filter((candidate) => candidate.inPlay).length;
	const allowed = candidates.filter(
		(candidate) => candidate.verdict === "allow",
	).length;
	const entryLine = finite(current?.causal?.baseline ?? current?.action?.entryLine);
	const entryScore = finite(
		current?.causal?.strength ?? current?.action?.entryScore,
	);
	const entryConfidence = finite(
		current?.causal?.confidence ?? current?.action?.entryConfidence,
	);

	return (
		<div className="grid h-full min-h-0 min-w-[1040px] grid-cols-[minmax(640px,1fr)_332px]">
			<div className="min-h-0 overflow-auto px-5 py-[18px]">
				<div className="mb-[18px] grid grid-cols-4 gap-2.5">
					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							Scanned
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--acc)">
							{scanned}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							universe
						</div>
					</div>

					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							Quoted
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--info)">
							{quoted}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							fresh ticks
						</div>
					</div>

					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							In Play
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--acc)">
							{inPlay}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							≥ entry line
						</div>
					</div>

					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							Allowed
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--up)">
							{allowed}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							edge clears
						</div>
					</div>
				</div>

				{current ? (
					<div className="mb-3.5 flex items-center gap-3.5 rounded border border-(--line) bg-(--sunken) px-3 py-2 font-mono text-[11.5px]">
						<span className="text-(--f3)">entry line</span>
						<span className="font-semibold text-(--acc)">
							{fixed(entryLine)}
						</span>
						<span className="text-(--f4)">·</span>
						<span className="text-(--f3)">strength {fixed(entryScore)}</span>
						<span className="text-(--f4)">·</span>
						<span className="text-(--f3)">
							confidence {fixed(entryConfidence)}
						</span>
						<span className="ml-auto text-(--f4)">
							support gate ≥ 2 · backend verdict wins
						</span>
					</div>
				) : null}

        <div className="mb-2 flex items-baseline justify-between">
          <span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
            Candidate evaluation
          </span>
          <span className="font-mono text-[9.5px] text-(--f4)">
            click a row to inspect attribution + counterfactuals
          </span>
        </div>

        <div className="flex flex-col gap-[7px]">
          {candidates.length === 0 ? (
            <div className="rounded border border-(--line) bg-(--surface) px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
              waiting for backend decision frames
            </div>
          ) : null}

          {candidates.map((candidate) => {
            const selected = candidate.symbol === current?.symbol;
						const scorePercent = Math.round(ratio(candidate.score) * 100);
						const verdictTone =
							candidate.verdict === "allow"
								? "var(--up)"
								: candidate.verdict === "blocked"
									? "var(--down)"
									: "var(--f3)";
						const verdictBackground =
							candidate.verdict === "allow"
								? "color-mix(in srgb,var(--up) 16%,transparent)"
								: candidate.verdict === "blocked"
									? "color-mix(in srgb,var(--down) 14%,transparent)"
									: "var(--line)";

            return (
              <button
                type="button"
                key={candidate.symbol}
                data-symbol={candidate.symbol}
                onClick={() => setSelectedSymbol(selected ? null : candidate.symbol)}
                className={cn(
                  "cursor-pointer overflow-hidden rounded border bg-(--surface) text-left font-[inherit]",
                  selected
										? "border-[color-mix(in_srgb,var(--up)_30%,transparent)]"
										: "border-(--line)",
                )}
              >
                <div className="grid grid-cols-[78px_1fr_132px_92px] items-center gap-3 px-3 py-2.5">
                <div>
                  <div className="font-mono font-semibold text-[13px] text-(--f1)">
                    {candidate.symbol}
                  </div>
                  <div className="font-mono text-[9px] text-(--f4)">
										×{candidate.support} src
									</div>
                </div>

                <div className="flex min-w-0 flex-col gap-1 font-mono text-[9px]">
									{candidate.bars.length === 0 ? (
										<div className="text-(--f4)">waiting for ladder frames</div>
									) : null}
									{candidate.bars.map((bar) => (
										<div key={bar.src} className="flex items-center gap-[7px]">
											<span className="w-16 text-(--f4)">{bar.src}</span>
											<div className="h-1 flex-1 overflow-hidden rounded-sm bg-(--line)">
												<div
													className="h-full"
													style={{
														width: `${Math.round(ratio(bar.value) * 100)}%`,
														background:
															ratio(bar.value) > 0.6
																? "var(--acc)"
																: "var(--info)",
													}}
												/>
											</div>
											<span className="w-[30px] text-right text-(--f3)">
												{fixed(bar.value)}
											</span>
										</div>
									))}
                </div>

                <div>
                  <div className="mb-0.5 flex items-center justify-between font-mono text-[9.5px] text-(--f4)">
                    <span>combined</span>
                    <span className="text-(--f1)">
                      {fixed(candidate.score)}
                    </span>
                  </div>
									<div className="h-1.5 overflow-hidden rounded-sm bg-(--line)">
										<div
											className="h-full"
											style={{
												width: `${scorePercent}%`,
												background:
													candidate.verdict === "allow"
														? "var(--up)"
														: candidate.inPlay
															? "var(--down)"
															: "var(--info)",
											}}
										/>
									</div>
									<div
										className="mt-1 font-mono text-[9px]"
										style={{
											color:
												entryScore >= entryLine ? "var(--up)" : "var(--down)",
										}}
									>
										edge {fixed(finite(candidate.causal?.strength) - finite(candidate.causal?.baseline))}
									</div>
                </div>

                <div className="text-right">
                  <span
                    className="inline-block rounded-sm px-2.5 py-1 font-semibold text-[10px] uppercase"
										style={{
											background: verdictBackground,
											color: verdictTone,
										}}
                  >
                    {String(candidate.verdict).toUpperCase()}
                  </span>
									<div className="mt-1 font-mono text-[9px] text-(--f4)">
										{candidate.why}
									</div>
                </div>
              </div>

                {selected ? (
                  <div className="grid grid-cols-2 gap-5 border-(--line) border-t bg-(--sunken) px-3.5 py-3 font-mono text-[9.5px]">
                    <div>
                      <div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.1em]">
												Score attribution
											</div>
											<div className="flex flex-col gap-1.5">
												{candidate.waterfall.map((row) => {
													const width = Math.min(46, Math.abs(row.delta) * 100);
													const positive = row.delta >= 0;

													return (
														<div key={row.src} className="flex items-center gap-2">
															<span className="w-[60px] text-(--f4)">
																{row.src}
															</span>
															<div className="relative h-3 flex-1 rounded-sm bg-(--line)">
																<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--f4)" />
																<div
																	className="absolute top-px bottom-px rounded-[1px]"
																	style={{
																		left: `${positive ? 50 : 50 - width}%`,
																		width: `${width}%`,
																		background: positive
																			? "var(--up)"
																			: "var(--down)",
																	}}
																/>
															</div>
															<span
																className="w-[50px] text-right"
																style={{
																	color: positive ? "var(--up)" : "var(--down)",
																}}
															>
																{positive ? "+" : "−"}
																{Math.abs(row.delta).toFixed(3)}
															</span>
														</div>
													);
												})}
											</div>
											<div className="mt-2 text-[9px] text-(--f4)">
												branch ·{" "}
												{candidate.action?.branchKey ??
													[
														candidate.manifold?.category,
														candidate.resonance?.category,
														candidate.causal?.category,
													]
														.filter(Boolean)
														.join(" / ")}
											</div>
                    </div>
                    <div>
                      <div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.1em]">
												Counterfactual probes · do(·)
											</div>
											<div className="flex flex-col gap-1.5">
												{candidate.probes.map((probe) => (
													<div
														key={probe.label}
														className="flex items-center justify-between gap-2 rounded-sm border border-(--line) bg-(--surface) px-2 py-1.5"
													>
														<span className="text-(--f2)">{probe.label}</span>
														<span className="text-(--f1)">
															{fixed(probe.value)}
														</span>
													</div>
												))}
											</div>
                    </div>
                  </div>
                ) : null}
              </button>
            );
          })}
        </div>
      </div>

      <div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
        <DecisionSideRail symbol={current?.symbol} />
      </div>
    </div>
  );
};
