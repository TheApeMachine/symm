import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { toneClasses } from "#/components/terminal/tone";
import { cn } from "#/lib/utils";

const Meter = ({
	label,
	value,
	percent,
	color = "bg-cyan-300",
}: {
	label: string;
	value: string;
	percent: number;
	color?: string;
}) => (
	<div>
		<div className="mb-1 flex justify-between font-mono text-[10px]">
			<span className="text-(--f4)">{label}</span>
			<span className="text-(--f2)">{value}</span>
		</div>
		<div className="h-1.5 overflow-hidden rounded-sm bg-(--line)">
			<div className={cn("h-full", color)} style={{ width: `${percent}%` }} />
		</div>
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
			className={cn(
				"font-mono text-2xl leading-none",
				accent ? "text-(--acc)" : "text-(--f1)",
			)}
		>
			{value}
		</div>
		<div className="mt-1 text-[9px] text-(--f4)">{label}</div>
	</div>
);

export const HealthPanel = () => {
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const total = Object.keys(readings).length;

	return (
		<div className="rounded-[3px] border border-(--line) bg-(--sunken) p-3">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-(--f1) text-xs">System health</span>
				<span
					className={cn(
						"rounded-full border px-2 py-0.5 font-semibold text-[10px] uppercase",
						toneClasses(total > 0 ? "good" : "bad"),
					)}
				>
					{total > 0 ? "Nominal" : "Waiting"}
				</span>
			</div>
			<div className="mt-3 grid grid-cols-3 gap-3">
				<Stat value={`${total}/${total || 1}`} label="origins" />
				<Stat value="—" label="avg conf" />
				<Stat value="0" label="firing" accent />
			</div>
			<div className="mt-3 space-y-2">
				<Meter
					label="Origins"
					value={total.toString()}
					percent={total > 0 ? 100 : 0}
					color="bg-(--up)"
				/>
			</div>
		</div>
	);
};

export const RadarPanel = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const regimeFrame = useSelector(
		measurementsStore,
		(state) => state.readings.regime?.[focusSymbol] ?? null,
	);
	const values = [
		regimeFrame?.volatility,
		regimeFrame?.trend,
		regimeFrame?.bullish,
		regimeFrame?.bearish,
		regimeFrame?.choppiness,
	].map((value) =>
		typeof value === "number" ? Math.min(1, Math.max(0, value)) : 0,
	);
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
		<div className="rounded-[3px] border border-(--line) bg-(--sunken) p-3">
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
