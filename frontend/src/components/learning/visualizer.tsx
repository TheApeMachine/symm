import { useMemo, useState } from "react";
import { Badge } from "#/components/ui/badge";
import { Canvas } from "#/components/ui/canvas";
import { Flex } from "#/components/ui/flex";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import { EventRhythm, LearningProgress, PipelineFunnel } from "./charts";
import { action, basis, clock } from "./format";
import type { Candidate, LearningEvent, LearningView, Skill } from "./state";

/* Recovered from 9ec57aadd: normal curve, shading, axes and mean marker.
The fit uses measured outcome moments, not uncertainty in the mean. */
export const EdgeDistributionPlot = ({ skill }: { skill?: Skill }) => {
	if (!skill?.defined) {
		return (
			<Typography.Mono className="p-4">
				No completed tape outcomes yet.
			</Typography.Mono>
		);
	}
	const defined = skill.defined;
	const meanBp = skill.mean * 10000;
	const dispersionBp = skill.varianceDefined
		? Math.sqrt(skill.variance) * 10000
		: undefined;
	// The recovered renderer shows 3.8 SD on either side of the mean.
	// This sets the drawing viewport, not a statistical decision boundary.
	const lowerBp =
		dispersionBp === undefined ? meanBp : meanBp - 3.8 * dispersionBp;
	const upperBp =
		dispersionBp === undefined ? meanBp : meanBp + 3.8 * dispersionBp;
	const extent = Math.max(Math.abs(lowerBp), Math.abs(upperBp));
	// One basis point is the display span only when all measured values are zero.
	const minBp = extent === 0 ? -1 : -extent;
	const maxBp = extent === 0 ? 1 : extent;
	const rangeBp = maxBp - minBp;

	const viewWidth = 520;
	const viewHeight = 180;
	const padding = { left: 45, right: 35, top: 25, bottom: 35 };
	const plotWidth = viewWidth - padding.left - padding.right;
	const plotHeight = viewHeight - padding.top - padding.bottom;

	const toSvgX = (valueBp: number) =>
		padding.left + ((valueBp - minBp) / rangeBp) * plotWidth;
	const zeroSvgX = toSvgX(0);
	const meanSvgX = toSvgX(meanBp);

	// Generate normal PDF curve points
	const steps = 80;
	const curvePoints: Array<{ x: number; y: number; bp: number }> = [];
	for (
		let index = 0;
		dispersionBp !== undefined && dispersionBp > 0 && index <= steps;
		index++
	) {
		const bp = minBp + (index / steps) * rangeBp;
		const exponent = -0.5 * ((bp - meanBp) / dispersionBp) ** 2;
		const density = Math.exp(exponent);
		const svgX = toSvgX(bp);
		const svgY = padding.top + plotHeight - density * (plotHeight * 0.88);
		curvePoints.push({ x: svgX, y: svgY, bp });
	}

	if (dispersionBp !== undefined && dispersionBp > 0) {
		curvePoints.push({
			x: meanSvgX,
			y: padding.top + plotHeight * (1 - 0.88),
			bp: meanBp,
		});
		curvePoints.sort((left, right) => left.bp - right.bp);
	}

	const fullCurveD = curvePoints.reduce(
		(accumulator, point, index) =>
			index === 0
				? `M ${point.x.toFixed(1)} ${point.y.toFixed(1)}`
				: `${accumulator} L ${point.x.toFixed(1)} ${point.y.toFixed(1)}`,
		"",
	);

	// Profit zone (bp > 0) shaded area
	const positivePoints = curvePoints.filter((point) => point.bp >= 0);
	let profitAreaD = "";
	if (positivePoints.length > 0) {
		const firstPoint = positivePoints[0];
		const lastPoint = positivePoints[positivePoints.length - 1];
		const baselineY = padding.top + plotHeight;
		profitAreaD = `M ${firstPoint.x.toFixed(1)} ${baselineY} L ${firstPoint.x.toFixed(1)} ${firstPoint.y.toFixed(1)}`;
		for (const point of positivePoints) {
			profitAreaD += ` L ${point.x.toFixed(1)} ${point.y.toFixed(1)}`;
		}
		profitAreaD += ` L ${lastPoint.x.toFixed(1)} ${baselineY} Z`;
	}

	const wins = skill.wins;
	const losses = skill.losses;
	const totalOutcomes = skill.samples;
	const zero = totalOutcomes - wins - losses;

	return (
		<Flex.Column className="h-full w-full justify-between gap-2 px-3">
			<div className="relative h-44 w-full overflow-hidden rounded bg-(--sunken) border border-(--line)">
				<svg
					viewBox={`0 0 ${viewWidth} ${viewHeight}`}
					className="h-full w-full select-none"
					preserveAspectRatio="none"
					role="img"
					aria-label="Normal fit to completed decision outcomes"
				>
					<title>Normal fit to completed decision outcomes</title>
					<defs>
						<linearGradient id="profitAreaGradient" x1="0" y1="0" x2="0" y2="1">
							<stop offset="0%" stopColor="var(--up)" stopOpacity="0.4" />
							<stop offset="100%" stopColor="var(--up)" stopOpacity="0.05" />
						</linearGradient>
					</defs>

					{/* Loss zone subtle background tint */}
					<rect
						x={padding.left}
						y={padding.top}
						width={Math.max(0, zeroSvgX - padding.left)}
						height={plotHeight}
						fill="var(--error)"
						opacity="0.04"
					/>

					{/* Profit zone subtle background tint */}
					<rect
						x={zeroSvgX}
						y={padding.top}
						width={Math.max(0, viewWidth - padding.right - zeroSvgX)}
						height={plotHeight}
						fill="var(--up)"
						opacity="0.05"
					/>

					{/* Baseline grid */}
					<line
						x1={padding.left}
						y1={padding.top + plotHeight}
						x2={viewWidth - padding.right}
						y2={padding.top + plotHeight}
						stroke="var(--line)"
						strokeWidth="1"
					/>

					{/* Positive portion of the fitted curve */}
					{profitAreaD && (
						<path d={profitAreaD} fill="url(#profitAreaGradient)" />
					)}

					{/* Normal PDF curve stroke */}
					{fullCurveD && (
						<path
							data-edge-curve="normal-fit"
							d={fullCurveD}
							fill="none"
							stroke="var(--info)"
							strokeWidth="2.2"
						/>
					)}

					{/* Breakeven zero line */}
					<line
						x1={zeroSvgX}
						y1={padding.top - 5}
						x2={zeroSvgX}
						y2={padding.top + plotHeight}
						stroke="var(--line2)"
						strokeWidth="1.5"
						strokeDasharray="3,3"
					/>
					<text
						x={zeroSvgX}
						y={padding.top - 8}
						textAnchor="middle"
						fill="var(--f3)"
						fontSize="9.5"
						fontFamily="monospace"
					>
						0.0 bp (Breakeven)
					</text>

					{/* Mean marker */}
					{defined && (
						<g>
							<line
								x1={meanSvgX}
								y1={padding.top + 10}
								x2={meanSvgX}
								y2={padding.top + plotHeight}
								stroke="var(--f2)"
								strokeWidth="1"
								strokeDasharray="2,2"
							/>
							<circle
								cx={meanSvgX}
								cy={padding.top + 10}
								r="3"
								fill="var(--f1)"
							/>
							<text
								x={meanSvgX}
								y={padding.top + 22}
								textAnchor="middle"
								fill="var(--f1)"
								fontSize="9"
								fontFamily="monospace"
							>
								μ {meanBp.toFixed(1)} bp
							</text>
						</g>
					)}

					{/* Axis labels */}
					<text
						x={padding.left}
						y={padding.top + plotHeight + 16}
						textAnchor="start"
						fill="var(--f4)"
						fontSize="9"
						fontFamily="monospace"
					>
						{minBp.toFixed(0)} bp
					</text>
					<text
						x={viewWidth - padding.right}
						y={padding.top + plotHeight + 16}
						textAnchor="end"
						fill="var(--f4)"
						fontSize="9"
						fontFamily="monospace"
					>
						+{maxBp.toFixed(0)} bp
					</text>
				</svg>
			</div>

			<Flex.Row className="justify-between gap-3 rounded px-3 py-1.5 border border-(--line) bg-(--sunken)">
				<Typography.Mono size="s" tone="f2">
					Mean {basis(skill.mean)}
				</Typography.Mono>
				<Typography.Mono size="s" tone="f3">
					{dispersionBp === undefined
						? "Spread not yet measured"
						: `Observed SD ${dispersionBp.toFixed(1)} bp`}
				</Typography.Mono>
			</Flex.Row>
			<Typography.Mono size="s" tone="f4">
				{dispersionBp === 0
					? "Zero observed spread; the outcomes have no bell-shaped density."
					: "Normal fit to the authority-weighted mean and variance of completed outcomes."}
			</Typography.Mono>
			<Flex.Column className="gap-1">
				<Flex.Row
					align="center"
					className="justify-between text-[10px] font-mono text-(--f4)"
				>
					<span>Outcomes ({totalOutcomes.toLocaleString()} total)</span>
					<span>
						<span className="text-(--up)">{wins} positive</span> ·{" "}
						<span className="text-(--error)">{losses} negative</span> · {zero}{" "}
						zero
					</span>
				</Flex.Row>
				<Flex.Row
					className="h-2 w-full overflow-hidden rounded-[3px] bg-(--line)"
					aria-label="Observed outcome signs"
				>
					<div
						className="h-full bg-(--up) transition-all duration-300"
						style={{ width: `${(100 * wins) / totalOutcomes}%` }}
					/>
					<div
						className="h-full bg-(--error) transition-all duration-300"
						style={{ width: `${(100 * losses) / totalOutcomes}%` }}
					/>
					<div
						className="h-full bg-(--f4) transition-all duration-300"
						style={{ width: `${(100 * zero) / totalOutcomes}%` }}
					/>
				</Flex.Row>
			</Flex.Column>
		</Flex.Column>
	);
};

