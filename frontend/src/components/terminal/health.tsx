import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	getHealthStatus,
	kernelFrameForSource,
	kernelReadout,
	type ReadingsState,
} from "#/components/terminal/rows";

const Meter = ({
	label,
	value,
	percent,
	color = "var(--info)",
}: {
	label: string;
	value: string;
	percent: number;
	color?: string;
}) => (
	<div className="flex items-center gap-2">
		<span className="w-[58px] text-[10px] text-(--f4)">{label}</span>
		<div className="h-[5px] flex-1 overflow-hidden rounded-[3px] bg-(--line)">
			<div
				className="h-full"
				style={{ width: `${percent}%`, backgroundColor: color }}
			/>
		</div>
		<span className="w-[18px] text-right font-mono text-[10px] text-(--f2)">
			{value}
		</span>
	</div>
);

const Stat = ({
	value,
	label,
	accent = false,
}: {
	value: string;
	label: string;
	accent?: boolean;
}) => (
	<div>
		<div
			className="font-mono font-semibold text-2xl leading-none"
			style={{ color: accent ? "var(--acc)" : "var(--f1)" }}
		>
			{value}
		</div>
		<div className="mt-1 text-[9px] text-(--f4)">{label}</div>
	</div>
);

export type HealthSummary = {
	avg: number;
	bars: Array<{ color: string; count: number; label: string; percent: number }>;
	firing: number;
	healthy: number;
	label: string;
	tone: string;
	total: number;
};

export const terminalHealthSummary = (
	readings: ReadingsState,
	focusSymbol: string,
	origins = Object.keys(readings),
): HealthSummary => {
	const total = origins.length;
	let confidenceSum = 0;
	let healthy = 0;
	let warming = 0;
	let degraded = 0;
	let firing = 0;

	for (const origin of origins) {
		const frame = kernelFrameForSource(readings, origin, focusSymbol);
		const { confidence, surprise } = kernelReadout(frame);
		const status = getHealthStatus(origin, confidence, surprise);

		confidenceSum += confidence;

		if (surprise >= 1.4) {
			firing += 1;
		}

		if (status === "healthy") {
			healthy += 1;
		} else if (status === "stale" || status === "fault") {
			degraded += 1;
		} else {
			warming += 1;
		}
	}

	const avg = total > 0 ? Math.round((confidenceSum / total) * 100) : 0;
	const label =
		degraded > 0 ? "Degraded" : healthy < total / 2 ? "Thin" : "Nominal";
	const tone =
		degraded > 0
			? "var(--down)"
			: healthy < total / 2
				? "var(--warn)"
				: "var(--up)";
	const bars = [
		{ label: "Healthy", count: healthy, color: "var(--up)" },
		{ label: "Warming", count: warming, color: "var(--warn)" },
		{ label: "Degraded", count: degraded, color: "var(--down)" },
	].map((bar) => ({
		...bar,
		percent: total > 0 ? Math.round((bar.count / total) * 100) : 0,
	}));

	return { avg, bars, firing, healthy, label, tone, total };
};

export const HealthPanel = ({ origins }: { origins?: string[] }) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const health = terminalHealthSummary(readings, focusSymbol, origins);

	return (
		<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-[13px]">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-(--f1) text-xs">System health</span>
				<span
					className="rounded-full border px-[9px] py-0.5 font-semibold text-[10px] uppercase"
					style={{ borderColor: health.tone, color: health.tone }}
				>
					{health.label}
				</span>
			</div>
			<div className="mt-3 flex gap-[18px]">
				<Stat value={`${health.healthy}/${health.total}`} label="healthy" />
				<Stat value={`${health.avg}%`} label="avg conf" />
				<Stat value={String(health.firing)} label="firing" accent />
			</div>
			<div className="mt-[13px] flex flex-col gap-1.5">
				{health.bars.map((bar) => (
					<Meter
						key={bar.label}
						label={bar.label}
						value={String(bar.count)}
						percent={bar.percent}
						color={bar.color}
					/>
				))}
			</div>
		</div>
	);
};

const finiteRatio = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? Math.min(1, Math.max(0, number)) : 0;
};

