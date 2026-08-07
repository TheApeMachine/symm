import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	readinessGate,
	sourceHeadlineMetric,
} from "#/components/terminal/kernel-meta";
import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";

/*
SignalDetail reads one kernel's measurement for the focused symbol.

A measurement row is a source, a symbol, an instant and a map of named metrics —
each of which carries its own raw value, its normalized value and its unit. This
panel used to bind a flattened row that had none of that structure: metric, raw,
unit and a validity block, none of which the wire sends. It now pins the row it
wants with data-scope and reads the kernel's headline metric out of the map by
name, so the reading and the unit under it always describe the same thing.

The badge is the readiness gate for the same kernel, because a measurement row
carries no verdict on itself.
*/
const Reading = ({ label, bind, format }: {
	label: string;
	bind: string;
	format?: string;
}) => (
	<div className="flex justify-between font-mono text-xs">
		<span className="text-(--f3)">{label}</span>
		<span data-paint={bind} data-paint-format={format} className="text-(--f1)" />
	</div>
);

export const SignalDetail = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const source = useSelector(terminalStore, (state) => state.selectedSource);
	const copy = kernelCopy(source, "");
	const headline = sourceHeadlineMetric(source);

	return (
		<Component registerKey="measurements">
			{({ ref, className }) => (
				<Flex.Column
					ref={ref}
					data-scope="source,symbol"
					data-filter={`${source},${focusSymbol}`}
					className={className ?? "min-h-0 overflow-auto px-5 py-4.5"}
				>
					<Flex.Row className="items-start justify-between gap-3">
						<div className="min-w-0">
							<span className="block font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]">
								{copy.name}
							</span>
							<span className="mt-1 block font-mono text-[10px] text-(--f4)">
								{copy.sub}
							</span>
						</div>
						<Component registerKey="readiness">
							{({ ref: gateRef }) => (
								<span ref={gateRef} className="contents">
									<span
										data-paint={readinessGate(source)}
										data-paint-prop="dataset.gate"
										data-paint-class="true:border-(--up),text-(--up) false:border-(--line2),text-(--f3)"
										className="group shrink-0 rounded-[3px] border border-(--line2) bg-(--sunken) px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
									>
										<span className="group-data-[gate=true]:hidden">
											standby
										</span>
										<span className="hidden group-data-[gate=true]:inline">
											live
										</span>
									</span>
								</span>
							)}
						</Component>
					</Flex.Row>

					<p className="mt-3.5 max-w-prose text-[12px] text-(--f3) leading-relaxed">
						{copy.blurb}
					</p>

					<div className="mt-4.5 grid grid-cols-2 gap-x-5.5 gap-y-3">
						<Reading label="Symbol" bind="symbol" />
						<Reading label="Observed" bind="at" format="time" />
						<Reading label="Metric" bind="source" />
						<Reading label="Unit" bind={`${headline}.unit`} />
					</div>

					<div className="mt-5 grid grid-cols-2 gap-x-5.5 gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
						<Reading label="Raw" bind={`${headline}.raw`} format=".4f" />
						<Reading
							label="Normalized"
							bind={`${headline}.normalized`}
							format=".4f"
						/>
						{/*
							Maturity is how much evidence the kernel has accumulated. Only
							some kernels publish it; where one does not, the slot keeps its
							last reading rather than inventing a zero.
						*/}
						<Reading label="Maturity" bind="maturity" format=".3f" />
						<Reading label="Horizon" bind="horizon" />
						<Reading label="Peer" bind="peer" />
						<Reading label="Since" bind="observedFrom" format="time" />
						<Reading
							label="Confidence"
							bind="uncertainty.confidence"
							format=".4f"
						/>
					</div>
				</Flex.Column>
			)}
		</Component>
	);
};
