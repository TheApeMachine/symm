import { useSelector } from "@tanstack/react-store";
import {
	confidenceMeterValue,
	evidenceMeterValue,
	freshnessMeterValue,
	healthMeterValue,
	SIGNAL_LABELS,
	signalHealthStatus,
	signalStore,
	surpriseMeterValue,
	warmupProgress,
} from "#/collections/signals";
import {
	Meter,
	MeterIndicator,
	MeterLabel,
	MeterTrack,
	MeterValue,
} from "#/components/ui/meter";
import { cn } from "@/lib/utils";

const STATUS_TONE: Record<string, string> = {
	waiting: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
	calibrating: "bg-amber-500/15 text-amber-400 border-amber-500/30",
	fault: "bg-red-500/15 text-red-400 border-red-500/30",
	stale: "bg-red-500/15 text-red-400 border-red-500/30",
	flat: "bg-amber-500/15 text-amber-400 border-amber-500/30",
	healthy: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
};

const STATUS_LABEL: Record<string, string> = {
	waiting: "Waiting",
	calibrating: "Calibrating",
	fault: "Fault",
	stale: "Stale",
	flat: "Flat",
	healthy: "Healthy",
};

const formatPercent = (value: number): string => `${Math.round(value)}%`;

const formatRatio = (value: number, threshold: number): string => {
	if (threshold <= 0) {
		return "-";
	}

	return `${value.toFixed(2)}× threshold`;
};

const formatObservedAge = (
	observedAt: number | null,
	updatedAt: number,
): string => {
	if (observedAt === null) {
		return "missing";
	}

	const elapsed = Math.max(0, updatedAt - observedAt);

	if (elapsed < 1000) {
		return `${Math.round(elapsed)}ms`;
	}

	return `${(elapsed / 1000).toFixed(1)}s`;
};

const formatElapsed = (elapsed: number): string => {
	if (elapsed <= 0) {
		return "missing";
	}

	return `${elapsed.toFixed(1)}s`;
};

const formatActiveReadings = (
	activeReadings: number,
	readingsCapacity: number,
	category: string,
): string => {
	if (readingsCapacity > 0) {
		return `${activeReadings.toLocaleString()} / ${readingsCapacity.toLocaleString()}`;
	}

	if (category !== "") {
		return category;
	}

	return "none";
};

const DiagnosticMeter = ({
	label,
	value,
	valueLabel,
}: {
	label: string;
	value: number;
	valueLabel?: string;
}) => (
	<Meter value={value}>
		<div className="flex items-center justify-between gap-2">
			<MeterLabel>{label}</MeterLabel>
			<MeterValue>{valueLabel ?? formatPercent(value)}</MeterValue>
		</div>
		<MeterTrack>
			<MeterIndicator />
		</MeterTrack>
	</Meter>
);

export const SignalPanel = ({ source }: { source: string }) => {
	const reading = useSelector(
		signalStore,
		(state) => state.readings[source] ?? null,
	);
	const label = SIGNAL_LABELS[source] ?? source;
	const status = signalHealthStatus(reading);
	const evidenceValue = reading === null ? 0 : evidenceMeterValue(reading);
	const freshnessValue = reading === null ? 0 : freshnessMeterValue(reading);
	const gapLabel =
		reading === null
			? ""
			: reading.gapReason || (reading.bestEffort ? "best_effort" : "none");

	return (
		<div className="flex flex-col gap-4 p-5">
			<div className="flex items-start justify-between gap-3">
				<div className="flex min-w-0 flex-col gap-1">
					<h2 className="font-semibold text-sm">{label}</h2>
					<p className="text-muted-foreground text-sm">
						Live confidence, surprise, strength, and publishable evidence.
					</p>
				</div>
				<span
					className={cn(
						"shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide",
						STATUS_TONE[status],
					)}
				>
					{STATUS_LABEL[status]}
				</span>
			</div>

			{reading === null ? (
				<p className="text-muted-foreground text-sm">
					No gauge frames received yet for{" "}
					<span className="font-mono">{source}</span>.
				</p>
			) : (
				<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
					<DiagnosticMeter label="Health" value={healthMeterValue(reading)} />
					<DiagnosticMeter
						label="Confidence"
						value={confidenceMeterValue(reading)}
					/>
					<DiagnosticMeter
						label="Surprise"
						value={surpriseMeterValue(reading)}
						valueLabel={formatRatio(
							reading.surprise,
							reading.surpriseThreshold,
						)}
					/>
					<DiagnosticMeter
						label="Evidence"
						value={evidenceValue}
						valueLabel={evidenceValue > 0 ? "Ready" : "Missing"}
					/>
					<DiagnosticMeter
						label="Freshness"
						value={freshnessValue}
						valueLabel={freshnessValue > 0 ? "Live" : "Stale"}
					/>
					{reading.calibrating ? (
						<DiagnosticMeter
							label="Warmup"
							value={warmupProgress(reading)}
							valueLabel={`${reading.samples.toLocaleString()} / ${reading.minSamples.toLocaleString()}`}
						/>
					) : (
						<DiagnosticMeter
							label="Calibration"
							value={reading.calibrated ? 100 : 0}
							valueLabel={reading.calibrated ? "Ready" : "Pending"}
						/>
					)}
					<div className="md:col-span-2 grid grid-cols-2 gap-x-6 gap-y-2 border-border/60 border-t pt-2 text-sm">
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Active</span>
							<span className="font-mono">
								{formatActiveReadings(
									reading.activeReadings,
									reading.readingsCapacity,
									reading.category,
								)}
							</span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Strength</span>
							<span className="font-mono">
								{reading.strength.toFixed(4)}
							</span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Observed</span>
							<span className="font-mono">
								{formatObservedAge(reading.observedAt, reading.updatedAt)} /{" "}
								{formatElapsed(reading.elapsed)}
							</span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Gap</span>
							<span className="max-w-48 truncate font-mono">
								{gapLabel}
							</span>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};
