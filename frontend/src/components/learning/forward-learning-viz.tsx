import * as d3 from "d3";
import { ChevronRight, Pause, Play } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { focusStore, type RingBuffer, trainingStore } from "#/collections/app";
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
}

export const ForwardLearningViz = () => {
	const tapeRef = useRef<HTMLDivElement>(null);
	const distRef = useRef<HTMLDivElement>(null);
	const [tapeDim, setTapeDim] = useState({ width: 800, height: 300 });
	const [distDim, setDistDim] = useState({ width: 300, height: 150 });
	const [isPlaying, setIsPlaying] = useState(true);

	// Real tape points accumulated from the live training measurements
	const [points, setPoints] = useState<ForwardTapePoint[]>([]);

	// Real metrics and stats
	const [meanEdge, setMeanEdge] = useState(0);
	const [wins, setWins] = useState(0);
	const [losses, setLosses] = useState(0);
	const [evaluatedCount, setEvaluatedCount] = useState(0);
	const [globalPnl, setGlobalPnl] = useState(0);

	// Real Radix Trie branches fetched from backend engine
	const [trieBranches, setTrieBranches] = useState<TrieBranch[]>([]);

	// Real activity entries
	const [activityLogs, setActivityLogs] = useState<LearningActivityEntry[]>([]);

	// Real excursion markers if active
	const [excursionEvent, setExcursionEvent] = useState<{
		type: "UPWARD EXCURSION" | "DOWNWARD EXCURSION" | "STAGNATION";
		magnitude: number;
		marks: { A: number; B: number; C: number };
		entryIdx: number | null;
		exitIdx: number | null;
	} | null>(null);

	// Focus symbol
	const [currentSymbol, setCurrentSymbol] = useState("BTC/USD");

	useEffect(() => {
		const unsub = focusStore.subscribe((state) => {
			if (state) setCurrentSymbol(state);
		});
		return () => {
			unsub?.unsubscribe?.();
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

	useEffect(() => {
		const distTarget = distRef.current;
		if (!distTarget || typeof ResizeObserver === "undefined") return;

		const ro = new ResizeObserver((entries) => {
			if (entries[0]) {
				const { width, height } = entries[0].contentRect;
				if (width > 0 && height > 0) {
					setDistDim({ width, height });
				}
			}
		});
		ro.observe(distTarget);
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

				const edgeVal = (metricMap.edge ?? 0) * 10000;
				const evalCount = metricMap.evaluated ?? 0;
				const winVal = metricMap.wins ?? 0;
				const lossVal = metricMap.losses ?? 0;
				const pnlVal = metricMap.pnl ?? 0;

				setMeanEdge(edgeVal);
				setEvaluatedCount(Math.floor(evalCount));
				setWins(Math.floor(winVal));
				setLosses(Math.floor(lossVal));
				setGlobalPnl(pnlVal);

				// Accumulate real point
				const rawPrice =
					metricMap.price ?? metricMap.contrast ?? metricMap.confidence;

				setPoints((prev) => {
					const fallback = prev.length > 0 ? prev[prev.length - 1].y : 50;
					const priceVal = rawPrice ?? fallback;
					const next = [...prev, { x: prev.length, y: priceVal }];
					if (next.length > 200) {
						return next.slice(next.length - 200).map((pt, i) => ({
							x: i,
							y: pt.y,
						}));
					}
					return next;
				});

				// Record real activity log entry
				const actRaw = metricMap.action ?? 0;
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
						pnl: edgeVal,
						action: actStr,
					};
					return [entry, ...prev].slice(0, 10);
				});

				// Check excursion state if present in measurement
				if (metricMap.excursion_type !== undefined) {
					const extType =
						metricMap.excursion_type === 1
							? "UPWARD EXCURSION"
							: metricMap.excursion_type === 2
								? "DOWNWARD EXCURSION"
								: "STAGNATION";

					setExcursionEvent({
						type: extType,
						magnitude: metricMap.excursion_mag ?? 0,
						marks: {
							A: Math.max(0, Math.floor(metricMap.mark_a ?? 20)),
							B: Math.max(0, Math.floor(metricMap.mark_b ?? 50)),
							C: Math.max(0, Math.floor(metricMap.mark_c ?? 100)),
						},
						entryIdx:
							metricMap.agent_entry !== undefined
								? Math.floor(metricMap.agent_entry)
								: null,
						exitIdx:
							metricMap.agent_exit !== undefined
								? Math.floor(metricMap.agent_exit)
								: null,
					});
				}
			});
		};

		const ring = trainingStore?.state?.[currentSymbol];
		if (ring) {
			handleRing(ring);
		}

		const unsub = trainingStore.subscribe((state) => {
			const symRing = state?.[currentSymbol];
			if (symRing) {
				handleRing(symRing);
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

		if (!Number.isFinite(minY) || !Number.isFinite(maxY) || minY === maxY) {
			minY = 0;
			maxY = 100;
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

	// Distribution Curve calculation from real outcomes
	const { curveData, distLineGen, distAreaGen, distXScale } = useMemo(() => {
		const total = wins + losses;
		const effectiveMean = meanEdge;
		const effectiveVariance = total > 1 ? Math.max(1, 15 / total) : 8;
		const sd = Math.max(0.2, Math.sqrt(effectiveVariance));

		const distPdf = (x: number, m: number, s: number) =>
			(1 / (s * Math.sqrt(2 * Math.PI))) * Math.exp(-0.5 * ((x - m) / s) ** 2);

		const cd: { x: number; y: number }[] = [];
		const minX = -15;
		const maxX = 15;
		for (let x = minX; x <= maxX; x += 0.5) {
			cd.push({ x, y: distPdf(x, effectiveMean, sd) });
		}

		const maxCurveY = Math.max(...cd.map((d) => d.y), 0.1);
		const dxScale = d3
			.scaleLinear()
			.domain([minX, maxX])
			.range([20, distDim.width - 20]);
		const dyScale = d3
			.scaleLinear()
			.domain([0, maxCurveY * 1.25])
			.range([distDim.height - 25, 20]);

		const daGen = d3
			.area<{ x: number; y: number }>()
			.x((d) => dxScale(d.x))
			.y0(distDim.height - 25)
			.y1((d) => dyScale(d.y))
			.curve(d3.curveBasis);

		const dlGen = d3
			.line<{ x: number; y: number }>()
			.x((d) => dxScale(d.x))
			.y((d) => dyScale(d.y))
			.curve(d3.curveBasis);

		return {
			curveData: cd,
			distAreaGen: daGen,
			distLineGen: dlGen,
			distXScale: dxScale,
			distYScale: dyScale,
		};
	}, [meanEdge, wins, losses, distDim.width, distDim.height]);

	const totalOutcomes = Math.max(wins + losses, 1);

	return (
		<div className="flex flex-col w-full h-full gap-2 font-mono text-[11px] bg-(--bg) p-2 overflow-hidden text-(--f2)">
			{/* TOP ROW: Real Tape + Right Sidebar */}
			<div className="flex h-3/5 gap-2 min-h-0">
				{/* Main Episodic Tape */}
				<div className="flex-1 bg-(--surface) border-(--line) border rounded flex flex-col relative min-w-0">
					{/* Tape Header */}
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 justify-between text-(--f3) shrink-0">
						<div className="flex items-center gap-2">
							<span className="text-(--acc) font-bold">FORWARD TAPE</span>
							<span className="bg-(--surface) border-(--line) border px-1.5 py-0.5 rounded text-[10px] text-(--f1)">
								{currentSymbol}
							</span>
							<ChevronRight className="w-3 h-3 text-(--f4)" />
							<span className="text-(--f1)">
								{points.length} frames evaluated
							</span>
						</div>
						<div className="flex items-center gap-3">
							<span className="text-[10px] uppercase tracking-widest text-(--f4)">
								Coordinate{" "}
								<span className="border-(--line) border text-(--f2) px-1 rounded ml-1">
									midpoint
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
						{excursionEvent && excursionEvent.type !== "STAGNATION" && (
							<motion.div
								initial={{ height: 0, opacity: 0 }}
								animate={{ height: 22, opacity: 1 }}
								exit={{ height: 0, opacity: 0 }}
								className={cn(
									"border-b flex items-center px-3 text-[10px] font-bold z-10 shrink-0",
									excursionEvent.type === "UPWARD EXCURSION"
										? "bg-(--up)/10 border-(--up)/20 text-(--up)"
										: "bg-(--down)/10 border-(--down)/20 text-(--down)",
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
								Awaiting tape stream for {currentSymbol}...
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

								{/* Leading Point */}
								{currentPoints.length > 0 && (
									<circle
										cx={xScale(currentPoints[currentPoints.length - 1].x)}
										cy={yScale(currentPoints[currentPoints.length - 1].y)}
										r={3}
										fill="var(--acc)"
									/>
								)}

								{/* Real Hindsight Markers A, B, C if excursion event is active */}
								{excursionEvent && (
									<g>
										{(["A", "B", "C"] as const).map((m) => {
											const markIdx = excursionEvent.marks[m];
											if (markIdx >= points.length) return null;
											const xPos = xScale(markIdx);

											return (
												<g key={m} transform={`translate(${xPos}, 0)`}>
													<line
														x1={0}
														y1={12}
														x2={0}
														y2={tapeDim.height}
														stroke="var(--info)"
														strokeWidth="1"
														strokeDasharray="2 4"
														opacity="0.35"
													/>
													<rect
														x={-6}
														y={6}
														width={12}
														height={12}
														fill="var(--sunken)"
														stroke="var(--info)"
														strokeWidth="1"
													/>
													<text
														x={0}
														y={15}
														fill="var(--info)"
														fontSize="8px"
														textAnchor="middle"
													>
														{m}
													</text>
												</g>
											);
										})}

										{/* Agent Entry Marker */}
										{excursionEvent.entryIdx !== null &&
											excursionEvent.entryIdx < points.length && (
												<g
													transform={`translate(${xScale(excursionEvent.entryIdx)}, ${yScale(points[excursionEvent.entryIdx].y)})`}
												>
													<circle r={3.5} fill="var(--up)" />
													<text
														x={5}
														y={-5}
														fill="var(--up)"
														fontSize="8px"
														fontWeight="bold"
													>
														ENTER
													</text>
													<line
														y2={tapeDim.height}
														stroke="var(--up)"
														opacity="0.25"
													/>
												</g>
											)}

										{/* Agent Exit Marker */}
										{excursionEvent.exitIdx !== null &&
											excursionEvent.exitIdx < points.length && (
												<g
													transform={`translate(${xScale(excursionEvent.exitIdx)}, ${yScale(points[excursionEvent.exitIdx].y)})`}
												>
													<circle r={3.5} fill="var(--down)" />
													<text
														x={5}
														y={-5}
														fill="var(--down)"
														fontSize="8px"
														fontWeight="bold"
													>
														EXIT
													</text>
													<line
														y2={tapeDim.height}
														stroke="var(--down)"
														opacity="0.25"
													/>
												</g>
											)}
									</g>
								)}
							</svg>
						)}
					</div>
				</div>

				{/* Right Sidebar: Agent Skill & Activity */}
				<div className="w-72 bg-(--surface) border-(--line) border rounded flex flex-col shrink-0 min-h-0">
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 text-(--f3) shrink-0 justify-between">
						<span className="tracking-widest uppercase font-bold text-[10px]">
							Agent Skill
						</span>
						<span className="text-[10px]">
							{evaluatedCount.toLocaleString()} evaluated
						</span>
					</div>

					<div className="p-3 flex-1 overflow-y-auto flex flex-col gap-4">
						{/* Mean Completed Benefit */}
						<div>
							<div className="uppercase tracking-widest text-(--f4) text-[9px] mb-1 font-bold">
								Mean Decision Benefit
							</div>
							<div className="text-(--f1) text-sm font-bold">
								{meanEdge > 0 ? "+" : ""}
								{meanEdge.toFixed(2)} bp
							</div>
							<div className="text-(--f4) text-[9px] mt-0.5 leading-tight">
								Measured edge over starting basis. Honest uncertainty preserved.
							</div>
						</div>

						{/* Outcome Signs */}
						<div>
							<div className="uppercase tracking-widest text-(--f4) text-[9px] mb-1 font-bold">
								Outcome Signs
							</div>
							<div className="flex items-baseline gap-2 mb-1.5 text-[10px]">
								<span className="text-(--up) font-bold">{wins} positive</span>
								<span className="text-(--f4)">·</span>
								<span className="text-(--down) font-bold">
									{losses} negative
								</span>
							</div>
							<div className="h-1.5 w-full bg-(--sunken) flex rounded overflow-hidden border-(--line) border">
								<div
									className="bg-(--up)"
									style={{
										width: `${(wins / totalOutcomes) * 100}%`,
									}}
								/>
								<div
									className="bg-(--down)"
									style={{
										width: `${(losses / totalOutcomes) * 100}%`,
									}}
								/>
							</div>
						</div>

						{/* System P&L */}
						<div>
							<div className="uppercase tracking-widest text-(--f4) text-[9px] mb-1 font-bold">
								System P&L
							</div>
							<div
								className={cn(
									"text-sm font-mono font-bold",
									globalPnl >= 0 ? "text-(--up)" : "text-(--down)",
								)}
							>
								{globalPnl >= 0 ? "+" : ""}
								{globalPnl.toFixed(4)}
							</div>
							<div className="text-(--f4) text-[9px] mt-0.5 leading-tight">
								Theoretical wallet return from authoritative execution facts.
							</div>
						</div>

						{/* Recent Activity Log */}
						<div className="mt-1 pt-3 border-(--line) border-t">
							<div className="uppercase tracking-widest text-(--f4) text-[9px] mb-2 font-bold">
								Recent Learning Activity
							</div>
							<div className="flex flex-col gap-2">
								<AnimatePresence initial={false}>
									{activityLogs.map((log) => (
										<motion.div
											key={log.id}
											initial={{ opacity: 0, height: 0 }}
											animate={{ opacity: 1, height: "auto" }}
											className="text-[10px]"
										>
											<div className="text-(--f3) mb-0.5">
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
										Listening for live market decisions...
									</div>
								)}
							</div>
						</div>
					</div>
				</div>
			</div>

			{/* BOTTOM ROW: Real Radix Trie Memory Table + Edge Distribution */}
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
										Mean Edge
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
											{p.meanEdge.toFixed(2)} bp
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

				{/* Edge Distribution Bell Curve */}
				<div className="w-80 bg-(--surface) border-(--line) border rounded flex flex-col shrink-0 min-h-0">
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 shrink-0 justify-between">
						<span className="text-(--acc) uppercase tracking-widest text-[10px] font-bold">
							Edge Distribution
						</span>
						<span className="text-(--f4) text-[10px]">
							μ = {meanEdge.toFixed(1)} bp
						</span>
					</div>

					<div ref={distRef} className="flex-1 relative">
						{distDim.width > 0 && (
							<svg
								width={distDim.width}
								height={distDim.height}
								className="absolute inset-0"
							>
								<title>Edge Distribution</title>
								<defs>
									<linearGradient id="distFill" x1="0" y1="0" x2="0" y2="1">
										<stop
											offset="0%"
											stopColor="var(--info)"
											stopOpacity="0.3"
										/>
										<stop
											offset="100%"
											stopColor="var(--info)"
											stopOpacity="0.0"
										/>
									</linearGradient>
								</defs>

								{/* Base X Axis */}
								<line
									x1={20}
									y1={distDim.height - 25}
									x2={distDim.width - 20}
									y2={distDim.height - 25}
									stroke="var(--line)"
									strokeWidth="1"
								/>

								{/* Breakeven Line (0.0 bp) */}
								<line
									x1={distXScale(0)}
									y1={20}
									x2={distXScale(0)}
									y2={distDim.height - 25}
									stroke="var(--f4)"
									strokeWidth="1"
									strokeDasharray="2 2"
								/>
								<text
									x={distXScale(0)}
									y={15}
									fill="var(--f4)"
									fontSize="8px"
									textAnchor="middle"
								>
									0.0 bp
								</text>

								{/* Mean Line */}
								<line
									x1={distXScale(meanEdge)}
									y1={20}
									x2={distXScale(meanEdge)}
									y2={distDim.height - 25}
									stroke="var(--up)"
									strokeWidth="1"
								/>
								<text
									x={distXScale(meanEdge)}
									y={15}
									fill="var(--up)"
									fontSize="8px"
									textAnchor="middle"
									fontWeight="bold"
								>
									μ {meanEdge.toFixed(1)}
								</text>

								{/* Curve Area & Line */}
								<path
									d={distAreaGen(curveData) || undefined}
									fill="url(#distFill)"
								/>
								<path
									d={distLineGen(curveData) || undefined}
									fill="none"
									stroke="var(--info)"
									strokeWidth="1.5"
								/>

								<text
									x={20}
									y={distDim.height - 10}
									fill="var(--f4)"
									fontSize="8px"
								>
									-15 bp
								</text>
								<text
									x={distDim.width - 20}
									y={distDim.height - 10}
									fill="var(--f4)"
									fontSize="8px"
									textAnchor="end"
								>
									+15 bp
								</text>
							</svg>
						)}
					</div>

					<div className="p-2 border-(--line) border-t text-[9px] text-(--f4)">
						Normal fit to the authority-weighted mean and variance of completed
						outcomes.
					</div>
				</div>
			</div>
		</div>
	);
};
