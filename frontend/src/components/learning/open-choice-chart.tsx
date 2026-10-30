import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { closeNowReturnPct, positionReadout } from "#/components/book-readout";
import { cn } from "#/lib/utils";
import {
	CHOICE_SAMPLE_MS,
	indexMean,
	linePath,
	openChoiceTrail,
	openChoiceValues,
	syncOpenChoiceLots,
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

const strokeFor = (id: string) => {
	let hash = 0;
	for (let i = 0; i < id.length; i++) {
		hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
	}
	return STROKES[hash % STROKES.length];
};

const trailPayload = (rows: Record<string, unknown>[]) =>
	rows.map((row) => ({ id: lotId(row), pnl: closeNowReturnPct(row) }));

const pctText = (value: number) => {
	const digits = Math.abs(value) >= 10 ? 0 : 1;
	const sign = value > 0 ? "+" : "";
	return `${sign}${value.toFixed(digits)}%`;
};

/*
Same index → x mapping as ui/Trail: oldest sample on the left, newest on the
right. Shorter lots are right-aligned on the shared slot axis (indexMean).
*/
const xAtSlot = (
	slot: number,
	maxLen: number,
	plotLeft: number,
	plotWidth: number,
) => {
	const slots = Math.max(maxLen - 1, 1);
	return plotLeft + (slot / slots) * plotWidth;
};

export const OpenChoiceChart = ({
	lots,
}: {
	lots: Record<string, unknown>[];
}) => {
	const [revision, setRevision] = useState(0);
	const plotRef = useRef<HTMLDivElement>(null);
	const lotsRef = useRef(lots);
	const [plotSize, setPlotSize] = useState({ w: 0, h: 168 });
	const [legendOpen, setLegendOpen] = useState(false);
	const clipId = useId().replace(/:/g, "");
	lotsRef.current = lots;

	useLayoutEffect(() => {
		if (syncOpenChoiceLots(trailPayload(lots))) {
			setRevision((value) => value + 1);
		}
	}, [lots]);

	useEffect(() => {
		const sample = () => {
			if (syncOpenChoiceLots(trailPayload(lotsRef.current))) {
				setRevision((value) => value + 1);
			}
		};
		sample();
		const timer = window.setInterval(sample, CHOICE_SAMPLE_MS);
		return () => window.clearInterval(timer);
	}, []);

	const series = lots.map((row) => {
		const id = lotId(row);
		const readout = positionReadout(row);
		return {
			id,
			symbol: String(row.symbol ?? id),
			pnl: readout.returnPctDisplay,
			values: openChoiceValues(id),
			stroke: strokeFor(id),
		};
	});
	void revision;

	const drawn = series.filter((item) => item.values.length > 0);
	const watching = drawn.length > 0;
	const trailSeries = drawn.map((item) => openChoiceTrail(item.id));
	const maxLen = trailSeries.reduce(
		(n, samples) => Math.max(n, samples.length),
		0,
	);
	const meanSeries = indexMean(trailSeries);

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

	const finiteValues = [
		...drawn.flatMap((item) => item.values.filter(Number.isFinite)),
		...meanSeries.filter(Number.isFinite),
	];
	const baseline = 0;
	const dataMin =
		finiteValues.length > 0
			? Math.min(baseline, ...finiteValues)
			: baseline;
	const dataMax =
		finiteValues.length > 0
			? Math.max(baseline, ...finiteValues)
			: baseline;
	const ySpan = dataMax - dataMin || 1;
	const yOf = (pnl: number) =>
		pad.t + 2 + (1 - (pnl - dataMin) / ySpan) * (plotH - 4);
	const yZero = yOf(0);

	const latestMean =
		[...meanSeries].reverse().find((value) => Number.isFinite(value)) ?? null;
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
						Same close-now trail as each position card. Dashed line is zero.
						Heavy line is the equal-weight book average.
					</div>
				</div>
				<div
					className="min-w-22 text-right text-[12px] font-bold tabular-nums"
					style={{
						color:
							latestMean === null
								? "var(--f4)"
								: bookTone === "ahead"
									? "var(--up)"
									: bookTone === "behind"
										? "var(--down)"
										: "var(--warn)",
					}}
				>
					{latestMean === null ? "book …" : `book ${pctText(latestMean)}`}
				</div>
			</div>
			<div
				ref={plotRef}
				className={cn(
					"w-full min-w-0",
					legendOpen ? "h-44" : "h-[min(22rem,42vh)]",
				)}
			>
				{watching && maxLen > 0 ? (
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
							older
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
							{drawn.map((item) => {
								const start = maxLen - item.values.length;
								const lastIdx = item.values.length - 1;
								const lastVal = item.values[lastIdx];
								return (
									<g key={item.id}>
										<path
											d={linePath(
												item.values.map((pnl, i) => ({
													x: xAtSlot(start + i, maxLen, pad.l, plotW),
													y: yOf(pnl),
												})),
											)}
											fill="none"
											stroke={item.stroke}
											strokeWidth={1.4}
										/>
										<circle
											cx={xAtSlot(start + lastIdx, maxLen, pad.l, plotW)}
											cy={yOf(lastVal)}
											r={3}
											fill={item.stroke}
										/>
									</g>
								);
							})}
							{meanSeries.filter((pnl) => Number.isFinite(pnl)).length > 1 && (
								<path
									d={linePath(
										meanSeries.flatMap((pnl, i) =>
											Number.isFinite(pnl)
												? [
														{
															x: xAtSlot(i, maxLen, pad.l, plotW),
															y: yOf(pnl),
														},
													]
												: [],
										),
									)}
									fill="none"
									stroke="var(--f1)"
									strokeWidth={2.2}
								/>
							)}
						</g>
					</svg>
				) : (
					<div className="relative flex h-full w-full items-center justify-center rounded border border-dashed border-(--line) bg-(--sunken) text-[11px] text-(--f4)">
						close-now return is not on these lots yet
					</div>
				)}
			</div>
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
					{meanSeries.filter((pnl) => Number.isFinite(pnl)).length > 1 && (
						<span className="inline-flex items-center gap-1 text-[10px] text-(--f1)">
							<span className="inline-block h-0.5 w-3 bg-(--f1)" />
							book
						</span>
					)}
				</div>
			) : null}
		</section>
	);
};
