import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { appStore } from "#/collections/app";
import { causalStore } from "#/collections/causal";
import {
	decisionStore,
	latestStrategyDecisions,
} from "#/collections/decisions";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import {
	verdictBadgeClassName,
	verdictToVariant,
} from "#/components/terminal/badge-tone";
import {
	causalAssociation,
	causalCategory,
	causalConfidence,
	causalContagion,
	causalEntryBaseline,
	causalIntervention,
	causalNoise,
	causalRatio,
	causalStrength,
} from "#/components/terminal/causal-view";
import { cn } from "#/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { causalCleared, judgeCandidate, pearlEdge } from "./decision-candidate";
import { fixed } from "./decision-format";
import { DecisionSideRail, LiveDecisionsEntryLine } from "./decision-side";
import { StrategyDecisionRows } from "./strategy-decisions";

const finite = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const ratio = (value: unknown): number =>
	Math.min(1, Math.max(0, finite(value)));

export const DecisionsSurface = () => {
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const strategyDecisions = useSelector(decisionStore, (state) =>
		latestStrategyDecisions(state.decisions),
	);
	const decisionsBySymbol = strategyDecisions.reduce(
		(out, decision) => {
			out[decision.symbol] = decision;

			return out;
		},
		{} as Record<string, (typeof strategyDecisions)[number]>,
	);
	useSelector(causalStore, (state) => state.version);
	useSelector(manifoldStore, (state) => state.version);
	useSelector(resonanceStore, (state) => state.version);
	const causalBySymbol = causalStore.state.causal;
	const manifoldBySymbol = manifoldStore.state.manifold;
	const resonanceBySymbol = resonanceStore.state.resonance;
	const measurementsBySymbol = useSelector(
		measurementsStore,
		(state) => state.measurements,
	);
	const instrumentSymbols = useSelector(
		instrumentsStore,
		(state) => state.symbols,
	);
	const symbolSet = new Set(
		[
			...Object.keys(causalBySymbol),
			...Object.keys(resonanceBySymbol),
			...Object.keys(manifoldBySymbol),
			...Object.keys(decisionsBySymbol),
		].filter((symbol) => symbol.includes("/")),
	);
	const symbols = [
		...instrumentSymbols.filter((symbol) => symbolSet.has(symbol)),
		...[...symbolSet]
			.filter((symbol) => !instrumentSymbols.includes(symbol))
			.sort(),
	];
	const candidates = symbols.map((symbol) => {
		const decision = decisionsBySymbol[symbol];
		const causal = causalBySymbol[symbol]?.values().at(-1);
		const resonance = resonanceBySymbol[symbol]?.values().at(-1);
		const manifold = manifoldBySymbol[symbol]?.values().at(-1);
		const causalStrengthValue = causalStrength(causal);
		const causalBaselineValue = causalEntryBaseline(causal);
		const resonanceConfidence = ratio(resonance?.confidence);
		const causalConfidenceValue = causalRatio(causalConfidence(causal));
		const score =
			decision?.utility ?? Math.min(resonanceConfidence, causalConfidenceValue);
		const support = [causal, resonance, manifold].filter(Boolean).length;
		const cleared = causalCleared(causal);
		const judgement = judgeCandidate(decision, support, cleared, {
			causal: causal === undefined,
			resonance: resonance === undefined,
			manifold: manifold === undefined,
		});

		return {
			decision,
			causal,
			manifold,
			resonance,
			symbol,
			support,
			score,
			inPlay: judgement.inPlay,
			verdict: judgement.verdict,
			why: judgement.why,
			bars: [
				{
					src: "causal",
					value: causalStrengthValue,
					present: causal !== undefined,
				},
				{
					src: "predict",
					value: finite(resonance?.confidence),
					present: resonance !== undefined,
				},
				{
					src: "manifold",
					value: finite(manifold?.momentum),
					present: manifold !== undefined,
				},
			].filter((bar) => bar.present),
			waterfall: [
				{
					src: "causal",
					delta: causalStrengthValue - causalBaselineValue,
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
				{ label: "beta", value: causalAssociation(causal) },
				{ label: "panic", value: causalContagion(causal) },
				{ label: "residual", value: causalNoise(causal) },
				{ label: "intervention", value: causalIntervention(causal) },
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

				<LiveDecisionsEntryLine symbol={current?.symbol} />

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
						<Panel
							variant="surface"
							size="bare"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for backend decision frames
						</Panel>
					) : null}

					{candidates.map((candidate) => {
						const selected = candidate.symbol === current?.symbol;
						const scorePercent = Math.round(ratio(candidate.score) * 100);
						const edge = pearlEdge(candidate.causal);

						return (
							<button
								type="button"
								key={candidate.symbol}
								data-symbol={candidate.symbol}
								onClick={() =>
									setSelectedSymbol(selected ? null : candidate.symbol)
								}
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
											<div className="text-(--f4)">
												waiting for ladder frames
											</div>
										) : null}
										{candidate.bars.map((bar) => (
											<Meter
												key={bar.src}
												layout="inline"
												label={bar.src}
												value={fixed(bar.value)}
												percent={Math.round(ratio(bar.value) * 100)}
												variant={
													bar.value === 0
														? "disabled"
														: ratio(bar.value) > 0.6
															? "warning"
															: "info"
												}
												size="s"
												labelClassName="w-16 text-(--f4)"
												valueClassName="w-[30px] text-(--f3)"
											/>
										))}
									</div>

									<div>
										<Meter
											layout="stacked"
											label={
												candidate.decision !== undefined
													? "utility"
													: "combined"
											}
											value={fixed(candidate.score)}
											percent={scorePercent}
											variant={
												candidate.verdict === "allow"
													? "success"
													: candidate.verdict === "blocked"
														? "error"
														: "info"
											}
											size="m"
											labelClassName="text-[9.5px] text-(--f4)"
										/>
										<div
											className="mt-1 font-mono text-[9px]"
											style={{
												color: edge >= 0 ? "var(--up)" : "var(--down)",
											}}
										>
											pearl Δ {fixed(edge)}
										</div>
									</div>

									<div className="text-right">
										<Badge
											label={String(candidate.verdict)}
											variant={verdictToVariant(String(candidate.verdict))}
											className={verdictBadgeClassName(
												String(candidate.verdict),
											)}
										/>
										<div className="mt-1 font-mono text-[9px] text-(--f4)">
											{candidate.why}
										</div>
									</div>
								</div>

								{selected ? (
									<div className="grid grid-cols-2 gap-5 border-(--line) border-t bg-(--sunken) px-3.5 py-3 font-mono text-[9.5px]">
										<div>
											<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-widest">
												Score attribution
											</div>
											<div className="flex flex-col gap-1.5">
												{candidate.waterfall.map((row) => {
													const width = Math.min(46, Math.abs(row.delta) * 100);
													const positive = row.delta >= 0;

													return (
														<div
															key={row.src}
															className="flex items-center gap-2"
														>
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
												{candidate.decision?.cause ??
													[
														candidate.manifold?.category,
														candidate.resonance?.category,
														candidate.causal === undefined
															? ""
															: String(causalCategory(candidate.causal)),
													]
														.filter(Boolean)
														.join(" / ")}
											</div>
										</div>
										<div>
											<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-widest">
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
				<StrategyDecisionRows symbol={current?.symbol} />
			</div>
		</div>
	);
};