/*
ActionSpectrumPlot displays horizontal divergence bars for feasible policy actions.
The chosen policy action is distinctly illuminated so an operator can instantly
see what action the agent selected and how its expected return compares against alternatives.
*/
export const ActionSpectrumPlot = ({
	candidates,
}: {
	candidates: Candidate[] | null | undefined;
}) => {
	const list = candidates ?? [];

	// Determine basis point range
	const values = list
		.filter((candidate) => candidate.prior.Defined)
		.map((candidate) => candidate.prior.Mean * 10000);
	const maxAbs = Math.max(...values.map(Math.abs), 5);
	const domain = Math.max(maxAbs * 1.25, 8);

	return (
		<Flex.Column className="h-full w-full justify-between gap-2 overflow-auto px-3">
			<Typography.Label size="s" tone="f4" weight="normal">
				Feasible actions at current impulse ({list.length} candidates)
			</Typography.Label>

			<Flex.Column className="gap-2">
				{list.map((candidate) => {
					const defined = candidate.prior.Defined;
					const meanBp = defined ? candidate.prior.Mean * 10000 : 0;
					const dispersionBp = candidate.prior.VarianceDefined
						? Math.sqrt(candidate.prior.Variance) * 10000
						: 0;
					const percentWidth = Math.min((Math.abs(meanBp) / domain) * 50, 48);
					const isPositive = meanBp >= 0;

					return (
						<div
							key={`${candidate.kind}-${candidate.power}-${candidate.reduce}`}
							className={`relative flex items-center justify-between rounded p-2 border transition-colors ${
								candidate.selected
									? "border-(--acc) bg-[color-mix(in_srgb,var(--acc)_12%,var(--surface))]"
									: "border-(--line) bg-(--sunken)"
							}`}
						>
							{/* Action label & selected badge */}
							<Flex.Row align="center" gap={3} className="w-32 shrink-0">
								<Typography.Mono
									tone={candidate.selected ? "accent" : "f1"}
									className="font-bold text-xs"
								>
									{action(candidate.kind, candidate.power, candidate.reduce)}
								</Typography.Mono>
								{candidate.selected && (
									<Badge label="choice" variant="success" size="xxs" />
								)}
							</Flex.Row>

							{/* Center diverging bar container */}
							<div className="relative mx-3 flex-1 h-5 flex items-center justify-center">
								{/* Center 0 line */}
								<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--line2) z-10" />

								{defined ? (
									<div
										className={`absolute top-1 bottom-1 rounded-xs transition-all duration-300 ${
											isPositive
												? "bg-(--up) left-1/2"
												: "bg-(--down) right-1/2"
										}`}
										style={{ width: `${percentWidth}%` }}
									/>
								) : (
									<span className="text-[10px] font-mono text-(--f4)">
										unexplored target
									</span>
								)}

								{/* Dispersion whisker */}
								{defined && dispersionBp > 0 && (
									<div
										className="absolute top-2 bottom-2 border-t border-b border-(--f3) opacity-50 pointer-events-none"
										style={{
											left: `${50 + ((meanBp - dispersionBp) / domain) * 50}%`,
											right: `${50 - ((meanBp + dispersionBp) / domain) * 50}%`,
										}}
									/>
								)}
							</div>

							{/* Return value & sample support */}
							<Flex.Row
								align="center"
								gap={4}
								className="w-32 shrink-0 justify-end font-mono text-xs"
							>
								<span
									className={
										defined
											? isPositive
												? "text-(--up)"
												: "text-(--down)"
											: "text-(--f4)"
									}
								>
									{defined ? `${basis(candidate.prior.Mean)}` : "—"}
								</span>
								<span className="text-[10px] text-(--f4)">
									{candidate.prior.Samples} obs
								</span>
							</Flex.Row>
						</div>
					);
				})}

				{list.length === 0 && (
					<Typography.Mono className="p-4 text-center text-(--f3)">
						No executable actions feasible at this market state.
					</Typography.Mono>
				)}
			</Flex.Column>

			<div className="flex justify-between border-(--line) border-t pt-1 font-mono text-[9px] text-(--f4)">
				<span>← Negative return expectation</span>
				<span>0.0 bp</span>
				<span>Positive return expectation →</span>
			</div>
		</Flex.Column>
	);
};

