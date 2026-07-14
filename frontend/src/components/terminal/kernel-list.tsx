import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	kernelStatusVariant,
} from "#/components/terminal/kernel-meta";
import {
	ageText,
	formatRaw,
	headlineMetric,
	latestByMetric,
	latestEpoch,
	metricLabel,
	percentOf,
	resolveStatus,
	seriesByMetric,
	stampOf,
} from "#/components/terminal/measurement-view";
import { Badge } from "@/components/ui/badge";
import { Meter } from "@/components/ui/meter";

export const KernelList = ({
	compact = false,
	sources: inputSources,
}: {
	compact?: boolean;
	sources?: string[];
}) => {
	const app = useSelector(appStore, (state) => state);
	const mStore = useSelector(measurementsStore, (state) => state);
	const measurements = mStore.measurements[app.focusSymbol] ?? {};
	const terminal = useSelector(terminalStore, (state) => state);
	const sources = (inputSources ?? Object.keys(measurements)).sort();
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
			{sources.map((source) => {
				const values = measurements[source]?.values() ?? [];
				const headline = headlineMetric(source);
				const latest =
					headline === null ? values.at(-1) : latestByMetric(values, headline);
				const status = resolveStatus(latest);
				const statusMeta = kernelStatusMeta(status);
				const copy = kernelCopy(source, source);
				const spark = kernelSparkPaths(
					headline === null ? [] : seriesByMetric(values, headline),
					status,
				);
				const inspecting = terminal.inspectorSource === source;
				const selected = terminal.selectedSource === source;
				const epoch = latestEpoch(values);
				const percent =
					headline === null || latest === undefined ? 0 : percentOf(latest);
				const readout =
					headline === null
						? `${epoch.length} readings`
						: latest === undefined
							? "—"
							: `${metricLabel(headline)} ${formatRaw(latest)}`;

				return (
					<button
						type="button"
						key={source}
						onClick={() =>
							compact ? selectSource(source) : inspectSource(source)
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
								className={`truncate font-semibold text-(--f1) ${
									compact ? "text-xs" : "text-[12.5px]"
								}`}
							>
								{compact ? copy.name : source}
							</span>

							{compact ? (
								<span
									className="size-[7px] shrink-0 rounded-full"
									style={{ backgroundColor: statusMeta.fg }}
								/>
							) : (
								<Badge
									label={statusMeta.label}
									variant={kernelStatusVariant(status)}
								/>
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
									<polyline
										points={spark.area}
										fill={spark.fill}
										stroke="none"
									/>
									<polyline
										points={spark.spark}
										fill="none"
										stroke={spark.line}
										strokeWidth="1.4"
										vectorEffect="non-scaling-stroke"
									/>
								</svg>

								<div className="mt-1.5 flex items-center gap-2">
									{headline === null ? null : (
										<Meter
											layout="bar"
											percent={percent}
											variant={spark.active ? "warning" : "info"}
											size="xs"
											trackClassName="flex-1"
											animated
										/>
									)}

									<span className="flex-1 truncate text-right font-mono text-[10px] text-(--f2)">
										{readout}
									</span>

									<span className="w-[46px] shrink-0 text-right font-mono text-[9.5px] text-(--f4)">
										{ageText(stampOf(latest?.at))}
									</span>
								</div>
							</>
						)}
						{compact ? (
							<div className="mt-1 truncate font-mono text-[9px] text-(--f4)">
								{latest === undefined ? statusMeta.label : readout}
							</div>
						) : null}
					</button>
				);
			})}
		</div>
	);
};
