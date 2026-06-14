import { useSelector } from "@tanstack/react-store";
import {
	confidenceMeterValue,
	SIGNAL_SOURCES,
	type SignalHealthStatus,
	signalHealthStatus,
	signalStore,
	surpriseMeterValue,
} from "#/collections/signals";
import { cn } from "#/lib/utils";

type Rollup = {
	total: number;
	healthy: number;
	calibrating: number;
	degraded: number;
	thin: number;
	waiting: number;
	avgConfidence: number;
	firing: number;
	overall: SignalHealthStatus;
};

const DEGRADED: SignalHealthStatus[] = ["fault", "stale"];

const computeRollup = (): Rollup => {
	const readings = signalStore.state.readings;
	let healthy = 0;
	let calibrating = 0;
	let degraded = 0;
	let thin = 0;
	let waiting = 0;
	let firing = 0;
	let confidenceSum = 0;
	let confidenceCount = 0;

	for (const source of SIGNAL_SOURCES) {
		const reading = readings[source] ?? null;
		const status = signalHealthStatus(reading);

		if (status === "healthy") {
			healthy += 1;
		} else if (status === "calibrating") {
			calibrating += 1;
		} else if (status === "waiting") {
			waiting += 1;
		} else if (status === "flat") {
			thin += 1;
		} else if (DEGRADED.includes(status)) {
			degraded += 1;
		}

		if (reading !== null && status !== "waiting") {
			confidenceSum += confidenceMeterValue(reading);
			confidenceCount += 1;

			if (surpriseMeterValue(reading) >= 100) {
				firing += 1;
			}
		}
	}

	const total = SIGNAL_SOURCES.length;
	const avgConfidence =
		confidenceCount > 0 ? confidenceSum / confidenceCount : 0;

	let overall: SignalHealthStatus = "healthy";

	if (degraded > 0) {
		overall = "fault";
	} else if (healthy === 0 && calibrating === 0) {
		overall = "waiting";
	} else if (calibrating > healthy) {
		overall = "calibrating";
	} else if (healthy < total / 2) {
		overall = "flat";
	}

	return {
		total,
		healthy,
		calibrating,
		degraded,
		thin,
		waiting,
		avgConfidence,
		firing,
		overall,
	};
};

const OVERALL_TONE: Record<string, string> = {
	healthy: "border-emerald-500/50 bg-emerald-500/10 text-emerald-300",
	calibrating: "border-amber-500/50 bg-amber-500/10 text-amber-300",
	fault: "border-red-500/50 bg-red-500/10 text-red-300",
	stale: "border-red-500/50 bg-red-500/10 text-red-300",
	flat: "border-amber-500/50 bg-amber-500/10 text-amber-300",
	waiting: "border-zinc-500/40 bg-zinc-500/10 text-zinc-400",
};

const OVERALL_LABEL: Record<string, string> = {
	healthy: "Nominal",
	calibrating: "Warming up",
	fault: "Degraded",
	stale: "Degraded",
	flat: "Thin",
	waiting: "Standby",
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
	const rollup = useSelector(signalStore, computeRollup);

	return (
		<div className="col-span-2 flex min-h-0 flex-col gap-2 rounded-lg border border-border bg-card/50 p-3">
			<div className="flex items-center justify-between gap-2">
				<span className="text-xs font-semibold">System Health</span>
				<span
					className={cn(
						"rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide",
						OVERALL_TONE[rollup.overall],
					)}
				>
					{OVERALL_LABEL[rollup.overall]}
				</span>
			</div>

			<div className="flex items-end gap-4">
				<div className="flex flex-col">
					<span className="text-2xl font-semibold tabular-nums leading-none">
						{rollup.healthy}
						<span className="text-sm text-muted-foreground">
							/{rollup.total}
						</span>
					</span>
					<span className="text-[10px] text-muted-foreground">
						signals healthy
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
					<span
						className={cn(
							"text-2xl font-semibold tabular-nums leading-none",
							rollup.firing > 0 ? "text-sky-300" : "",
						)}
					>
						{rollup.firing}
					</span>
					<span className="text-[10px] text-muted-foreground">firing now</span>
				</div>
			</div>

			<div className="mt-auto flex flex-col gap-1">
				<StatBar
					label="Healthy"
					count={rollup.healthy}
					total={rollup.total}
					tone="bg-emerald-400"
				/>
				<StatBar
					label="Warming"
					count={rollup.calibrating}
					total={rollup.total}
					tone="bg-amber-400"
				/>
				<StatBar
					label="Degraded"
					count={rollup.degraded}
					total={rollup.total}
					tone="bg-red-400"
				/>
			</div>
		</div>
	);
};
