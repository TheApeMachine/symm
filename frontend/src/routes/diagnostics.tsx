import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { appStore } from "#/collections/app";
import {
	confidenceMeterValue,
	SIGNAL_LABELS,
	SIGNAL_SOURCES,
	signalHealthStatus,
	signalStore,
	surpriseMeterValue,
} from "#/collections/signals";
import { SignalPanel } from "#/components/diagnostics/signal-panel";
import { SignalSparkline } from "#/components/diagnostics/SignalSparkline";
import { SystemHealthCell } from "#/components/diagnostics/SystemHealthCell";
import {
	Frame,
	FrameDescription,
	FrameHeader,
	FrameTitle,
} from "#/components/ui/frame";
import { cn } from "#/lib/utils";

const STATUS_DOT: Record<string, string> = {
	waiting: "bg-zinc-500",
	calibrating: "bg-amber-400",
	fault: "bg-red-500",
	stale: "bg-red-500",
	flat: "bg-amber-400",
	healthy: "bg-emerald-400",
};

const SignalCell = ({
	source,
	selected,
	onSelect,
}: {
	source: string;
	selected: boolean;
	onSelect: (source: string) => void;
}) => {
	const reading = useSelector(
		signalStore,
		(state) => state.readings[source] ?? null,
	);
	const label = SIGNAL_LABELS[source] ?? source;
	const status = signalHealthStatus(reading);
	const confidence = reading === null ? 0 : confidenceMeterValue(reading);
	const surprise = reading === null ? 0 : surpriseMeterValue(reading);

	return (
		<button
			type="button"
			onClick={() => onSelect(source)}
			className={cn(
				"flex min-h-0 flex-col gap-1 rounded-lg border bg-card/40 p-2 text-left transition-colors",
				selected
					? "border-sky-400/70 bg-sky-400/10"
					: "border-border hover:border-border/80 hover:bg-card/60",
			)}
		>
			<div className="flex items-center justify-between gap-2">
				<span className="truncate text-xs font-semibold">{label}</span>
				<span
					className={cn(
						"size-2 shrink-0 rounded-full",
						STATUS_DOT[status] ?? "bg-zinc-500",
					)}
					title={status}
				/>
			</div>

			<div className="min-h-0 flex-1">
				<SignalSparkline source={source} />
			</div>

			<div className="flex items-center justify-between gap-2 text-[10px] tabular-nums text-muted-foreground">
				<span>conf {Math.round(confidence)}%</span>
				<span>surp {Math.round(surprise)}%</span>
			</div>
		</button>
	);
};

const DiagnosticsPage = () => {
	const online = useSelector(appStore, (state) => state.online);
	const [selected, setSelected] = useState(SIGNAL_SOURCES[0]);

	return (
		<div className="flex h-full min-h-0 w-full flex-col gap-3">
			<Frame className="w-full shrink-0">
				<FrameHeader className="py-3">
					<div className="flex items-center gap-2">
						<FrameTitle>Signal Insight</FrameTitle>
						{online ? (
							<span className="rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-emerald-400">
								Live
							</span>
						) : null}
					</div>
					<FrameDescription>
						Every signal at a glance — confidence history, surprise, and health.
						Select a cell for full diagnostics.
					</FrameDescription>
				</FrameHeader>
			</Frame>

			<div className="grid min-h-0 flex-1 grid-cols-[2fr_1fr] gap-3">
				<div className="grid min-h-0 grid-cols-4 grid-rows-4 gap-2">
					{SIGNAL_SOURCES.map((source) => (
						<SignalCell
							key={source}
							source={source}
							selected={source === selected}
							onSelect={setSelected}
						/>
					))}
					<SystemHealthCell />
				</div>

				<div className="min-h-0 overflow-auto rounded-xl border border-border bg-card/40">
					<SignalPanel source={selected} />
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsPage,
});
