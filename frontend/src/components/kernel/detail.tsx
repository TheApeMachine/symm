import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	metricLabel,
	sourceHeadlineMetric,
	sourceMetrics,
} from "#/components/terminal/kernel-meta";
import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { Gate } from "@/components/ui/gate";
import { Typography } from "@/components/ui/typography";

/*
SignalDetail reads one kernel's measurement for the focused symbol.

A measurement row is a source, a symbol, an instant and a map of named metrics —
each of which carries its own raw value, its normalized value and its unit. The
panel pins the row it wants with data-scope and reads that kernel's metrics out
of the map by name, so every reading and the unit under it describe the same
thing.

The badge is raised by the presence of this source and focused symbol's own
measurement row, independently of the engine-wide readiness gates.
*/
const Reading = ({
	label,
	bind,
	format,
}: {
	label: string;
	bind: string;
	format?: string;
}) => (
	<Flex.Row justify="between" align="baseline" className="gap-2">
		<Typography.Label size="xxs" tone="f3" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono
			size="m"
			tone="f1"
			data-paint={bind}
			data-paint-format={format}
			data-paint-absent="—"
		/>
	</Flex.Row>
);

/*
MetricMeter is one named reading of the kernel: its raw value beside a bar of
where that value sits on its own normalized scale.

The bar is clamped rather than scaled — a signed metric runs through zero, and
the half of its range below zero is a real reading, not a bar to stretch. A
metric the kernel publishes without a normalized estimate keeps an empty bar
beside a live figure, which is the honest picture of a reading that has no scale
behind it yet.
*/
const MetricMeter = ({ metric }: { metric: string }) => (
	<div>
		<Flex.Row justify="between" align="center" className="mb-1.5 gap-2">
			<Typography.Label size="xxs" tone="f3" weight="normal">
				{metricLabel(metric)}
			</Typography.Label>
			<Typography.Mono
				size="s"
				tone="f1"
				data-paint={`metrics.${metric}.raw`}
				data-paint-format=".4f"
				data-paint-absent="—"
			/>
		</Flex.Row>
		<div className="h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
			<div
				data-set={`metrics.${metric}.normalized`}
				data-target="style.--fill"
				className="h-full bg-(--acc) transition-[width] duration-500 ease-out"
				style={{ width: "calc(clamp(0, var(--fill, 0), 1) * 100%)" }}
			/>
		</div>
	</div>
);

/*
HeatCell is one symbol's standing on the selected kernel. The cell is pinned to
its own row, so a symbol the kernel has not measured stays unlit rather than
borrowing its neighbour's colour.
*/
const HeatCell = ({ source, symbol }: { source: string; symbol: string }) => (
	<div
		data-scope="source,symbol"
		data-filter={`${source},${symbol}`}
		data-set={`${sourceHeadlineMetric(source)}.normalized`}
		data-target="style.--heat"
		title={`${symbol} · ${source}`}
		className="flex aspect-square items-center justify-center overflow-hidden rounded-xs border border-(--line) font-mono text-[8px] text-(--f3)"
		style={{
			background:
				"color-mix(in srgb, var(--acc) calc(clamp(0, var(--heat, 0), 1) * 100%), var(--sunken))",
		}}
	>
		{symbol.replace(/\/.*$/, "")}
	</div>
);

export const SignalDetail = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const kernels = useSelector(appStore, (state) => state.kernels);
	const symbols = useSelector(appStore, (state) => state.symbols);
	const selected = useSelector(terminalStore, (state) => state.selectedSource);
	/*
		Nothing is selected until the user picks a kernel, and the rail is ordered,
		so the panel opens on the first kernel the run actually has rather than on a
		name that may not be among them.
	*/
	const source = selected || kernels[0] || "";
	const copy = kernelCopy(source, "");
	const metrics = sourceMetrics(source);

	return (
		<Component registerKey="measurements">
			{({ ref, className }) => (
				<Flex.Column
					/*
						Direct paint retains what it last wrote, which is what keeps a
						surface alive between sparse batches. Across a change of kernel that
						retention would be a lie, so the panel is rebuilt on the source it
						is reading and every slot starts empty again.
					*/
					key={source}
					ref={ref}
					className={className ?? "min-h-0 overflow-auto px-5 py-4.5"}
				>
					<Flex.Row className="items-start justify-between gap-3">
						<Flex.Column className="min-w-0 gap-1">
							<Typography.Display size="xl">{copy.name}</Typography.Display>
							<Typography.Mono size="s" tone="f4">
								{copy.sub}
							</Typography.Mono>
						</Flex.Column>
						<span
							data-scope="source,symbol"
							data-filter={`${source},${focusSymbol}`}
						>
							<Gate bind="symbol" presence size="s" />
						</span>
					</Flex.Row>

					<Typography.Paragraph className="mt-3.5 max-w-prose text-[12px] text-(--f3) leading-relaxed">
						{copy.blurb}
					</Typography.Paragraph>

					<div
						data-scope="source,symbol"
						data-filter={`${source},${focusSymbol}`}
					>
						<div className="mt-4.5 grid grid-cols-2 gap-x-5.5 gap-y-3">
							{metrics.map((metric) => (
								<MetricMeter key={metric} metric={metric} />
							))}
						</div>

						<div className="mt-5 grid grid-cols-2 gap-x-5.5 gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
							<Reading label="Symbol" bind="symbol" />
							<Reading label="Observed" bind="at" format="time" />
							<Reading
								label="Unit"
								bind={`${sourceHeadlineMetric(source)}.unit`}
							/>
							{/*
								Maturity is how much evidence the kernel has accumulated. Only
								some kernels publish it; where one does not, the slot reads as
								absent rather than inventing a zero.
							*/}
							<Reading label="Maturity" bind="maturity" format=".3f" />
							<Reading label="Peer" bind="peer" />
							{/*
								ObservedFrom and Horizon are deliberately absent. Most kernels
								emit a zero-valued window start, which reads as a real instant
								in year one, and Horizon is a nanosecond count that no format
								here turns back into a duration. Both would state something the
								row does not actually say.
							*/}
						</div>
					</div>

					{/*
						The batch is every reading the engine published this epoch, across
						every kernel — the run's measurement rate, which is the one number
						that says whether this surface is being fed at all.
					*/}
					<div className="mt-3.5 grid grid-cols-2 gap-x-5.5 gap-y-2 font-mono text-xs">
						<Reading label="Readings this epoch" bind="length" />
					</div>

					{symbols.length === 0 ? null : (
						<div className="mt-5">
							<Typography.Label
								size="xxs"
								tone="f3"
								className="mb-2 block tracking-[0.13em]"
							>
								Cross-section · {source} headline
							</Typography.Label>
							<div className="grid grid-cols-12 gap-0.75">
								{symbols.slice(0, 24).map((symbol) => (
									<HeatCell key={symbol} source={source} symbol={symbol} />
								))}
							</div>
						</div>
					)}
				</Flex.Column>
			)}
		</Component>
	);
};
