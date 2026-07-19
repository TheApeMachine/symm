import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { appStore } from "#/collections/app";
import type {
	CausalFrame,
	Instrument,
	ManifoldFrame,
	Measurement,
	ResonanceFrame,
} from "#/collections/types";
import type { StrategyDecision } from "#/types/thesis";
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
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";
import { Badge } from "@/components/ui/badge";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import {
	causalCleared,
	judgeCandidate,
	manifoldField,
	pearlEdge,
	resonanceEdge,
	resonancePredict,
} from "./decision-candidate";
import { fixed } from "./decision-format";
import { DecisionSideRail, LiveDecisionsEntryLine } from "./decision-side";
import { StrategyDecisionRows } from "./strategy-decisions";

const latestBySymbol = <T extends { symbol: string }>(
	rows: T[],
): Record<string, T> => {
	const map: Record<string, T> = {};

	for (const row of rows) {
		map[row.symbol] = row;
	}

	return map;
};

const finite = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const ratio = (value: unknown): number =>
	Math.min(1, Math.max(0, finite(value)));

const present = (value: number | null): number => (value === null ? 0 : value);

export const DecisionsSurface = () => {
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const [strategyDecisions, setStrategyDecisions] = useState<
		StrategyDecision[]
	>([]);
	const [causalBySymbol, setCausalBySymbol] = useState<
		Record<string, CausalFrame>
	>({});
	const [manifoldBySymbol, setManifoldBySymbol] = useState<
		Record<string, ManifoldFrame>
	>({});
	const [resonanceBySymbol, setResonanceBySymbol] = useState<
		Record<string, ResonanceFrame>
	>({});
	const [measured, setMeasured] = useState(0);
	const [instrumentSymbols, setInstrumentSymbols] = useState<string[]>([]);

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "decisions", key: "" },
			{ store: "causal", key: "" },
			{ store: "manifold", key: "" },
			{ store: "resonance", key: "" },
			{ store: "measurements", key: "" },
			{ store: "instruments", key: "" },
		],
		(buffers) => {
			setStrategyDecisions(
				(buffers["decisions:"] ?? []) as StrategyDecision[],
			);
			setCausalBySymbol(
				latestBySymbol((buffers["causal:"] ?? []) as CausalFrame[]),
			);
			setManifoldBySymbol(
				latestBySymbol((buffers["manifold:"] ?? []) as ManifoldFrame[]),
			);
			setResonanceBySymbol(
				latestBySymbol((buffers["resonance:"] ?? []) as ResonanceFrame[]),
			);
			setMeasured(
				new Set(
					((buffers["measurements:"] ?? []) as Measurement[]).map(
						(measurement) => measurement.symbol,
					),
				).size,
			);
			setInstrumentSymbols(
				((buffers["instruments:"] ?? []) as Instrument[])
					.map((instrument) => instrument.symbol)
					.sort(),
			);
		},
		[online],
	);

	const decisionsBySymbol = latestBySymbol(strategyDecisions);
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
		const causal = causalBySymbol[symbol];
		const resonance = resonanceBySymbol[symbol];
		const manifold = manifoldBySymbol[symbol];
		const causalStrengthValue = causalStrength(causal);
		const causalBaselineValue = causalEntryBaseline(causal);
		const predict = resonancePredict(resonance);
		const field = manifoldField(manifold);
		const predictEdge = resonanceEdge(resonance);
		const causalConfidenceValue = causalRatio(causalConfidence(causal));
		const score =
			decision?.utility ??
			Math.min(ratio(present(predict)), causalConfidenceValue);
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
					value: present(predict),
					present: resonance !== undefined,
				},
				{
					src: "manifold",
					value: present(field),
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
					delta: present(predictEdge),
				},
				{
					src: "field",
					delta: present(field),
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
	// Field = symbols with causal/resonance/manifold (the list below).
	// Measured = symbols that emitted any signal frame. Do not conflate them.
	const field = candidates.length;
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
							Field
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--acc)">
							{field}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							evaluated
						</div>
					</div>

					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							Measured
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--info)">
							{measured}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							signal symbols
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
