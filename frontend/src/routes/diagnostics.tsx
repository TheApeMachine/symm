import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { SignalSparkline } from "#/components/diagnostics/SignalSparkline";
import { SystemHealthCell } from "#/components/diagnostics/SystemHealthCell";
import { SignalPanel } from "#/components/diagnostics/signal-panel";
import {
	Frame,
	FrameDescription,
	FrameHeader,
	FrameTitle,
} from "#/components/ui/frame";
import { cn } from "#/lib/utils";

const STATUS_DOT: Record<string, string> = {
	waiting: "bg-zinc-500",
	healthy: "bg-emerald-400",
};

const SignalCell = ({
	origin,
	selected,
	onSelect,
}: {
	origin: string;
	selected: boolean;
	onSelect: (origin: string) => void;
}) => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const frame = useSelector(
		measurementsStore,
		(state) => state.readings[origin]?.[focusSymbol] ?? null,
	);
	const output = (frame?.output ?? {}) as Record<string, unknown>;
	const confidence = (output.confidence as number) ?? 0;
	const surprise = (output.surprise as number) ?? 0;
	const status = frame === null ? "waiting" : "healthy";

	return (
		<button
			type="button"
			onClick={() => onSelect(origin)}
			className={cn(
				"flex min-h-0 flex-col gap-1 rounded-lg border bg-card/40 p-2 text-left transition-colors",
				selected
					? "border-sky-400/70 bg-sky-400/10"
					: "border-border hover:border-border/80 hover:bg-card/60",
			)}
		>
			<div className="flex items-center justify-between gap-2">
				<span className="truncate text-xs font-semibold">{origin}</span>
				<span
					className={cn(
						"size-2 shrink-0 rounded-full",
						STATUS_DOT[status] ?? "bg-zinc-500",
					)}
					title={status}
				/>
			</div>

			<div className="min-h-0 flex-1">
				<SignalSparkline origin={origin} />
			</div>

			<div className="flex items-center justify-between gap-2 text-[10px] tabular-nums text-muted-foreground">
				<span>conf {Math.round(confidence * 100)}%</span>
				<span>surp {surprise.toFixed(2)}</span>
			</div>
		</button>
	);
};

const DiagnosticsPage = () => {
	const online = useSelector(appStore, (state) => state.online);
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const { selectSource } = terminalStore.actions;
	const origins = Object.keys(readings).sort();
	const selected = origins.includes(selectedSource)
		? selectedSource
		: (origins[0] ?? "");

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
					{origins.length === 0 ? (
						<div className="col-span-4 row-span-4 flex items-center justify-center rounded-lg border border-border bg-card/40 p-4 text-sm text-muted-foreground">
							Waiting for measurement frames.
						</div>
					) : (
						origins.map((origin) => (
							<SignalCell
								key={origin}
								origin={origin}
								selected={origin === selected}
								onSelect={selectSource}
							/>
						))
					)}
					<SystemHealthCell />
				</div>

				<div className="min-h-0 overflow-auto rounded-xl border border-border bg-card/40">
					{selected === "" ? (
						<div className="p-5 text-sm text-muted-foreground">
							Select a signal once frames arrive.
						</div>
					) : (
						<SignalPanel origin={selected} />
					)}
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsPage,
});
