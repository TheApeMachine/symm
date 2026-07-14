import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { StatusBadge } from "#/components/dashboard/status-badge";
import {
	kernelCopy,
	kernelStatusMeta,
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
	sideLabel,
	stampOf,
} from "#/components/terminal/measurement-view";
import { Flex } from "@/components/ui/flex";
import { Grid } from "@/components/ui/grid";
import { InspectorMeter } from "./meter";

const clampPercent = (value: number) => Math.max(0, Math.min(100, value * 100));

export const SignalDetail = () => {
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const measurements = useSelector(measurementsStore, (state) => state);
	const source = selectedSource;
	const history =
		measurements.measurements[focusSymbol]?.[source]?.values() ?? [];
	const headline = headlineMetric(source);
	const latest =
		headline === null ? history.at(-1) : latestByMetric(history, headline);
	const epoch = latestEpoch(history);
	const status = resolveStatus(latest);
	const copy = kernelCopy(selectedSource, selectedSource);
	const statusMeta = kernelStatusMeta(status);
	const observedStamp = stampOf(latest?.at);
	const active = Object.values(measurements.measurements).reduce(
		(sum, sources) => sum + (sources[source]?.values().length ?? 0),
		0,
	);
	const total = Object.values(measurements.measurements).reduce(
		(sum, sources) =>
			sum +
			Object.values(sources).reduce(
				(sourceSum, sourceHistory) => sourceSum + sourceHistory.values().length,
				0,
			),
		0,
	);
	const heatmap =
		headline === null
			? []
			: Object.entries(measurements.measurements).flatMap(
					([symbol, sources]) => {
						const frame = latestByMetric(
							sources[source]?.values() ?? [],
							headline,
						);

						return frame === undefined
							? []
							: [{ symbol, value: percentOf(frame) / 100 }];
					},
				);

	return (
		<Flex.Column className="min-h-0 overflow-auto px-5 py-[18px]">
			<Flex.Row className="items-start justify-between gap-3">
				<Flex.Column>
					<Flex className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]">
						{copy.name}
					</Flex>
					<Flex className="mt-1 font-mono text-[11px] text-(--f3)">
						{copy.sub}
					</Flex>
				</Flex.Column>
				<StatusBadge label={statusMeta.label} tone={statusMeta.fg} />
			</Flex.Row>
			<Flex className="mt-3.5 max-w-[560px] font-serif text-[15px] text-(--f2) leading-[1.55]">
				{copy.blurb}
			</Flex>
			{epoch.length === 0 ? (
				<Flex className="mt-[18px] rounded border border-(--line) bg-(--sunken) px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
					waiting for backend {selectedSource} measurement
				</Flex>
			) : null}
			{epoch.length === 0 ? null : (
				<Grid
					cols={2}
					className="mt-[18px] gap-x-[22px] gap-y-3"
					style={{ gridTemplateColumns: "repeat(2, minmax(0, 1fr))" }}
				>
					{epoch.map((measurement) => (
						<InspectorMeter
							key={`${measurement.metric}:${measurement.side ?? ""}`}
							label={[
								metricLabel(measurement.metric),
								sideLabel(measurement.side),
							]
								.filter(Boolean)
								.join(" · ")}
							value={formatRaw(measurement)}
							percent={percentOf(measurement)}
							color={
								measurement.metric === headline ? "var(--acc)" : "var(--info)"
							}
						/>
					))}
				</Grid>
			)}
			<Grid
				cols={2}
				className="mt-5 gap-x-[22px] gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs"
				style={{ gridTemplateColumns: "repeat(2, minmax(0, 1fr))" }}
			>
				<Flex.Row className="justify-between">
					<Flex className="text-(--f3)">Active readings</Flex>
					<Flex className="text-(--f1)">
						{active.toLocaleString()} / {total.toLocaleString()}
					</Flex>
				</Flex.Row>
				<Flex.Row className="justify-between">
					<Flex className="text-(--f3)">Metrics this tick</Flex>
					<Flex className="text-(--f1)">{epoch.length}</Flex>
				</Flex.Row>
				<Flex.Row className="justify-between">
					<Flex className="text-(--f3)">Observed</Flex>
					<Flex className="text-(--f1)">
						{Number.isFinite(observedStamp)
							? `${new Date(observedStamp).toLocaleTimeString("en-US", {
									hour12: false,
								})} / ${ageText(observedStamp)}`
							: "— / —"}
					</Flex>
				</Flex.Row>
				<Flex.Row className="justify-between">
					<Flex className="text-(--f3)">Validity</Flex>
					<Flex className="text-(--f1)">
						{latest?.validity?.reason || latest?.validity?.state || "—"}
					</Flex>
				</Flex.Row>
			</Grid>
			{headline === null ? null : (
				<Flex.Column className="mt-[18px]">
					<Flex className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Cross-section · {metricLabel(headline)} heatmap
					</Flex>
					<Grid cols={12} className="gap-[3px]">
						{heatmap.map((cell) => {
							const percent = Math.round(clampPercent(cell.value));
							const label = cell.symbol.split("/")[0] ?? cell.symbol;

							return (
								<Flex
									key={cell.symbol}
									data-symbol={cell.symbol}
									title={`${cell.symbol} · ${percent}%`}
									className="aspect-square cursor-pointer items-center justify-center rounded-[2px] font-mono text-[8px]"
									style={{
										background: `color-mix(in srgb, var(--acc) ${percent}%, var(--sunken))`,
										color: percent > 62 ? "#14110f" : "var(--f3)",
									}}
								>
									{label}
								</Flex>
							);
						})}
					</Grid>
				</Flex.Column>
			)}
		</Flex.Column>
	);
};