/* LearningTrajectoryPlot shows the consolidated wallet's actual valuations. */
export const LearningTrajectoryPlot = ({
	events,
	initialCapital,
}: {
	events: LearningEvent[];
	initialCapital?: string;
}) => {
	const marks = useMemo(() => {
		const policy = events
			.filter((event) => event.mode === "policy" && event.kind === "valued")
			.sort(
				(left, right) =>
					Date.parse(left.at) - Date.parse(right.at) || left.id - right.id,
			);
		return policy.filter(
			(event, index) => index === 0 || event.at !== policy[index - 1].at,
		);
	}, [events]);
	const capital = Number(initialCapital);
	const profitPoints =
		capital > 0 ? marks.map((event) => (event.profit / capital) * 10000) : [];

	if (profitPoints.length === 0) {
		return (
			<Typography.Mono className="p-3 text-(--f3)">
				Policy wallet trajectory unavailable: recorded valuations and starting
				capital are required.
			</Typography.Mono>
		);
	}

	const viewWidth = 520;
	const viewHeight = 160;
	const padding = { left: 45, right: 35, top: 20, bottom: 25 };
	const plotWidth = viewWidth - padding.left - padding.right;
	const plotHeight = viewHeight - padding.top - padding.bottom;

	const minVal = Math.min(...profitPoints, 0);
	const maxVal = Math.max(...profitPoints, 0);
	const rangeVal = maxVal - minVal || 1;

	const toSvgY = (value: number) =>
		padding.top + plotHeight - ((value - minVal) / rangeVal) * plotHeight;
	const zeroY = toSvgY(0);

	const pathPoints = profitPoints.map((value, index) => {
		const svgX =
			padding.left + (index / Math.max(profitPoints.length - 1, 1)) * plotWidth;
		const svgY = toSvgY(value);
		return { x: svgX, y: svgY, val: value };
	});

	const lineD = pathPoints.reduce(
		(accumulator, point, index) =>
			index === 0
				? `M ${point.x.toFixed(1)} ${point.y.toFixed(1)}`
				: `${accumulator} L ${point.x.toFixed(1)} ${point.y.toFixed(1)}`,
		"",
	);

	const areaD =
		pathPoints.length > 0
			? `${lineD} L ${pathPoints[pathPoints.length - 1].x.toFixed(1)} ${zeroY.toFixed(1)} L ${pathPoints[0].x.toFixed(1)} ${zeroY.toFixed(1)} Z`
			: "";

	const lastValue = profitPoints[profitPoints.length - 1] ?? 0;
	const trendingUp = lastValue >= 0;

	return (
		<Flex.Column className="h-full w-full justify-between gap-2 px-3">
			<Flex.Row align="center" className="justify-between text-xs font-mono">
				<span className="text-(--f3)">
					Consolidated wallet profit ({marks.length} valuations)
				</span>
				<span
					className={`font-bold ${trendingUp ? "text-(--up)" : "text-(--warn)"}`}
				>
					{lastValue >= 0 ? `+${lastValue.toFixed(1)}` : lastValue.toFixed(1)}{" "}
					bp net
				</span>
			</Flex.Row>

			<div className="relative h-40 w-full overflow-hidden rounded bg-(--sunken) border border-(--line)">
				<svg
					viewBox={`0 0 ${viewWidth} ${viewHeight}`}
					className="h-full w-full select-none"
					preserveAspectRatio="none"
					role="img"
					aria-label="Policy wallet recorded net profit"
				>
					<title>Policy wallet recorded net profit</title>
					<defs>
						<linearGradient id="trajectoryGradient" x1="0" y1="0" x2="0" y2="1">
							<stop
								offset="0%"
								stopColor={trendingUp ? "var(--up)" : "var(--warn)"}
								stopOpacity="0.35"
							/>
							<stop
								offset="100%"
								stopColor={trendingUp ? "var(--up)" : "var(--warn)"}
								stopOpacity="0.02"
							/>
						</linearGradient>
					</defs>

					{/* Breakeven reference line */}
					<line
						x1={padding.left}
						y1={zeroY}
						x2={viewWidth - padding.right}
						y2={zeroY}
						stroke="var(--line2)"
						strokeWidth="1"
						strokeDasharray="3,3"
					/>
					<text
						x={viewWidth - padding.right + 4}
						y={zeroY + 3}
						fill="var(--f4)"
						fontSize="8"
						fontFamily="monospace"
					>
						0 bp
					</text>

					{/* Area fill */}
					{areaD && <path d={areaD} fill="url(#trajectoryGradient)" />}

					{/* Line stroke */}
					{lineD && (
						<path
							d={lineD}
							fill="none"
							stroke={trendingUp ? "var(--up)" : "var(--warn)"}
							strokeWidth="2"
						/>
					)}

					{/* Last point dot */}
					{pathPoints.length > 0 && (
						<circle
							cx={pathPoints[pathPoints.length - 1].x}
							cy={pathPoints[pathPoints.length - 1].y}
							r="4"
							fill={trendingUp ? "var(--up)" : "var(--warn)"}
						/>
					)}
				</svg>
			</div>

			<div className="flex justify-between font-mono text-[9px] text-(--f4)">
				<span>{clock(marks[0].at)}</span>
				<span>{clock(marks[marks.length - 1].at)}</span>
			</div>
		</Flex.Column>
	);
};

