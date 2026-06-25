import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { kernelCopy } from "#/components/terminal/kernel-meta";

export const KernelList = ({
	compact = false,
	origins,
}: {
	compact?: boolean;
	origins?: string[];
}) => {
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const { inspectSource, selectSource } = terminalStore.actions;
	const sources = origins ?? Object.keys(readings);

	return (
		<div className="min-h-0 overflow-auto">
			{sources.map((origin) => {
				const frame = readings[origin]?.[focusSymbol] as
					| Record<string, unknown>
					| undefined;
				const output = (frame?.output ?? {}) as Record<string, unknown>;
				const confidence = (output.confidence as number) ?? 0;
				const surprise = (output.surprise as number) ?? 0;
				const copy = kernelCopy(origin, String(output.category ?? origin));
				const inspecting = inspectorSource === origin;
				const selected = selectedSource === origin;
				const confidenceText = `${Math.round(confidence * 100)}%`;

				return (
					<button
						type="button"
						key={origin}
						onClick={() =>
							compact ? selectSource(origin) : inspectSource(origin)
						}
						className="block w-full cursor-pointer border-(--line) border-b border-l-2 px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
						style={{
							borderLeftColor:
								inspecting || selected ? "var(--acc)" : "transparent",
							background:
								inspecting || selected ? "var(--raised)" : "transparent",
						}}
					>
						<div className="flex items-center justify-between gap-2">
							<span
								className={`truncate font-semibold text-(--f1) ${compact ? "text-xs" : "text-[12.5px]"}`}
							>
								{origin}
							</span>
							{compact ? (
								<span className="size-[7px] shrink-0 rounded-full bg-(--up)" />
							) : (
								<span className="shrink-0 font-mono text-[10px] text-(--f2)">
									{confidenceText}
								</span>
							)}
						</div>
						<div className="mt-0.5 truncate font-mono text-[9.5px] text-(--f4)">
							{compact ? `${confidenceText} conf` : copy.sub}
						</div>
						{compact ? null : (
							<div className="mt-[5px] flex items-center gap-2">
								<div className="h-1 flex-1 overflow-hidden rounded-[2px] bg-(--line)">
									<div
										className="h-full bg-info"
										style={{ width: `${Math.round(confidence * 100)}%` }}
									/>
								</div>
								<span className="w-[62px] text-right font-mono text-[9.5px] text-(--f4)">
									{surprise.toFixed(2)}
								</span>
							</div>
						)}
					</button>
				);
			})}
		</div>
	);
};

export const DecisionRows = () => (
	<div className="min-h-0 flex-1 overflow-auto">
		<div className="px-3 py-8 text-center font-mono text-(--f4) text-xs">
			waiting for backend decision rows
		</div>
	</div>
);

export const PositionRows = () => (
	<div className="min-h-0 flex-1 overflow-auto p-1.5">
		<div className="px-2 py-8 text-center font-mono text-(--f4) text-xs">
			No open positions
		</div>
	</div>
);

export const AuditRows = () => (
	<div className="min-h-0 flex-1 overflow-auto py-0.5">
		<div className="px-3 py-8 text-center font-mono text-(--f4) text-xs">
			No audit events
		</div>
	</div>
);
