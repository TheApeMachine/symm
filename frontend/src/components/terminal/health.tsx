import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";

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
	bars: Array<{ color: string; count: number; label: string; percent: number }>;
	avg: number;
	firing: number;
	label: string;
	measured: number;
	tone: string;
	total: number;
};

export const terminalHealthSummary = (
	readings: typeof measurementsStore.state,
	focusSymbol: string,
	sources = [
		...new Set(
			Object.values(readings.measurements).flatMap((sourceMap) =>
				Object.keys(sourceMap),
			),
		),
	],
): HealthSummary => {
	const total = sources.length;
	let measured = 0;
	let warming = 0;
	let degraded = 0;
	let confidence = 0;
	let confidenceCount = 0;
	let firing = 0;

	for (const source of sources) {
		const history =
			focusSymbol === "stream"
				? Object.values(readings.measurements).flatMap(
						(sourceMap) => sourceMap[source]?.values() ?? [],
					)
				: (readings.measurements[focusSymbol]?.[source]?.values() ?? []);
		const frame = history.at(-1);
		const status =
			frame === undefined
				? "waiting"
				: frame.status === "fault" ||
						frame.status === "ambiguous" ||
						frame.status === "calibrating"
					? frame.status
					: "measured";
		const category = frame?.categories?.at(0);

		if (status === "ambiguous" || status === "fault") {
			degraded += 1;
		} else if (status === "waiting" || status === "calibrating") {
			warming += 1;
		} else {
			measured += 1;
		}

		if (typeof category?.confidence === "number") {
			confidence += category.confidence;
			confidenceCount += 1;
		}

		if (frame !== undefined) {
			firing += 1;
		}
	}

	const label =
		degraded > 0
			? "Degraded"
			: measured < total / 2
				? "Thin"
				: "Nominal";
	const tone =
		degraded > 0
			? "var(--down)"
			: measured < total / 2
				? "var(--warn)"
				: "var(--up)";
	const bars = [
		{ label: "Healthy", count: measured, color: "var(--up)" },
		{ label: "Warming", count: warming, color: "var(--warn)" },
		{ label: "Degraded", count: degraded, color: "var(--down)" },
	].map((bar) => ({
		...bar,
		percent: total > 0 ? Math.round((bar.count / total) * 100) : 0,
	}));

	return {
		avg: confidenceCount > 0 ? Math.round((confidence / confidenceCount) * 100) : 0,
		bars,
		firing,
		label,
		measured,
		tone,
		total,
	};
};

export const HealthPanel = ({ sources }: { sources?: string[] }) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const health = terminalHealthSummary(readings, focusSymbol, sources);

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
				<Stat value={`${health.measured}/${health.total}`} label="healthy" />
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
	readings: typeof measurementsStore.state,
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

	const rows = Object.values(readings.measurements).flatMap((sources) =>
		Object.values(sources).flatMap((history) => {
			const frame = history.values().at(-1);

			return (
				frame?.categories?.map((category) => ({
					source: frame.source,
					type: category.type,
					value: Math.max(category.confidence, category.strength),
				})) ?? []
			);
		}),
	);
	const axis = (matches: (row: (typeof rows)[number]) => boolean) => {
		const values = rows.flatMap((row) => (matches(row) ? [row.value] : []));

		return values.length === 0
			? 0
			: finiteRatio(values.reduce((sum, value) => sum + value, 0) / values.length);
	};
	const contains = (row: (typeof rows)[number], terms: string[]) =>
		terms.some((term) => row.type.includes(term) || row.source.includes(term));

	return [
		axis((row) =>
			contains(row, [
				"turbulent",
				"frenzy",
				"ignition",
				"vacuum",
				"shock",
				"collapse",
				"expansion",
				"reversal",
				"bluff",
				"spoof",
				"thinning",
				"scarcity",
			]),
		),
		axis((row) =>
			contains(row, [
				"trend",
				"drift",
				"drive",
				"laminar",
				"edge",
				"alpha",
				"beta",
				"surge",
			]),
		),
		axis((row) =>
			contains(row, [
				"edge",
				"ignition",
				"trend",
				"surge",
				"drive",
				"robust",
				"absorption",
				"support",
				"alpha",
			]),
		),
		axis((row) =>
			contains(row, [
				"slump",
				"vacuum",
				"collapse",
				"exhaustion",
				"starvation",
				"thinning",
				"bluff",
				"stress",
				"faded",
				"scarcity",
			]),
		),
		axis((row) =>
			contains(row, [
				"balance",
				"neutrality",
				"equilibrium",
				"noise",
				"decoupled",
				"stall",
				"median",
				"saturation",
			]),
		),
	];
};

export const RadarPanel = () => {
	const readings = useSelector(measurementsStore, (state) => state);
	const values = regimeValuesFromFrames(readings, null);
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
