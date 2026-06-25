import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	Meter,
	MeterIndicator,
	MeterLabel,
	MeterTrack,
} from "#/components/ui/meter";
import { cn } from "@/lib/utils";

const STATUS_TONE: Record<string, string> = {
	waiting: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
	healthy: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
};

const STATUS_LABEL: Record<string, string> = {
	waiting: "Waiting",
	healthy: "Live",
};

const formatPercent = (value: number): string => `${Math.round(value)}%`;

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
			<span className="text-foreground text-sm tabular-nums">
				{valueLabel ?? formatPercent(value)}
			</span>
		</div>
		<MeterTrack>
			<MeterIndicator />
		</MeterTrack>
	</Meter>
);

const formatField = (value: unknown): string => {
	if (value === null || value === undefined) {
		return "—";
	}

	if (typeof value === "number") {
		return Number.isInteger(value) ? value.toLocaleString() : value.toFixed(4);
	}

	return String(value);
};

export const SignalPanel = ({ origin }: { origin: string }) => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const frame = useSelector(measurementsStore, (state) => {
		const reading = state.readings[origin]?.[focusSymbol];

		return (reading ?? null) as Record<string, unknown> | null;
	});
	const output = (frame?.output ?? {}) as Record<string, unknown>;
	const confidence = (output.confidence as number) ?? 0;
	const surprise = (output.surprise as number) ?? 0;
	const strength = (output.strength as number) ?? 0;
	const elapsed = (output.elapsed as number) ?? 0;
	const category = String(output.category ?? frame?.category ?? "");
	const status = frame === null ? "waiting" : "healthy";

	return (
		<div className="flex flex-col gap-4 p-5">
			<div className="flex items-start justify-between gap-3">
				<div className="flex min-w-0 flex-col gap-1">
					<h2 className="font-semibold text-sm">{origin}</h2>
					<p className="text-muted-foreground text-sm">
						{focusSymbol} · live measurement frame
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

			{frame === null ? (
				<p className="text-muted-foreground text-sm">
					No measurement frame received yet for{" "}
					<span className="font-mono">{origin}</span> on{" "}
					<span className="font-mono">{focusSymbol}</span>.
				</p>
			) : (
				<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
					<DiagnosticMeter
						label="Confidence"
						value={Math.min(100, Math.max(0, confidence * 100))}
					/>
					<DiagnosticMeter
						label="Surprise"
						value={Math.min(100, Math.max(0, surprise * 100))}
						valueLabel={surprise.toFixed(4)}
					/>
					<DiagnosticMeter
						label="Strength"
						value={Math.min(100, Math.max(0, strength * 100))}
						valueLabel={strength.toFixed(4)}
					/>
					<DiagnosticMeter
						label="Calibration"
						value={frame.calibrated === true ? 100 : 0}
						valueLabel={frame.calibrated === true ? "Ready" : "Pending"}
					/>
					<div className="md:col-span-2 grid grid-cols-2 gap-x-6 gap-y-2 border-border/60 border-t pt-2 text-sm">
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Scope</span>
							<span className="font-mono">
								{String(frame.scope ?? focusSymbol)}
							</span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Category</span>
							<span className="font-mono">{category || "—"}</span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Samples</span>
							<span className="font-mono">{formatField(frame.samples)}</span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-muted-foreground">Elapsed</span>
							<span className="font-mono">
								{elapsed > 0 ? `${elapsed.toFixed(1)}s` : "—"}
							</span>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};
