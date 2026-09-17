import { useEffect, useRef, useState } from "react";
import { line as d3Line, curveMonotoneX } from "d3-shape";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { DistributionCurve } from "#/components/ui/distribution-curve";
import { RatioBar } from "#/components/ui/ratio-bar";
import { Typography } from "#/components/ui/typography";
import { basis, clock, percent } from "./format";

type EpisodeType = "UPWARD EXCURSION" | "DOWNWARD EXCURSION" | "STAGNATION";

interface TrainingEpisode {
	id: number;
	type: EpisodeType;
	length: number;
	points: { x: number; y: number }[];
	marks: { A: number; B: number; C: number };
	model: {
		entryIdx: number | null;
		exitIdx: number | null;
		reward: number;
	};
	magnitude: number;
}

interface TriePath {
	hash: string;
	depth: number;
	visits: number;
	meanEdge: number;
	confidence: number;
	policy: "ENTER" | "WAIT" | "RETREAT";
}

interface ActivityEntry {
	id: number;
	time: string;
	action: string;
	pnl: number;
}

const generateEpisode = (id: number): TrainingEpisode => {
	const length = 250;
	const baseA = 30;
	const A = baseA + Math.floor(Math.random() * 20);
	const B = A + 30 + Math.floor(Math.random() * 20);
	const C = B + 60 + Math.floor(Math.random() * 40);

	const r = Math.random();
	let type: EpisodeType = "STAGNATION";
	let direction = 0;
	if (r > 0.6) {
		type = "UPWARD EXCURSION";
		direction = 1;
	} else if (r > 0.3) {
		type = "DOWNWARD EXCURSION";
		direction = -1;
	}

	const points: { x: number; y: number }[] = [];
	let y = 50;
	let magnitude = 0;

	for (let i = 0; i < length; i++) {
		y += (Math.random() - 0.5) * 1.6;
		if (i > A && i < C && type !== "STAGNATION") {
			const strength = i < B ? 0.4 : 0.2;
			y -= direction * (Math.random() * strength * 2);
			magnitude += direction * strength;
		}
		y += (50 - y) * 0.01;
		y = Math.max(10, Math.min(90, y));
		points.push({ x: i, y });
	}

	let entryIdx: number | null = null;
	let exitIdx: number | null = null;
	let reward = 0;

	if (type !== "STAGNATION") {
		entryIdx = B + Math.floor((Math.random() - 0.3) * 15);
		exitIdx = C + Math.floor((Math.random() - 0.5) * 20);
		const entryError = Math.abs(entryIdx - B);
		const exitError = Math.abs(exitIdx - C);
		reward = 8 - entryError * 0.15 - exitError * 0.1;
		if (Math.random() > 0.88) {
			entryIdx = null;
			exitIdx = null;
			reward = -4.5;
		}
	} else {
		if (Math.random() > 0.8) {
			entryIdx = A + Math.floor(Math.random() * 40);
			exitIdx = entryIdx + 40;
			reward = -3.2;
		} else {
			reward = 2.4;
		}
	}

	return {
		id,
		type,
		length,
		points,
		marks: { A, B, C },
		model: { entryIdx, exitIdx, reward },
		magnitude: Math.abs(magnitude / 100),
	};
};

export interface ForwardViewProps {
	liveEdge?: number;
	liveDecisions?: number;
	liveAccuracy?: number;
}