export const regimeValuesFromFrames = (
	readings: ReadingsState,
	regimeFrame: Record<string, unknown> | null,
): [number, number, number, number, number] => {
	if (regimeFrame !== null) {
		return [
			finiteRatio(regimeFrame.volatility),
			finiteRatio(regimeFrame.trend),
			finiteRatio(regimeFrame.bullish),
			finiteRatio(regimeFrame.bearish),
			finiteRatio(regimeFrame.choppiness),
		];
	}

	const rows = Object.values(readings).flatMap((bySymbol) =>
		Object.values(bySymbol).map((frame) => {
			const readout = kernelReadout(frame);
			const category = String(readout.output.category ?? "").toLowerCase();

			return { ...readout, category };
		}),
	);

	if (rows.length === 0) {
		return [0, 0, 0, 0, 0];
	}

	const mean = (values: number[]) =>
		values.reduce((sum, value) => sum + value, 0) / values.length;
	const hasAny = (category: string, tokens: string[]) =>
		tokens.some((token) => category.includes(token));
	const volatility = mean(rows.map((row) => finiteRatio(row.surprise / 2.4)));
	const trend = mean(rows.map((row) => row.confidence));
	// ponytail: until a backend regime frame is routed, bullish/bearish/chop are
	// a display-only projection from category words. Upgrade path: publish role
	// "regime" with the five axes and remove this lexical fallback.
	const bullishRows = rows.filter((row) =>
		hasAny(row.category, [
			"alpha",
			"drive",
			"frenzy",
			"ignite",
			"surge",
			"trend",
		]),
	);
	const bearishRows = rows.filter((row) =>
		hasAny(row.category, ["collapse", "dump", "exhaust", "scarcity", "slump"]),
	);
	const chopRows = rows.filter((row) =>
		hasAny(row.category, [
			"balance",
			"decoupled",
			"drift",
			"flat",
			"median",
			"noise",
		]),
	);
	const strengthMean = (items: typeof rows) =>
		items.length === 0 ? 0 : mean(items.map((row) => row.confidence));

	return [
		volatility,
		trend,
		strengthMean(bullishRows),
		strengthMean(bearishRows),
		chopRows.length === 0 ? 1 - trend : strengthMean(chopRows),
	];
};

export const RadarPanel = () => {
	const readings = useSelector(measurementsStore, (state) => state);
	const regimeFrame = useSelector(appStore, (state) => state.lastRegimeFrame);
	const values = regimeValuesFromFrames(readings, regimeFrame);
	const units = [
		[0, -1],
		[0.951, -0.309],
		[0.588, 0.809],
		[-0.588, 0.809],
		[-0.951, -0.309],
	];
	const points = units
		.map(
			([x, y], index) =>
				`${110 + x * 84 * values[index]},${105 + y * 84 * values[index]}`,
		)
		.join(" ");

	return (
		<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-[13px]">
			<div className="mb-2 font-semibold text-(--f1) text-xs">Regime radar</div>
			<div className="mb-2 font-mono text-[9.5px] text-(--f4)">
				cross-section mean · market
			</div>
			<svg viewBox="0 0 220 210" className="block w-full">
				<title>Regime radar</title>
				<polygon
					points="110,21 190,79 159,173 61,173 30,79"
					fill="none"
					stroke="#3a342b"
				/>
				<polygon
					points="110,49 163,87 142,154 78,154 57,87"
					fill="none"
					stroke="#2b251e"
				/>
				<polygon
					points="110,77 137,94 126,134 94,134 83,94"
					fill="none"
					stroke="#2b251e"
				/>
				{units.map(([x, y]) => (
					<line
						key={`${x}:${y}`}
						x1="110"
						y1="105"
						x2={110 + x * 84}
						y2={105 + y * 84}
						stroke="#2b251e"
					/>
				))}
				<polygon
					points={points}
					fill="rgba(232,163,61,0.22)"
					stroke="#e8a33d"
					strokeWidth="1.6"
				/>
				{["volatility", "trend", "bullish", "bearish", "chop"].map(
					(label, index) => (
						<text
							key={label}
							x={110 + units[index][0] * 98}
							y={105 + units[index][1] * 98}
							textAnchor="middle"
							fontSize="9"
							fill="#938a7e"
						>
							{label}
						</text>
					),
				)}
			</svg>
		</div>
	);
};