type VisualInsightMode =
	| "learning"
	| "edge"
	| "actions"
	| "trajectory"
	| "pipeline"
	| "rhythm";

const MODES: Array<{ key: VisualInsightMode; label: string }> = [
	{ key: "learning", label: "Learning" },
	{ key: "edge", label: "Edge" },
	{ key: "actions", label: "Actions" },
	{ key: "trajectory", label: "Trajectory" },
	{ key: "pipeline", label: "Where it stops" },
	{ key: "rhythm", label: "Rhythm" },
];

/*
LearningVisualizer hosts the rich visual diagnostics suite next to the impulse map,
empowering visual operators to intuitively grasp agent learning momentum and action reasoning.
*/
export const LearningVisualizer = ({
	view,
	events,
	className,
}: {
	view: LearningView | null;
	events: LearningEvent[];
	className?: string;
}) => {
	/*
		Learning opens first. The edge is the headline measurement, but it stays
		at zero for a long time while the agent is still earning one — so a
		surface that opens on it shows nothing moving and reads as broken, while
		what is actually happening has no place on the screen at all.
	*/
	const [mode, setMode] = useState<VisualInsightMode>("learning");
	const skill = view?.skill;

	const meanBp = skill?.defined ? (skill?.mean ?? 0) * 10000 : 0;
	const selectedCandidate = view?.candidates?.find((item) => item.selected);

	return (
		<Canvas
			title="Learning"
			className={`h-full w-full min-h-80 ${className ?? ""}`}
			topRight={
				<Tabs size="xs" className="pointer-events-auto relative z-10">
					{MODES.map((entry) => (
						<Tabs.Tab
							key={entry.key}
							size="xs"
							active={mode === entry.key}
							onClick={() => setMode(entry.key)}
						>
							{entry.label}
						</Tabs.Tab>
					))}
				</Tabs>
			}
			footer={
				<Flex.Row gap={4} align="center">
					<span>
						Edge:{" "}
						<strong
							className={
								skill?.defined && skill.mean > 0 ? "text-(--up)" : "text-(--f2)"
							}
						>
							{skill?.defined ? `${meanBp.toFixed(1)} bp` : "unmeasured"}
						</strong>
					</span>
					<span>
						Policy choice:{" "}
						<strong className="text-(--acc)">
							{selectedCandidate
								? action(
										selectedCandidate.kind,
										selectedCandidate.power,
										selectedCandidate.reduce,
									)
								: "wait"}
						</strong>
					</span>
					<span>
						Learned from:{" "}
						<strong className="text-(--acc)">
							{(view?.forward?.trained ?? 0).toLocaleString()} moves
						</strong>
					</span>
				</Flex.Row>
			}
		>
			<div className="h-full w-full pt-9 pb-7">
				{mode === "learning" && <LearningProgress view={view} />}
				{mode === "edge" && <EdgeDistributionPlot skill={skill} />}
				{mode === "actions" && (
					<ActionSpectrumPlot candidates={view?.candidates} />
				)}
				{mode === "trajectory" && (
					<LearningTrajectoryPlot
						events={events}
						initialCapital={view?.initialCapital}
					/>
				)}
				{mode === "pipeline" && (
					<div className="h-full w-full overflow-auto">
						<PipelineFunnel view={view} />
					</div>
				)}
				{mode === "rhythm" && (
					<div className="h-full w-full overflow-auto">
						<EventRhythm events={events} />
					</div>
				)}
			</div>
		</Canvas>
	);
};