export const ForwardView = ({
	liveEdge,
	liveDecisions,
	liveAccuracy,
}: ForwardViewProps) => {
	const tapeRef = useRef<HTMLDivElement>(null);
	const [tapeDim, setTapeDim] = useState({ width: 600, height: 260 });
	const [isPlaying, setIsPlaying] = useState(true);

	const [history, setHistory] = useState<TrainingEpisode[]>([]);
	const [currentEpisode, setCurrentEpisode] = useState<TrainingEpisode>(() =>
		generateEpisode(1),
	);
	const [tick, setTick] = useState(0);
	const [phase, setPhase] = useState<"PLAYING" | "EVALUATING">("PLAYING");

	const [triePaths, setTriePaths] = useState<TriePath[]>(() => [
		{
			hash: "0x4e8a10",
			depth: 4,
			visits: 3412,
			meanEdge: 2.14,
			confidence: 88.4,
			policy: "ENTER",
		},
		{
			hash: "0x12c9b4",
			depth: 3,
			visits: 2190,
			meanEdge: 1.05,
			confidence: 72.1,
			policy: "ENTER",
		},
		{
			hash: "0x8fa37d",
			depth: 5,
			visits: 1845,
			meanEdge: -0.82,
			confidence: 61.3,
			policy: "WAIT",
		},
		{
			hash: "0x5d02e8",
			depth: 4,
			visits: 1420,
			meanEdge: 0.44,
			confidence: 55.0,
			policy: "WAIT",
		},
		{
			hash: "0x91b402",
			depth: 6,
			visits: 980,
			meanEdge: -1.65,
			confidence: 49.2,
			policy: "RETREAT",
		},
		{
			hash: "0x33e51a",
			depth: 3,
			visits: 870,
			meanEdge: 1.82,
			confidence: 79.5,
			policy: "ENTER",
		},
	]);

	const [globalPnl, setGlobalPnl] = useState(-1.248);
	const [logs, setLogs] = useState<ActivityEntry[]>(() => [
		{
			id: 1,
			time: clock(new Date().toISOString()),
			action: "policy ENTER · @ask fill",
			pnl: 3.42,
		},
		{
			id: 2,
			time: clock(new Date(Date.now() - 4000).toISOString()),
			action: "policy WAIT · neutral impulse",
			pnl: 1.15,
		},
		{
			id: 3,
			time: clock(new Date(Date.now() - 8000).toISOString()),
			action: "policy RETREAT · adverse shift",
			pnl: -0.92,
		},
	]);

	useEffect(() => {
		const target = tapeRef.current;
		if (!target || typeof ResizeObserver === "undefined") return;

		const observer = new ResizeObserver((entries) => {
			if (entries[0]) {
				const { width, height } = entries[0].contentRect;
				if (width > 0 && height > 0) {
					setTapeDim({ width, height });
				}
			}
		});

		observer.observe(target);
		return () => observer.disconnect();
	}, []);

	// Main Simulation Loop
	useEffect(() => {
		if (!isPlaying) return;

		let timer: number;
		if (phase === "PLAYING") {
			timer = window.setInterval(() => {
				setTick((t) => {
					if (t >= currentEpisode.length - 1) {
						setPhase("EVALUATING");
						return t;
					}
					return t + 2;
				});
			}, 20);
		} else if (phase === "EVALUATING") {
			timer = window.setTimeout(() => {
				const reward = currentEpisode.model.reward;

				setHistory((prev) => [...prev.slice(-99), currentEpisode]);
				setGlobalPnl((prev) => prev + reward * 0.04);

				setTriePaths((prev) => {
					const updated = [...prev];
					if (updated[0]) {
						updated[0].visits += 1;
						updated[0].meanEdge = updated[0].meanEdge * 0.95 + reward * 0.05;
						updated[0].confidence = Math.min(
							99.5,
							Math.max(10, updated[0].confidence + (reward > 0 ? 0.6 : -0.6)),
						);
					}
					return updated;
				});

				const timeStr = clock(new Date().toISOString());
				const actDesc = currentEpisode.model.entryIdx
					? "policy ENTER · excursion capture"
					: "policy WAIT · stationary band";

				setLogs((prev) => [
					{
						id: Date.now(),
						time: timeStr,
						action: actDesc,
						pnl: reward,
					},
					...prev.slice(0, 7),
				]);

				setCurrentEpisode(generateEpisode(currentEpisode.id + 1));
				setTick(0);
				setPhase("PLAYING");
			}, 2000);
		}

		return () => {
			clearInterval(timer);
			clearTimeout(timer);
		};
	}, [isPlaying, phase, currentEpisode]);

	// Geometry calculations
	const currentPoints = currentEpisode.points.slice(0, tick);
	const usableTapeW = Math.max(1, tapeDim.width);
	const usableTapeH = Math.max(1, tapeDim.height);

	const scaleX = (x: number) => (x / 250) * usableTapeW;
	const scaleY = (y: number) =>
		usableTapeH - 24 - (y / 100) * (usableTapeH - 48);

	const lineGen = d3Line<{ x: number; y: number }>()
		.x((d) => scaleX(d.x))
		.y((d) => scaleY(d.y))
		.curve(curveMonotoneX);

	const tapePath = lineGen(currentPoints) ?? "";

	// Stats for Distribution Curve
	const wins = history.filter((h) => h.model.reward > 0).length;
	const losses = history.filter((h) => h.model.reward < 0).length;
	const meanEdge =
		liveEdge !== undefined
			? liveEdge
			: history.length > 0
				? history.reduce((sum, e) => sum + e.model.reward, 0) / history.length
				: 1.25;

	const variance =
		history.length > 1
			? history.reduce((acc, ep) => acc + (ep.model.reward - meanEdge) ** 2, 0) /
				(history.length - 1)
			: 9.0;
	const sd = Math.max(0.5, Math.sqrt(variance));

	return (
		<div className="flex h-full w-full min-h-0 flex-col overflow-hidden bg-(--bg) font-mono text-xs text-(--f2)">
			{/* TOP ROW: Waveform Tape (Screen) + Precursor Performance */}
			<div className="flex h-[58%] min-h-0 border-b border-(--line)">
				{/* Episodic Continuous Tape - Rendered on black screen surface bg-(--sunken) */}
				<div className="flex flex-1 min-w-0 flex-col bg-(--sunken) overflow-hidden relative">
					<div className="flex h-8 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[11px] text-(--f4)">
						<div className="flex items-center gap-2">
							<span className="font-bold text-(--acc)">CONTINUOUS RUN</span>
							<Badge
								variant="brand"
								label={`EP-${currentEpisode.id.toString().padStart(4, "0")}`}
								size="s"
							/>
							<span className="text-(--f3)">→</span>
							<span className="text-(--f1)">FORWARD EVALUATION</span>
						</div>

						<div className="flex items-center gap-3">
							<span className="text-[10px] text-(--f4)">
								Status:{" "}
								<span className="text-(--acc)">
									{phase === "PLAYING" ? "OBSERVING" : "RESOLVING"}
								</span>
							</span>
							<Button
								variant="bare"
								size="s"
								onClick={() => setIsPlaying(!isPlaying)}
								className="text-(--acc) hover:text-(--f1)"
							>
								{isPlaying ? "PAUSE" : "RESUME"}
							</Button>
						</div>
					</div>

					{/* Excursion Banner */}
					{phase === "EVALUATING" && currentEpisode.type !== "STAGNATION" && (
						<div
							className={`flex h-5 shrink-0 items-center justify-between border-b px-4 text-[10px] font-bold ${
								currentEpisode.type === "UPWARD EXCURSION"
									? "border-(--up)/30 bg-(--up)/10 text-(--up)"
									: "border-(--down)/30 bg-(--down)/10 text-(--down)"
							}`}
						>
							<span>{currentEpisode.type} · confirmed</span>
							<span>
								{currentEpisode.type === "UPWARD EXCURSION" ? "+" : "-"}
								{currentEpisode.magnitude.toFixed(2)}%
							</span>
						</div>
					)}

					<div ref={tapeRef} className="relative flex-1 min-h-0 overflow-hidden">
						{tapeDim.width > 0 && (
							<svg
								width={tapeDim.width}
								height={tapeDim.height}
								className="absolute inset-0 block h-full w-full"
							>
								{/* Grid Lines */}
								<line
									x1={0}
									y1={usableTapeH / 2}
									x2={usableTapeW}
									y2={usableTapeH / 2}
									stroke="var(--line)"
									strokeWidth={1}
									strokeDasharray="2 4"
								/>

								{/* Waveform Line */}
								{tapePath && (
									<path
										d={tapePath}
										fill="none"
										stroke="var(--acc)"
										strokeWidth={1.6}
									/>
								)}

								{/* Head Tracker Circle */}
								{currentPoints.length > 0 && (
									<circle
										cx={scaleX(currentPoints[currentPoints.length - 1].x)}
										cy={scaleY(currentPoints[currentPoints.length - 1].y)}
										r={3}
										fill="var(--acc)"
									/>
								)}

								{/* Hindsight Outcome Markers A / B / C */}
								{phase === "EVALUATING" && (
									<g>
										{(["A", "B", "C"] as const).map((marker) => {
											const idx = currentEpisode.marks[marker];
											const posX = scaleX(idx);
											return (
												<g key={marker} transform={`translate(${posX}, 0)`}>
													<line
														x1={0}
														y1={12}
														x2={0}
														y2={usableTapeH}
														stroke="var(--info)"
														strokeWidth={1}
														strokeDasharray="2 3"
														opacity={0.4}
													/>
													<rect
														x={-7}
														y={6}
														width={14}
														height={14}
														fill="var(--sunken)"
														stroke="var(--info)"
														strokeWidth={1}
														rx={2}
													/>
													<text
														x={0}
														y={16}
														fill="var(--info)"
														fontSize="9px"
														textAnchor="middle"
														fontWeight="bold"
													>
														{marker}
													</text>
												</g>
											);
										})}
									</g>
								)}

								{/* Model Policy Action Points */}
								{currentEpisode.model.entryIdx &&
									tick >= currentEpisode.model.entryIdx && (
										<g
											transform={`translate(${scaleX(
												currentEpisode.model.entryIdx,
											)}, ${scaleY(
												currentEpisode.points[currentEpisode.model.entryIdx]?.y ??
													50,
											)})`}
										>
											<circle r={4} fill="var(--up)" />
											<text
												x={6}
												y={-6}
												fill="var(--up)"
												fontSize="9px"
												fontWeight="bold"
											>
												ENTER
											</text>
											<line
												y2={usableTapeH}
												stroke="var(--up)"
												strokeWidth={1}
												opacity={0.3}
											/>
										</g>
									)}

								{currentEpisode.model.exitIdx &&
									tick >= currentEpisode.model.exitIdx && (
										<g
											transform={`translate(${scaleX(
												currentEpisode.model.exitIdx,
											)}, ${scaleY(
												currentEpisode.points[currentEpisode.model.exitIdx]?.y ??
													50,
											)})`}
										>
											<circle r={4} fill="var(--down)" />
											<text
												x={6}
												y={-6}
												fill="var(--down)"
												fontSize="9px"
												fontWeight="bold"
											>
												EXIT
											</text>
											<line
												y2={usableTapeH}
												stroke="var(--down)"
												strokeWidth={1}
												opacity={0.3}
											/>
										</g>
									)}
							</svg>
						)}
					</div>
				</div>

				{/* Precursor Performance & Activity Stream - Flush with border-l */}
				<div className="flex w-80 shrink-0 flex-col border-l border-(--line) bg-(--surface) overflow-hidden min-h-0">
					<div className="flex h-8 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[11px] text-(--f4)">
						<span className="font-bold text-(--f2)">PRECURSOR SKILL</span>
						<span>{history.length} evaluated</span>
					</div>

					<div className="flex flex-1 flex-col gap-3 p-4 overflow-hidden min-h-0">
						{/* Mean Benefit */}
						<div>
							<Typography.Label size="s" tone="f4" weight="normal">
								MEAN DECISION BENEFIT
							</Typography.Label>
							<div className="mt-0.5 flex items-baseline justify-between">
								<span className="text-base font-bold text-(--f1)">
									{basis(meanEdge)}
								</span>
								{liveAccuracy !== undefined && liveAccuracy > 0 && (
									<span className="text-[10px] text-(--info)">
										Acc: {percent(liveAccuracy)}
									</span>
								)}
							</div>
							<Typography.Mono size="s" tone="f4" className="mt-0.5 block text-[10px]">
								Completed evaluations against honest market quotes.
							</Typography.Mono>
						</div>

						{/* Outcome Signs */}
						<div>
							<Typography.Label size="s" tone="f4" weight="normal">
								OUTCOME SIGNS
							</Typography.Label>
							<div className="mt-0.5 flex items-baseline gap-2 text-[10px]">
								<span className="text-(--up)">{wins} positive</span>
								<span className="text-(--f4)">·</span>
								<span className="text-(--down)">{losses} negative</span>
							</div>
							<div className="mt-1">
								<RatioBar
									size="s"
									segments={[
										{ value: wins, tone: "up", label: "Positive" },
										{ value: losses, tone: "down", label: "Negative" },
									]}
								/>
							</div>
						</div>

						{/* Model Theoretical Portfolio P&L */}
						<div>
							<Typography.Label size="s" tone="f4" weight="normal">
								MODEL THEORETICAL EDGE
							</Typography.Label>
							<div
								className={`mt-0.5 text-base font-bold ${
									globalPnl >= 0 ? "text-(--up)" : "text-(--down)"
								}`}
							>
								{globalPnl >= 0 ? `+${globalPnl.toFixed(4)}` : globalPnl.toFixed(4)}{" "}
								bp
							</div>
						</div>

						{/* Recent Learning Activity Log */}
						<div className="flex flex-1 flex-col border-t border-(--line) pt-2 min-h-0 overflow-hidden">
							<Typography.Label size="s" tone="f4" weight="normal" className="mb-1.5 shrink-0">
								RECENT ACTIVITY
							</Typography.Label>
							<div className="flex flex-1 flex-col gap-1.5 overflow-hidden">
								{logs.slice(0, 4).map((entry) => (
									<div
										key={entry.id}
										className="flex items-center justify-between border-b border-(--line)/40 pb-1 text-[10px]"
									>
										<span className="truncate text-(--f3)">
											{entry.time} · {entry.action}
										</span>
										<span
											className={`shrink-0 font-bold ${
												entry.pnl >= 0 ? "text-(--up)" : "text-(--down)"
											}`}
										>
											{entry.pnl >= 0 ? `+${entry.pnl.toFixed(2)}` : entry.pnl.toFixed(2)}
										</span>
									</div>
								))}
							</div>
						</div>
					</div>
				</div>
			</div>

			{/* BOTTOM ROW: Trie Memory Active Branches + Edge Distribution (Screen) */}
			<div className="flex flex-1 min-h-0">
				{/* Radix Trie Memory Table */}
				<div className="flex flex-1 min-w-0 flex-col border-r border-(--line) bg-(--surface) overflow-hidden">
					<div className="flex h-7 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[10px] text-(--f4)">
						<span className="font-bold text-(--f2)">
							ACTIVE BRANCHES · RADIX TRIE MEMORY
						</span>
						<span>
							{liveDecisions
								? `${liveDecisions} situations`
								: `${triePaths.reduce((a, b) => a + b.visits, 0)} observations`}
						</span>
					</div>

					<div className="flex-1 overflow-hidden p-2">
						<table className="w-full text-left text-[10.5px]">
							<thead>
								<tr className="border-b border-(--line) text-(--f4)">
									<th className="font-normal pb-1 px-3">Path Signature</th>
									<th className="font-normal pb-1 px-3 text-right">Depth</th>
									<th className="font-normal pb-1 px-3 text-right">Visits</th>
									<th className="font-normal pb-1 px-3 text-right">Mean Edge</th>
									<th className="font-normal pb-1 px-3">Confidence</th>
									<th className="font-normal pb-1 px-3 text-right">Policy</th>
								</tr>
							</thead>
							<tbody>
								{triePaths.slice(0, 5).map((path, i) => (
									<tr
										key={path.hash}
										className="border-b border-(--line)/40 last:border-0 hover:bg-(--raised)"
									>
										<td
											className={`py-1 px-3 font-mono ${
												i === 0 ? "font-bold text-(--acc)" : "text-(--f2)"
											}`}
										>
											{path.hash}
										</td>
										<td className="py-1 px-3 text-right text-(--f3)">{path.depth}</td>
										<td className="py-1 px-3 text-right text-(--f2)">
											{path.visits.toLocaleString()}
										</td>
										<td
											className={`py-1 px-3 text-right font-bold ${
												path.meanEdge >= 0 ? "text-(--up)" : "text-(--down)"
											}`}
										>
											{path.meanEdge >= 0
												? `+${path.meanEdge.toFixed(2)}`
												: path.meanEdge.toFixed(2)}{" "}
											bp
										</td>
										<td className="py-1 px-3">
											<div className="flex items-center gap-2">
												<div className="h-1 w-14 overflow-hidden rounded-full bg-(--line)">
													<div
														className="h-full bg-(--info)"
														style={{ width: `${path.confidence}%` }}
													/>
												</div>
												<span className="text-(--f4)">
													{path.confidence.toFixed(0)}%
												</span>
											</div>
										</td>
										<td className="py-1 px-3 text-right">
											<Badge
												size="s"
												variant={
													path.policy === "ENTER"
														? "success"
														: path.policy === "RETREAT"
															? "error"
															: "info"
												}
												label={path.policy}
											/>
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				</div>

				{/* Edge Distribution - Rendered on dark screen surface bg-(--sunken) */}
				<div className="flex w-96 shrink-0 flex-col bg-(--sunken) overflow-hidden min-h-0">
					<div className="flex h-7 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[10px] text-(--f4)">
						<span className="font-bold text-(--f2)">EDGE DISTRIBUTION</span>
						<span>Authority-weighted</span>
					</div>

					<div className="relative flex-1 min-h-0 p-2">
						<DistributionCurve
							mean={meanEdge}
							sd={sd}
							min={-15}
							max={15}
							breakeven={0}
							tone="info"
							unit="bp"
						/>
					</div>

					<div className="border-t border-(--line) bg-(--surface) px-4 py-1 text-[9px] text-(--f4)">
						Continuous empirical fit against honest quote resolutions.
					</div>
				</div>
			</div>
		</div>
	);
};
