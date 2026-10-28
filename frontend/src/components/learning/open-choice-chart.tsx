import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { positionReadout } from "#/components/book-readout";
import { cn } from "#/lib/utils";
import {
	CHOICE_WINDOW_MS,
	type ChoiceSample,
	choiceWindow,
	dropOpenChoices,
	extendToNow,
	meanPath,
	noteOpenChoice,
	openChoiceTrail,
	samplesInWindow,
} from "./open-choice-trail";

const STROKES = [
	"#7eb6ff",
	"#f0b429",
	"#5eead4",
	"#c4b5fd",
	"#e07a7a",
	"#86efac",
	"#f9a8d4",
];

const lotId = (row: Record<string, unknown>) => String(row.id ?? row.symbol);

/* Color follows the lot, so a neighbor arriving or leaving does not repaint the line. */
const strokeFor = (id: string) => {
	let hash = 0;
	for (let i = 0; i < id.length; i++) {
		hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
	}
	return STROKES[hash % STROKES.length];
};

const pctText = (value: number) => {
	const digits = Math.abs(value) >= 10 ? 0 : 1;
	const sign = value > 0 ? "+" : "";
	return `${sign}${value.toFixed(digits)}%`;
};

const lineOf = (
	samples: ChoiceSample[],
	xOf: (t: number) => number,
	yOf: (pnl: number) => number,
) =>
	samples
		.map((sample, index) => {
			const cmd = index === 0 ? "M" : "L";
			return `${cmd}${xOf(sample.t).toFixed(1)},${yOf(sample.pnl).toFixed(1)}`;
		})
		.join(" ");

