import * as d3 from "d3";
import { ChevronRight, Pause, Play } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
	focusAtom,
	positionCountAtom,
	type RingBuffer,
	signals,
} from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import { hubBaseUrl } from "#/lib/hub";
import { cn } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { action, basis, clock } from "./format";
import type {
	CognitionTreeResponse,
	LearningActivityEntry,
	TrieBranch,
} from "./types";

interface ForwardTapePoint {
	x: number;
	y: number;
	seq?: number;
}

const getTrainingRing = (
	records: Record<string, RingBuffer<MeasurementT>> | undefined,
	focus: string,
): RingBuffer<MeasurementT> | null => {
	if (!records) return null;
	return (
		records[focus] ??
		records.learner ??
		records[""] ??
		Object.values(records)[0] ??
		null
	);
};

export const ForwardLearningViz = () => {
	const tapeRef = useRef<HTMLDivElement>(null);
	const [tapeDim, setTapeDim] = useState({ width: 800, height: 300 });
	const [isPlaying, setIsPlaying] = useState(true);

	// Real tape points accumulated from the live training measurements
	const [points, setPoints] = useState<ForwardTapePoint[]>([]);

	// Real stage & execution mode
	const [stageCode, setStageCode] = useState(0);
	const [stageName, setStageName] = useState("MODEL DEVELOPMENT");
	const [stageBlocker, setStageBlocker] = useState("");
	const [openPositionsCount, setOpenPositionsCount] = useState(0);
	const [isPaperFilled, setIsPaperFilled] = useState(false);

	// Precursor & prediction
	const [precursorLength, setPrecursorLength] = useState(0);
	const [rawPrecursorTokens, setRawPrecursorTokens] = useState<string[]>([]);
	const [frozenAction, setFrozenAction] = useState("WAIT");
	const [delayedLabel, setDelayedLabel] = useState("RESOLVING");

	// Historical held-out metrics
	const [histOpportunities, setHistOpportunities] = useState(0);
	const [histCorrectEnter, setHistCorrectEnter] = useState(0);
	const [histMissedEnter, setHistMissedEnter] = useState(0);
	const [histFalseEnter, setHistFalseEnter] = useState(0);
	const [histMeanReturn, setHistMeanReturn] = useState(0);
	const [histLowerBound, setHistLowerBound] = useState(0);

	// Forward paper metrics
	const [fwdPredictions, setFwdPredictions] = useState(0);
	const [fwdPaperTrades, setFwdPaperTrades] = useState(0);
	const [fwdPaperMeanReturn, setFwdPaperMeanReturn] = useState(0);
	const [fwdPaperLowerBound, setFwdPaperLowerBound] = useState(0);

	// Overall counters
	const [evaluatedCount, setEvaluatedCount] = useState(0);

	// Real Radix Trie branches fetched from backend engine
	const [trieBranches, setTrieBranches] = useState<TrieBranch[]>([]);

	// Real activity entries
	const [activityLogs, setActivityLogs] = useState<LearningActivityEntry[]>([]);

	// Real excursion markers if active
	const [excursionEvent, setExcursionEvent] = useState<{
		type: "UPWARD EXCURSION" | "DOWNWARD EXCURSION" | "CHOPPY MARKET" | "FLAT TAPE";
		magnitude: number;
		marks: { A: number; B: number; C: number };
		entryIdx: number | null;
		exitIdx: number | null;
	} | null>(null);

	// Focus symbol
	const [currentSymbol, setCurrentSymbol] = useState(focusAtom.get() || "");

	useEffect(() => {
		const unsubFocus = focusAtom.subscribe((state) => {
			if (state) {
				setCurrentSymbol(state);
				setPoints([]);
				setExcursionEvent(null);
			}
		});
		const unsubPos = positionCountAtom.subscribe((count) => {
			setOpenPositionsCount(count);
		});
		setOpenPositionsCount(positionCountAtom.get());

		return () => {
			unsubFocus?.unsubscribe?.();
			unsubPos?.unsubscribe?.();
		};
	}, []);

	// Resize observers
	useEffect(() => {
		const tapeTarget = tapeRef.current;
		if (!tapeTarget || typeof ResizeObserver === "undefined") return;

		const ro = new ResizeObserver((entries) => {
			if (entries[0]) {
				const { width, height } = entries[0].contentRect;
				if (width > 0 && height > 0) {
					setTapeDim({ width, height });
				}
			}
		});
		ro.observe(tapeTarget);
		return () => ro.disconnect();
	}, []);

	// Fetch real Radix Trie tree & branch data from backend
	useEffect(() => {
		let isMounted = true;

		const fetchTree = async () => {
			try {
				const res = await fetch(`${hubBaseUrl()}/cognition/tree`);
				if (!res.ok) return;
				const data: CognitionTreeResponse = await res.json();
				if (!isMounted) return;

				if (data.branches && data.branches.length > 0) {
					setTrieBranches(data.branches);
				}
			} catch {
				// Backend endpoint connecting
			}
		};

		fetchTree();
		const interval = window.setInterval(fetchTree, 4000);
		return () => {
			isMounted = false;
			clearInterval(interval);
		};
	}, []);

	// Subscribe to real live training measurements stream
	useEffect(() => {
		const cursor = new RingCursor<MeasurementT>();
		let nextLogId = 1;

		const handleRing = (ring: RingBuffer<MeasurementT>) => {
			if (!ring || ring.isEmpty()) return;

			cursor.read(ring, (measurement) => {
				const metricMap: Record<string, number> = {};
				for (const m of measurement.metrics ?? []) {
					if (!m?.name) continue;
					metricMap[String(m.name)] = m.raw ?? 0;
				}

				const evalCount = metricMap.evaluated ?? 0;
				setEvaluatedCount(Math.floor(evalCount));

				// Update stage
				const sCode = metricMap.stage_code ?? 0;
				setStageCode(sCode);
				let sName = "MODEL DEVELOPMENT";
				if (sCode === 1) sName = "HISTORICAL VALIDATION";
				if (sCode === 2) sName = "FORWARD PAPER LEARNING";
				if (sCode === 3) sName = "FORWARD SKILL DEMONSTRATED";

				if (measurement.provenance) {
					for (const p of measurement.provenance) {
						if (p?.name === "stage" && p.value) {
							sName = String(p.value);
						}
						if (p?.name === "stage_blocker" && p.value !== undefined) {
							setStageBlocker(String(p.value));
						}
					}
				}
				setStageName(sName);

				// Precursor & prediction
				const pLen = Math.floor(metricMap.precursor_length ?? 0);
				setPrecursorLength(pLen);

				let tokensList: string[] = [];
				if (measurement.provenance) {
					for (const p of measurement.provenance) {
						if (p?.name === "precursor_tokens" && p.value) {
							tokensList = String(p.value).split(",").filter(Boolean);
						}
					}
				}
				if (tokensList.length === 0 && measurement.metadata) {
					for (const m of measurement.metadata) {
						if (m?.name === "precursor_tokens" && m.value) {
							tokensList = String(m.value).split(",").filter(Boolean);
						}
					}
				}
				setRawPrecursorTokens(tokensList);

				const filledRaw = metricMap.paper_filled ?? 0;
				setIsPaperFilled(filledRaw === 1);

				const actRaw = metricMap.action ?? 0;
				const frozenRaw = metricMap.frozen_prediction !== undefined ? metricMap.frozen_prediction : actRaw;
				setFrozenAction(frozenRaw === 1 ? "ENTER" : frozenRaw === 2 ? "EXIT" : "WAIT");

				const extRaw = metricMap.excursion_type ?? 0;
				if (extRaw === 1) setDelayedLabel("UP");
				else if (extRaw === 2) setDelayedLabel("DOWN");
				else if (extRaw === 3) setDelayedLabel("CHOP");
				else if (extRaw === 4) setDelayedLabel("FLAT");
				else if (metricMap.delayed_target !== undefined) setDelayedLabel(metricMap.delayed_target === 1 ? "ENTER (CLEARS)" : "WAIT");
				else setDelayedLabel("RESOLVING");

				// Historical held-out
				setHistOpportunities(Math.floor(metricMap.hist_opportunities ?? 0));
				setHistCorrectEnter(Math.floor(metricMap.hist_correct_enter ?? 0));
				setHistMissedEnter(Math.floor(metricMap.hist_missed_enter ?? 0));
				setHistFalseEnter(Math.floor(metricMap.hist_false_enter ?? 0));
				setHistMeanReturn(metricMap.hist_mean_return ?? 0);
				setHistLowerBound(metricMap.hist_lower_bound ?? 0);

				// Forward paper
				setFwdPredictions(Math.floor(metricMap.fwd_enter_predictions ?? 0));
				setFwdPaperTrades(Math.floor(metricMap.fwd_paper_trades ?? 0));
				setFwdPaperMeanReturn(metricMap.fwd_paper_mean_return ?? 0);
				setFwdPaperLowerBound(metricMap.fwd_paper_lower_bound ?? 0);

				// Accumulate real point directly from backend measurement
				const rawPrice =
					metricMap.price && metricMap.price > 0 ? metricMap.price : undefined;

				if (rawPrice !== undefined) {
					const seqVal = Number(measurement.tick ?? 0n);
					setPoints((prev) => {
						const next = [
							...prev,
							{ x: prev.length, y: rawPrice, seq: seqVal },
						];
						if (next.length > 200) {
							return next.slice(next.length - 200).map((pt, i) => ({
								x: i,
								y: pt.y,
								seq: pt.seq,
							}));
						}
						return next;
					});
				}

				// Record real activity log entry
				const atNs = measurement.at ?? 0n;
				const timeStr =
					atNs > 0n
						? clock(new Date(Number(atNs / 1_000_000n)).toISOString())
						: clock("");
				const actStr = action(
					actRaw === 1 ? "enter" : actRaw === 2 ? "exit" : "wait",
					1,
					false,
				);

				setActivityLogs((prev) => {
					const entry: LearningActivityEntry = {
						id: nextLogId++,
						time: timeStr,
						message: `${actStr} · edge ${basis(metricMap.edge ?? 0)}`,
						pnl: metricMap.edge ?? 0,
						action: actStr,
					};
					return [entry, ...prev].slice(0, 10);
				});

				// Check excursion state if present in measurement
				if (
					metricMap.excursion_type !== undefined &&
					metricMap.excursion_type > 0
				) {
					let extType: "UPWARD EXCURSION" | "DOWNWARD EXCURSION" | "CHOPPY MARKET" | "FLAT TAPE" = "UPWARD EXCURSION";
					if (metricMap.excursion_type === 2) extType = "DOWNWARD EXCURSION";
					if (metricMap.excursion_type === 3) extType = "CHOPPY MARKET";
					if (metricMap.excursion_type === 4) extType = "FLAT TAPE";

					setExcursionEvent({
						type: extType,
						magnitude: metricMap.excursion_mag ?? 0,
						marks: {
							A: Math.floor(metricMap.mark_a ?? 0),
							B: Math.floor(metricMap.mark_b ?? 0),
							C: Math.floor(metricMap.mark_c ?? 0),
						},
						entryIdx:
							metricMap.agent_entry !== undefined && metricMap.agent_entry > 0
								? Math.floor(metricMap.agent_entry)
								: null,
						exitIdx:
							metricMap.agent_exit !== undefined && metricMap.agent_exit > 0
								? Math.floor(metricMap.agent_exit)
								: null,
					});
				} else {
					setExcursionEvent(null);
				}
			});
		};

		const ring = getTrainingRing(signals.training?.state, currentSymbol);
		if (ring) {
			handleRing(ring);
		}

		const unsub = signals.training.subscribe((state) => {
			const activeRing = getTrainingRing(state, currentSymbol);
			if (activeRing) {
				handleRing(activeRing);
			}
		});

		return () => {
			unsub?.unsubscribe?.();
		};
	}, [currentSymbol]);

	// Scales for real tape rendering
	const { xScale, yScale, currentPoints, lineGenerator } = useMemo(() => {
		const count = Math.max(points.length, 50);
		const xs = d3.scaleLinear().domain([0, count]).range([0, tapeDim.width]);

		let minY = Number.POSITIVE_INFINITY;
		let maxY = Number.NEGATIVE_INFINITY;
		for (const pt of points) {
			if (pt.y < minY) minY = pt.y;
			if (pt.y > maxY) maxY = pt.y;
		}

		if (!Number.isFinite(minY) || !Number.isFinite(maxY)) {
			minY = 0;
			maxY = 100;
		} else if (minY === maxY) {
			const delta = minY !== 0 ? Math.abs(minY) * 0.005 : 1;
			minY -= delta;
			maxY += delta;
		} else {
			const pad = (maxY - minY) * 0.15;
			minY -= pad;
			maxY += pad;
		}

		const ys = d3
			.scaleLinear()
			.domain([minY, maxY])
			.range([tapeDim.height - 35, 35]);

		const lg = d3
			.line<ForwardTapePoint>()
			.x((d) => xs(d.x))
			.y((d) => ys(d.y))
			.curve(d3.curveMonotoneX);

		return {
			xScale: xs,
			yScale: ys,
			currentPoints: points,
			lineGenerator: lg,
		};
	}, [points, tapeDim.width, tapeDim.height]);

	const isForward = stageCode >= 2;
	const precursorTokens = useMemo(() => {
		if (rawPrecursorTokens.length > 0) {
			return rawPrecursorTokens.map((t) => `[${t}]`);
		}
		if (precursorLength === 0) {
			return ["―"];
		}
		return ["unavailable"];
	}, [rawPrecursorTokens, precursorLength]);

	return (
		<div className="flex flex-col w-full h-full gap-2 font-mono text-[11px] bg-(--bg) p-2 overflow-hidden text-(--f2)">
			{/* TOP ROW: Real Tape + Right Sidebar */}
			<div className="flex h-3/5 gap-2 min-h-0">
				{/* Main Episodic Tape */}
				<div className="flex-1 bg-(--surface) border-(--line) border rounded flex flex-col relative min-w-0">
					{/* Tape Header */}
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 justify-between text-(--f3) shrink-0">
						<div className="flex items-center gap-2">
							<span
								data-l="tape-title"
								className={cn(
									"font-bold px-1.5 py-0.5 rounded text-[10px]",
									isForward
										? "bg-(--acc)/10 text-(--acc) border border-(--acc)/30"
										: "bg-(--info)/10 text-(--info) border border-(--info)/30",
								)}
							>
								{isForward ? "LIVE FORWARD PAPER TAPE" : "HISTORICAL REPLAY TAPE"}
							</span>
							<span className="bg-(--surface) border-(--line) border px-1.5 py-0.5 rounded text-[10px] text-(--f1)">
								{currentSymbol}
							</span>
							<ChevronRight className="w-3 h-3 text-(--f4)" />
							<span className="text-(--f1)">
								{points.length} frames evaluated
							</span>
						</div>
						<div className="flex items-center gap-3">
							{currentPoints.length > 0 && (
								<span className="text-[11px] font-bold text-(--acc) bg-(--acc)/10 px-2 py-0.5 rounded border border-(--acc)/20">
									$
									{currentPoints[currentPoints.length - 1].y.toLocaleString(
										undefined,
										{
											minimumFractionDigits: 2,
											maximumFractionDigits: 2,
										},
									)}
								</span>
							)}
							<span className="text-[10px] uppercase tracking-widest text-(--f4)">
								STAGE:{" "}
								<span data-l="training-stage" className="border-(--line) border text-(--f2) px-1 rounded ml-1 font-bold">
									{stageName}
								</span>
							</span>
							<button
								type="button"
								onClick={() => setIsPlaying(!isPlaying)}
								className="text-(--acc) hover:text-(--f1) transition-colors p-1"
								title={isPlaying ? "Pause tape" : "Resume tape"}
							>
								{isPlaying ? (
									<Pause className="w-3.5 h-3.5" />
								) : (
									<Play className="w-3.5 h-3.5" />
								)}
							</button>
						</div>
					</div>

					{/* Confirmed Excursion Banner */}
					<AnimatePresence>
						{excursionEvent && (
							<motion.div
								initial={{ height: 0, opacity: 0 }}
								animate={{ height: 22, opacity: 1 }}
								exit={{ height: 0, opacity: 0 }}
								className={cn(
									"border-b flex items-center px-3 text-[10px] font-bold z-10 shrink-0",
									excursionEvent.type === "UPWARD EXCURSION"
										? "bg-(--up)/10 border-(--up)/20 text-(--up)"
										: excursionEvent.type === "DOWNWARD EXCURSION"
											? "bg-(--down)/10 border-(--down)/20 text-(--down)"
											: "bg-(--sunken) border-(--line) text-(--f3)",
								)}
							>
								<span>
									{excursionEvent.type}{" "}
									<span className="text-(--f3) ml-1 font-normal">
										confirmed
									</span>
								</span>
								<span className="ml-auto">
									{excursionEvent.type === "UPWARD EXCURSION" ? "+" : "-"}
									{excursionEvent.magnitude.toFixed(2)}%
								</span>
							</motion.div>
						)}
					</AnimatePresence>

					{/* SVG Tape Canvas */}
					<div ref={tapeRef} className="flex-1 relative overflow-hidden">
						{points.length === 0 && (
							<div className="absolute inset-0 flex items-center justify-center text-(--f4) text-xs tracking-wider">
								Awaiting {isForward ? "live market forward" : "historical replay"} tape stream for {currentSymbol}...
							</div>
						)}

						{tapeDim.width > 0 && points.length > 0 && (
							<svg
								width={tapeDim.width}
								height={tapeDim.height}
								className="absolute inset-0"
							>
								<title>Tape of {currentSymbol}</title>
								{/* Midpoint Guideline */}
								<g
									className="text-(--line) stroke-current"
									strokeWidth="1"
									strokeDasharray="2 4"
								>
									<line
										x1="0"
										y1={tapeDim.height / 2}
										x2={tapeDim.width}
										y2={tapeDim.height / 2}
									/>
								</g>

								{/* Price Trajectory Path */}
								<path
									d={lineGenerator(currentPoints) || undefined}
									fill="none"
									stroke="var(--acc)"
									strokeWidth="1.5"
								/>

								{/* Leading Point & Real Price Badge */}
								{currentPoints.length > 0 && (
									<g
										transform={`translate(${xScale(currentPoints[currentPoints.length - 1].x)}, ${yScale(currentPoints[currentPoints.length - 1].y)})`}
									>
										<circle r={3.5} fill="var(--acc)" />
										<text
											x={-6}
											y={-8}
											fill="var(--acc)"
											fontSize="9px"
											fontWeight="bold"
											textAnchor="end"
										>
											$
											{currentPoints[currentPoints.length - 1].y.toLocaleString(
												undefined,
												{
													minimumFractionDigits: 2,
													maximumFractionDigits: 2,
												},
											)}
										</text>
									</g>
								)}

								{/* Hindsight Markers A, B, C and Decisions */}
								{excursionEvent && points.length > 0 && (() => {
									const resolveIdx = (tickSeq: number | null): number | null => {
										if (tickSeq === null || tickSeq <= 0 || points.length === 0) return null;
										let minDiff = Number.POSITIVE_INFINITY;
										let found = -1;
										for (let i = 0; i < points.length; i++) {
											const seq = points[i].seq;
											if (seq === undefined || seq < 0) continue;
											const diff = Math.abs(seq - tickSeq);
											if (diff < minDiff) {
												minDiff = diff;
												found = i;
											}
										}
										return found >= 0 ? found : null;
									};

									const markAIdx =
										excursionEvent.marks.A > 0
											? resolveIdx(excursionEvent.marks.A)
											: null;
									const markBIdx =
										excursionEvent.marks.B > 0
											? resolveIdx(excursionEvent.marks.B)
											: null;
									const markCIdx =
										excursionEvent.marks.C > 0
											? resolveIdx(excursionEvent.marks.C)
											: null;

									const marksList: { name: string; idx: number }[] = [];
									if (markAIdx !== null) marksList.push({ name: "A", idx: markAIdx });
									if (markBIdx !== null) marksList.push({ name: "B", idx: markBIdx });
									if (markCIdx !== null) marksList.push({ name: "C", idx: markCIdx });

									const entryPtIdx =
										excursionEvent.entryIdx !== null && excursionEvent.entryIdx > 0
											? resolveIdx(excursionEvent.entryIdx)
											: null;
									const exitPtIdx =
										excursionEvent.exitIdx !== null && excursionEvent.exitIdx > 0
											? resolveIdx(excursionEvent.exitIdx)
											: null;

									return (
										<g>
											{marksList.map(({ name, idx }) => {
												if (idx < 0 || idx >= points.length) return null;
												const xPos = xScale(idx);
												return (
													<g key={name} transform={`translate(${xPos}, 0)`}>
														<line
															x1={0}
															y1={15}
															x2={0}
															y2={tapeDim.height}
															stroke="#0ea5e9"
															strokeWidth="1"
															strokeDasharray="2 4"
															opacity="0.35"
														/>
														<rect
															x={-7}
															y={8}
															width={14}
															height={14}
															fill="#050505"
															stroke="#0ea5e9"
															strokeWidth="1"
														/>
														<text
															x={0}
															y={18}
															fill="#0ea5e9"
															fontSize="9px"
															textAnchor="middle"
														>
															{name}
														</text>
													</g>
												);
											})}

											{/* Entry Boundary Marker (Rule 42: OPPORTUNITY B vs PAPER ENTER) */}
											{entryPtIdx !== null && entryPtIdx >= 0 && entryPtIdx < points.length && (
												<g
													transform={`translate(${xScale(entryPtIdx)}, ${yScale(points[entryPtIdx].y)})`}
												>
													<circle r={4} fill="#22c55e" />
													<text
														x={6}
														y={-6}
														fill="#22c55e"
														fontSize="9px"
														fontWeight="bold"
													>
														{isForward ? "PAPER ENTER" : "OPPORTUNITY B"}
													</text>
													<line
														y2={tapeDim.height}
														stroke="#22c55e"
														opacity="0.3"
													/>
												</g>
											)}

											{/* Paper Entry Fill Marker (Rule 52: Only when paper fill exists in forward stage) */}
											{isForward && isPaperFilled && entryPtIdx !== null && entryPtIdx >= 0 && entryPtIdx < points.length && (
												<g
													transform={`translate(${xScale(entryPtIdx)}, ${yScale(points[entryPtIdx].y) + 14})`}
												>
													<rect x={4} y={-10} width={90} height={12} fill="#050505" stroke="#22c55e" strokeWidth={0.5} />
													<text
														x={6}
														y={-1}
														fill="#22c55e"
														fontSize="8px"
														fontWeight="bold"
													>
														PAPER ENTRY FILL
													</text>
												</g>
											)}

											{/* Exit Boundary Marker (Rule 42: CAUSAL EXIT C vs PAPER EXIT) */}
											{exitPtIdx !== null && exitPtIdx >= 0 && exitPtIdx < points.length && (
												<g
													transform={`translate(${xScale(exitPtIdx)}, ${yScale(points[exitPtIdx].y)})`}
												>
													<circle r={4} fill="#ef4444" />
													<text
														x={6}
														y={-6}
														fill="#ef4444"
														fontSize="9px"
														fontWeight="bold"
													>
														{isForward ? "PAPER EXIT" : "CAUSAL EXIT C"}
													</text>
													<line
														y2={tapeDim.height}
														stroke="#ef4444"
														opacity="0.3"
													/>
												</g>
											)}
										</g>
									);
								})()}
							</svg>
						)}
					</div>

					{/* Temporal Precursor Fragment Bar (Section 36) */}
					<div className="h-7 border-t border-(--line) bg-(--sunken) flex items-center px-3 justify-between text-[10px] text-(--f3) shrink-0">
						<div className="flex items-center gap-1.5" data-l="temporal-precursor">
							<span className="text-(--f4) uppercase tracking-wider font-bold">Temporal Precursor:</span>
							<span className="text-(--acc) font-mono">
								{precursorTokens.join(" → ")}
							</span>
						</div>
						<div className="flex items-center gap-2" data-l="abc-markers">
							<span className="text-(--f4)">Boundaries:</span>
							<span>A: {excursionEvent?.marks.A ?? 0}</span>
							<span>→</span>
							<span className="text-(--up)">B (Entry): {excursionEvent?.marks.B ?? 0}</span>
							<span>→</span>
							<span className="text-(--down)">C (Exit): {excursionEvent?.marks.C ?? 0}</span>
						</div>
						<div className="flex items-center gap-2">
							<span className="text-(--f4)">Pre-Outcome Prediction:</span>
							<span data-l="frozen-prediction" className="font-bold text-(--acc)">
								{frozenAction}
							</span>
							<span className="text-(--f4)">Actual Delayed Label:</span>
							<span data-l="delayed-label" className="font-bold text-(--f1)">
								{delayedLabel}
							</span>
						</div>
					</div>
				</div>

				{/* Right Sidebar: Separated Historical Held-Out & Forward Paper Evidence */}
				<div className="w-72 bg-(--surface) border-(--line) border rounded flex flex-col shrink-0 min-h-0">
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 text-(--f3) shrink-0 justify-between">
						<span className="tracking-widest uppercase font-bold text-[10px]">
							{isForward ? "Forward Paper Authority" : "Historical Validation"}
						</span>
						<span className="text-[10px]">
							{evaluatedCount.toLocaleString()} frames
						</span>
					</div>

					<div className="p-3 flex-1 overflow-y-auto flex flex-col gap-3">
						{/* Historical Held-Out Evidence Card */}
						<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-1.5">
							<div className="uppercase tracking-widest text-(--f4) text-[9px] font-bold flex justify-between">
								<span>Historical Held-Out Evidence</span>
								<span className="text-(--f2)">{histOpportunities} opps</span>
							</div>
							<div className="flex justify-between items-baseline text-[11px]">
								<span className="text-(--f4)">Mean Return:</span>
								<span className="text-(--f1) font-bold" data-metric="hist_mean_return" data-format="insufficient_if_zero">
									{histOpportunities > 0 ? basis(histMeanReturn) : "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px]">
								<span className="text-(--f4)">Lower Bound (L95):</span>
								<span className="text-(--acc) font-bold" data-metric="hist_lower_bound" data-format="insufficient_if_zero">
									{histOpportunities > 0 ? basis(histLowerBound) : "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px] text-(--f3)">
								<span>Correct: <strong className="text-(--up)" data-metric="hist_correct_enter">{histCorrectEnter}</strong></span>
								<span>False: <strong className="text-(--down)" data-metric="hist_false_enter">{histFalseEnter}</strong></span>
								<span>Missed: <strong className="text-(--down)" data-metric="hist_missed_enter">{histMissedEnter}</strong></span>
							</div>
						</div>

						{/* Forward Paper Evidence Card (Rule 41 & Rule 52: Independent from Historical) */}
						<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-1.5">
							<div className="uppercase tracking-widest text-(--f4) text-[9px] font-bold flex justify-between">
								<span>Forward Paper Evidence</span>
								<span className={isForward ? "text-(--up) font-bold" : "text-(--f4)"}>
									{isForward ? "ACTIVE" : "GATED"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[11px]">
								<span className="text-(--f4)">Paper Mean Return:</span>
								<span className="text-(--f1) font-bold" data-metric="fwd_paper_mean_return" data-format="insufficient_if_zero">
									{fwdPaperTrades > 0 ? basis(fwdPaperMeanReturn) : "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px]">
								<span className="text-(--f4)">Paper Lower Bound:</span>
								<span className="text-(--acc) font-bold" data-metric="fwd_paper_lower_bound" data-format="insufficient_if_zero">
									{fwdPaperTrades > 0 ? basis(fwdPaperLowerBound) : "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px] text-(--f3)">
								<span>Trades: <strong className="text-(--f1)" data-metric="fwd_paper_trades" data-format="integer">{fwdPaperTrades}</strong></span>
								<span>Predictions: <strong className="text-(--f2)" data-metric="fwd_enter_predictions" data-format="integer">{fwdPredictions}</strong></span>
							</div>
							<div className="flex justify-between items-baseline text-[10px] text-(--f4)">
								<span>Active Position:</span>
								<span data-l="paper-position" className="text-(--f2) font-bold">
									{openPositionsCount > 0 ? `${openPositionsCount} active` : "None"}
								</span>
							</div>
						</div>

						{/* Stage Gate Status */}
						<div className="text-[10px] border border-(--line) p-2 rounded bg-(--sunken) flex flex-col gap-1">
							<div className="flex justify-between font-bold">
								<span className="text-(--f4) uppercase">Gate Blocker:</span>
								<span data-l="stage-blocker" className="text-(--down) truncate max-w-[130px]">
									{stageBlocker || "None (Ready)"}
								</span>
							</div>
						</div>

						{/* Recent Activity Log */}
						<div className="mt-1 pt-2 border-(--line) border-t flex-1 overflow-hidden flex flex-col">
							<div className="uppercase tracking-widest text-(--f4) text-[9px] mb-1 font-bold">
								Recent Learning Activity
							</div>
							<div className="flex flex-col gap-1.5 overflow-y-auto flex-1">
								<AnimatePresence initial={false}>
									{activityLogs.map((log) => (
										<motion.div
											key={log.id}
											initial={{ opacity: 0, height: 0 }}
											animate={{ opacity: 1, height: "auto" }}
											className="text-[10px]"
										>
											<div className="text-(--f3)">
												{log.time} · {log.message}
											</div>
											<div className="text-(--f4) text-[9px]">
												Outcome:{" "}
												<span
													className={
														log.pnl >= 0 ? "text-(--up)" : "text-(--down)"
													}
												>
													{log.pnl >= 0 ? "+" : ""}
													{log.pnl.toFixed(1)} bp
												</span>
											</div>
										</motion.div>
									))}
								</AnimatePresence>
								{activityLogs.length === 0 && (
									<div className="text-(--f4) text-[10px]">
										Listening for model decisions...
									</div>
								)}
							</div>
						</div>
					</div>
				</div>
			</div>

			{/* BOTTOM ROW: Real Radix Trie Memory Table */}
			<div className="flex h-2/5 gap-2 min-h-0">
				{/* Radix Trie Memory Table */}
				<div className="flex-1 bg-(--surface) border-(--line) border rounded flex flex-col min-w-0">
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 justify-between shrink-0">
						<span className="text-(--f3) uppercase tracking-widest text-[10px] font-bold">
							Radix Trie Memory (Active Learned Branches)
						</span>
						<span className="text-(--f4) text-[10px]">
							{trieBranches
								.reduce((acc, p) => acc + p.visits, 0)
								.toLocaleString()}{" "}
							observations routed
						</span>
					</div>

					<div className="flex-1 overflow-auto p-2">
						<table className="w-full text-left border-collapse text-[10px]">
							<thead>
								<tr className="text-(--f4) border-(--line) border-b">
									<th className="font-normal pb-1.5 px-2">Path Signature</th>
									<th className="font-normal pb-1.5 px-2 text-right">Depth</th>
									<th className="font-normal pb-1.5 px-2 text-right">Visits</th>
									<th className="font-normal pb-1.5 px-2 text-right">
										Policy Bias
									</th>
									<th className="font-normal pb-1.5 px-2">Confidence</th>
									<th className="font-normal pb-1.5 px-2 text-right">
										Learned Policy
									</th>
								</tr>
							</thead>
							<tbody>
								{trieBranches.map((p, i) => (
									<tr
										key={p.hash}
										className="border-(--line)/50 border-b last:border-0 hover:bg-(--raised)"
									>
										<td
											className={cn(
												"py-1.5 px-2 font-mono",
												i === 0 ? "text-(--acc)" : "text-(--f2)",
											)}
										>
											{p.hash}
										</td>
										<td className="py-1.5 px-2 text-right">{p.depth}</td>
										<td className="py-1.5 px-2 text-right">
											{p.visits.toLocaleString()}
										</td>
										<td
											className={cn(
												"py-1.5 px-2 text-right font-bold",
												p.meanEdge >= 0 ? "text-(--up)" : "text-(--down)",
											)}
										>
											{p.meanEdge >= 0 ? "+" : ""}
											{(p.meanEdge * 100).toFixed(1)}%
										</td>
										<td className="py-1.5 px-2">
											<div className="flex items-center gap-2">
												<div className="w-12 h-1 bg-(--sunken) rounded overflow-hidden">
													<div
														className="h-full bg-(--info)"
														style={{ width: `${p.confidence}%` }}
													/>
												</div>
												<span>{p.confidence.toFixed(1)}%</span>
											</div>
										</td>
										<td className="py-1.5 px-2 text-right">
											<span
												className={cn(
													"px-1.5 py-0.5 rounded text-[8px] border font-bold",
													p.policy === "ENTER"
														? "bg-(--up)/10 border-(--up)/30 text-(--up)"
														: "bg-(--line2) border-(--line) text-(--f3)",
												)}
											>
												{p.policy}
											</span>
										</td>
									</tr>
								))}
								{trieBranches.length === 0 && (
									<tr>
										<td colSpan={6} className="py-4 text-center text-(--f4)">
											No cognitive radix branches registered yet.
										</td>
									</tr>
								)}
							</tbody>
						</table>
					</div>
				</div>
			</div>
		</div>
	);
};
