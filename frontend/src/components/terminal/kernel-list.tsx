import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	orderedKernelSources,
} from "#/components/terminal/kernel-meta";
import {
	kernelFrameForSource,
	kernelHistoryValues,
	kernelReadout,
} from "#/components/terminal/kernel-readout";

export const KernelList = ({
	compact = false,
	origins,
}: {
	compact?: boolean;
	origins?: string[];
}) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const sources = orderedKernelSources(
		origins ?? Object.keys(readings.measurements),
	);
	const { inspectSource, selectSource } = terminalStore.actions;

	if (sources.length === 0) {
		return (
			<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
				waiting for backend measurement frames
			</div>
		);
	}

	return (
		<div className="min-h-0 overflow-auto">
			{sources.map((origin) => {
				const frame = kernelFrameForSource(readings, origin, focusSymbol);
				const { output, confidence, surprise, status } = kernelReadout(frame);
				const copy = kernelCopy(origin, String(output.category ?? origin));
				const statusMeta = kernelStatusMeta(status);
				const spark = kernelSparkPaths(kernelHistoryValues(frame), status);
				const inspecting = inspectorSource === origin;
				const selected = selectedSource === origin;

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
							background: inspecting || selected ? "var(--raised)" : "transparent",
						}}
					>
						<div className="flex items-center justify-between gap-2">
							<span
								className={`truncate font-semibold text-(--f1) ${
									compact ? "text-xs" : "text-[12.5px]"
								}`}
							>
								{copy.name}
							</span>
							{compact ? (
								<span
									className="size-[7px] shrink-0 rounded-full"
									style={{ backgroundColor: statusMeta.fg }}
								/>
							) : (
								<span
									className="shrink-0 rounded-[2px] border px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase"
									style={{
										borderColor: statusMeta.bd,
										backgroundColor: statusMeta.bg,
										color: statusMeta.fg,
									}}
								>
									{statusMeta.label}
								</span>
							)}
						</div>
						{compact ? null : (
							<>
								<svg
									viewBox="0 0 150 30"
									preserveAspectRatio="none"
									className="mt-1.5 block h-[26px] w-full"
								>
									<title>Signal sparkline</title>
									<polyline points={spark.area} fill={spark.fill} stroke="none" />
									<polyline
										points={spark.spark}
										fill="none"
										stroke={spark.line}
										strokeWidth="1.4"
										vectorEffect="non-scaling-stroke"
									/>
								</svg>
								<div className="mt-1.5 flex items-center gap-2">
									<div className="h-1 flex-1 overflow-hidden rounded-[2px] bg-(--line)">
										<div
											className="h-full transition-all duration-500 ease-out"
											style={{
												width: `${confidence * 100}%`,
												backgroundColor: spark.line,
											}}
										/>
									</div>
									<span className="w-7 text-right font-mono text-[10px] text-(--f2)">
										{Math.floor(confidence * 100)}%
									</span>
									<span
										className="w-[62px] text-right font-mono text-[9.5px]"
										style={{
											color:
												status === "ambiguous" ? "var(--acc)" : "var(--f4)",
										}}
									>
										{surprise.toFixed(2)}x thr
									</span>
								</div>
							</>
						)}
					</button>
				);
			})}
		</div>
	);
};