export const OpenChoiceChart = ({
	lots,
}: {
	lots: Record<string, unknown>[];
}) => {
	const [now, setNow] = useState(() => Date.now());
	const [revision, setRevision] = useState(0);
	const plotRef = useRef<HTMLDivElement>(null);
	const [plotSize, setPlotSize] = useState({ w: 0, h: 168 });
	const [legendOpen, setLegendOpen] = useState(false);
	const clipId = useId().replace(/:/g, "");

	useEffect(() => {
		const live = new Set<string>();
		for (const row of lots) {
			const id = lotId(row);
			live.add(id);
			const pnl = positionReadout(row).returnPctDisplay;
			if (pnl !== null) noteOpenChoice(id, pnl);
		}
		dropOpenChoices(live);
		setRevision((value) => value + 1);
	}, [lots]);

	useEffect(() => {
		const timer = window.setInterval(() => setNow(Date.now()), 1000);
		return () => window.clearInterval(timer);
	}, []);

	const series = lots.map((row) => {
		const id = lotId(row);
		const readout = positionReadout(row);
		return {
			id,
			symbol: String(row.symbol ?? id),
			pnl: readout.returnPctDisplay,
			samples: openChoiceTrail(id),
			stroke: strokeFor(id),
		};
	});
	void revision;

	const drawn = series
		.map((item) => ({ ...item, samples: extendToNow(item.samples, now) }))
		.filter((item) => item.samples.length > 0);
	const mean = meanPath(
		series.map((item) => item.samples),
		now,
	);
	const watching = drawn.length > 0;

	useLayoutEffect(() => {
		const node = plotRef.current;
		if (!node || !watching) return;

		const sync = () => {
			const w = Math.max(280, Math.round(node.clientWidth));
			const h = Math.max(120, Math.round(node.clientHeight));
			setPlotSize((prev) => (prev.w === w && prev.h === h ? prev : { w, h }));
		};

		sync();
		if (typeof ResizeObserver === "undefined") return;
		const observer = new ResizeObserver(sync);
		observer.observe(node);
		return () => observer.disconnect();
	}, [watching]);

	const width = Math.max(plotSize.w, 280);
	const height = plotSize.h;
	const pad = { l: 52, r: 12, t: 16, b: 22 };
	const plotW = width - pad.l - pad.r;
	const plotH = height - pad.t - pad.b;
	const { from: t0, span } = choiceWindow(now, CHOICE_WINDOW_MS);
	const shown = drawn.map((item) => ({
		...item,
		samples: samplesInWindow(item.samples, t0),
	}));
	const meanShown = samplesInWindow(mean, t0);
	const values = [
		...shown.flatMap((item) => item.samples.map((sample) => sample.pnl)),
		...meanShown.map((sample) => sample.pnl),
	];
	const dataMin = values.length > 0 ? Math.min(...values) : 0;
	const dataMax = values.length > 0 ? Math.max(...values) : 0;
	const rawSpan = Math.max(dataMax, 0) - Math.min(dataMin, 0) || 1;
	const yMin = Math.min(dataMin, 0) - rawSpan * 0.16;
	const yMax = Math.max(dataMax, 0) + rawSpan * 0.12;
	const ySpan = yMax - yMin || 1;
	const xOf = (t: number) => pad.l + ((t - t0) / span) * plotW;
	const yOf = (pnl: number) => pad.t + (1 - (pnl - yMin) / ySpan) * plotH;
	const yZero = yOf(0);
	const latestMean = mean.length > 0 ? mean[mean.length - 1].pnl : null;
	const bookTone =
		latestMean === null
			? "flat"
			: latestMean >= 2
				? "ahead"
				: latestMean <= -2
					? "behind"
					: "flat";

	return (
		<section className="w-full max-w-none shrink-0 self-stretch border-b border-(--line) bg-(--surface) px-3 py-2">
			<div className="mb-1 flex items-baseline justify-between gap-3">
				<div>
					<div className="text-[11px] font-bold uppercase tracking-wider text-(--f2)">
						Open choices
					</div>
					<div className="text-[10px] text-(--f4)">
						Close-now return while the lot is still open. The dashed line is
						entry after fees.
					</div>
				</div>
				{latestMean !== null && (
					<div
						className="text-[12px] font-bold tabular-nums"
						style={{
							color:
								bookTone === "ahead"
									? "var(--up)"
									: bookTone === "behind"
										? "var(--down)"
										: "var(--warn)",
						}}
					>
						book {pctText(latestMean)}
					</div>
				)}
			</div>
			{watching ? (
				<div
					ref={plotRef}
					className={cn(
						"w-full min-w-0",
						legendOpen ? "h-44" : "h-[min(22rem,42vh)]",
					)}
				>
					<svg
						viewBox={`0 0 ${width} ${height}`}
						preserveAspectRatio="none"
						className="block h-full w-full"
						role="img"
						aria-label="Close-now return of open positions"
					>
						<title>Close-now return of open positions</title>
						<defs>
							<clipPath id={clipId}>
								<rect x={pad.l} y={0} width={width - pad.l} height={height} />
							</clipPath>
						</defs>
						<line
							x1={pad.l}
							x2={width - pad.r}
							y1={yZero}
							y2={yZero}
							stroke="var(--f4)"
							strokeDasharray="4 4"
						/>
						<text
							x={pad.l - 6}
							y={yZero}
							fill="var(--f4)"
							fontSize={9}
							textAnchor="end"
							dominantBaseline="middle"
						>
							0
						</text>
						{dataMax > 0.05 && yOf(0) - yOf(dataMax) > 14 && (
							<text
								x={pad.l - 6}
								y={yOf(dataMax)}
								fill="var(--f4)"
								fontSize={9}
								textAnchor="end"
								dominantBaseline="middle"
							>
								{pctText(dataMax)}
							</text>
						)}
						{dataMin < -0.05 && yOf(dataMin) - yOf(0) > 14 && (
							<text
								x={pad.l - 6}
								y={yOf(dataMin)}
								fill="var(--f4)"
								fontSize={9}
								textAnchor="end"
								dominantBaseline="middle"
							>
								{pctText(dataMin)}
							</text>
						)}
						<text x={pad.l} y={height - 4} fill="var(--f4)" fontSize={9}>
							{Math.round(CHOICE_WINDOW_MS / 60000)}m ago
						</text>
						<text
							x={width - pad.r}
							y={height - 4}
							fill="var(--f4)"
							fontSize={9}
							textAnchor="end"
						>
							now
						</text>
						<g clipPath={`url(#${clipId})`}>
							{shown.map((item) => {
								const last = item.samples[item.samples.length - 1];
								return (
									<g key={item.id}>
										<path
											d={lineOf(item.samples, xOf, yOf)}
											fill="none"
											stroke={item.stroke}
											strokeWidth={1.6}
										/>
										<circle
											cx={xOf(last.t)}
											cy={yOf(last.pnl)}
											r={3}
											fill={item.stroke}
										/>
									</g>
								);
							})}
							{meanShown.length > 1 && (
								<path
									d={lineOf(meanShown, xOf, yOf)}
									fill="none"
									stroke="var(--f1)"
									strokeWidth={2.2}
								/>
							)}
						</g>
					</svg>
				</div>
			) : (
				<div className="flex h-24 items-center justify-center text-[11px] text-(--f4)">
					close-now return is not on these lots yet
				</div>
			)}
			<button
				type="button"
				className="mt-1 flex w-full items-center justify-between gap-2 rounded border border-(--line) bg-(--sunken) px-2 py-0.5 text-left text-[10px] text-(--f3) transition-colors hover:border-(--acc)/40 hover:text-(--f2)"
				aria-expanded={legendOpen}
				onClick={() => setLegendOpen((open) => !open)}
			>
				<span>
					{legendOpen ? "Hide" : "Show"} symbol legend
					<span className="ml-1 tabular-nums text-(--f4)">
						({series.length})
					</span>
				</span>
				<span className="text-(--f4)" aria-hidden>
					{legendOpen ? "▾" : "▸"}
				</span>
			</button>
			{legendOpen ? (
				<div className="mt-1 max-h-40 overflow-auto flex flex-wrap gap-x-3 gap-y-1 border border-(--line) border-t-0 bg-(--sunken)/50 px-2 py-1.5">
					{series.map((item) => (
						<span
							key={item.id}
							className="inline-flex items-center gap-1 text-[10px] text-(--f3)"
						>
							<span
								className="inline-block h-1.5 w-3 rounded-sm"
								style={{ background: item.stroke }}
							/>
							{item.symbol}
							<span className="tabular-nums text-(--f2)">
								{item.pnl === null ? "…" : pctText(item.pnl)}
							</span>
						</span>
					))}
					{mean.length > 1 && (
						<span className="inline-flex items-center gap-1 text-[10px] text-(--f1)">
							<span className="inline-block h-0.5 w-3 bg-(--f1)" />
							book
						</span>
					)}
					<span className="text-[10px] text-(--f4)">entry after fees</span>
				</div>
			) : null}
		</section>
	);
};
