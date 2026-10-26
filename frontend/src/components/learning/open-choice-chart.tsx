import { useEffect, useState } from "react";
import { positionReadout } from "#/components/book-readout";
import {
	type ChoiceSample,
	dropOpenChoices,
	extendToNow,
	meanPath,
	noteOpenChoice,
	openChoiceTrail,
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

	const series = lots.map((row, index) => {
		const id = lotId(row);
		const readout = positionReadout(row);
		return {
			id,
			symbol: String(row.symbol ?? id),
			pnl: readout.returnPctDisplay,
			samples: openChoiceTrail(id),
			stroke: STROKES[index % STROKES.length],
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

	const width = 640;
	const height = 168;
	const pad = { l: 52, r: 12, t: 16, b: 22 };
	const plotW = width - pad.l - pad.r;
	const plotH = height - pad.t - pad.b;
	const times = drawn.flatMap((item) => item.samples.map((sample) => sample.t));
	const values = drawn.flatMap((item) =>
		item.samples.map((sample) => sample.pnl),
	);
	const t0 = times.length > 0 ? Math.min(...times) : now;
	const t1 = Math.max(
		now,
		...(times.length > 0 ? [Math.max(...times)] : [now]),
	);
	const span = Math.max(t1 - t0, 1);
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
		<section className="min-w-0 border-r border-b border-(--line) bg-(--surface) px-3 py-2">
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
				<svg
					viewBox={`0 0 ${width} ${height}`}
					className="h-40 w-full"
					role="img"
					aria-label="Close-now return of open positions"
				>
					<title>Close-now return of open positions</title>
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
						watched
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
					{drawn.map((item) => {
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
					{mean.length > 1 && (
						<path
							d={lineOf(mean, xOf, yOf)}
							fill="none"
							stroke="var(--f1)"
							strokeWidth={2.2}
						/>
					)}
				</svg>
			) : (
				<div className="flex h-24 items-center justify-center text-[11px] text-(--f4)">
					close-now return is not on these lots yet
				</div>
			)}
			<div className="mt-1 flex flex-wrap gap-x-3 gap-y-1">
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
		</section>
	);
};
