import { useSelector } from "@tanstack/react-store";
import { measurementsStore, type MeasurementFrame } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { cn } from "#/lib/utils";

type Rollup = {
	total: number;
	live: number;
	waiting: number;
	avgConfidence: number;
};

const computeRollup = (
	readings: Record<string, Record<string, MeasurementFrame>>,
	focusSymbol: string,
): Rollup => {
	const origins = Object.keys(readings).sort();
	let live = 0;
	let confidenceSum = 0;

	for (const origin of origins) {
		const frame = readings[origin]?.[focusSymbol] ?? null;

		if (frame === null) {
			continue;
		}

		live += 1;
		const output = (frame.output ?? {}) as Record<string, unknown>;
		const confidence = (output.confidence as number) ?? 0;
		confidenceSum += confidence;
	}

	const total = origins.length;
	const waiting = total - live;
	const avgConfidence = live > 0 ? (confidenceSum / live) * 100 : 0;

	return {
		total,
		live,
		waiting,
		avgConfidence,
	};
};

const OVERALL_TONE: Record<string, string> = {
	nominal: "border-emerald-500/50 bg-emerald-500/10 text-emerald-300",
	waiting: "border-zinc-500/40 bg-zinc-500/10 text-zinc-400",
};

const StatBar = ({
	label,
	count,
	total,
	tone,
}: {
	label: string;
	count: number;
	total: number;
	tone: string;
}) => {
	const pct = total > 0 ? (count / total) * 100 : 0;

	return (
		<div className="flex items-center gap-2">
			<span className="w-16 shrink-0 text-[10px] text-muted-foreground">
				{label}
			</span>
			<div className="h-1.5 flex-1 overflow-hidden rounded-full bg-input">
				<div
					className={cn("h-full rounded-full transition-all", tone)}
					style={{ width: `${pct}%` }}
				/>
			</div>
			<span className="w-6 shrink-0 text-right text-[10px] tabular-nums text-foreground">
				{count}
			</span>
		</div>
	);
};

export const SystemHealthCell = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const rollup = useSelector(measurementsStore, (state) =>
		computeRollup(state.readings, focusSymbol),
	);
	const overall =
		rollup.total === 0 || rollup.live === 0 ? "waiting" : "nominal";

	return (
		<div className="col-span-2 flex min-h-0 flex-col gap-2 rounded-lg border border-border bg-card/50 p-3">
			<div className="flex items-center justify-between gap-2">
				<span className="text-xs font-semibold">System Health</span>
				<span
					className={cn(
						"rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide",
						OVERALL_TONE[overall],
					)}
				>
					{overall === "nominal" ? "Nominal" : "Waiting"}
				</span>
			</div>

			<div className="flex items-end gap-4">
				<div className="flex flex-col">
					<span className="text-2xl font-semibold tabular-nums leading-none">
						{rollup.live}
						<span className="text-sm text-muted-foreground">
							/{rollup.total || 1}
						</span>
					</span>
					<span className="text-[10px] text-muted-foreground">
						origins live
					</span>
				</div>
				<div className="flex flex-col">
					<span className="text-2xl font-semibold tabular-nums leading-none">
						{Math.round(rollup.avgConfidence)}%
					</span>
					<span className="text-[10px] text-muted-foreground">
						avg confidence
					</span>
				</div>
				<div className="flex flex-col">
					<span className="text-2xl font-semibold tabular-nums leading-none">
						{focusSymbol}
					</span>
					<span className="text-[10px] text-muted-foreground">focus scope</span>
				</div>
			</div>

			<div className="mt-auto flex flex-col gap-1">
				<StatBar
					label="Live"
					count={rollup.live}
					total={rollup.total || 1}
					tone="bg-emerald-400"
				/>
				<StatBar
					label="Waiting"
					count={rollup.waiting}
					total={rollup.total || 1}
					tone="bg-zinc-400"
				/>
			</div>
		</div>
	);
};
