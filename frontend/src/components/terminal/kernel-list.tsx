import { useSelector } from "@tanstack/react-store";
import { appStore, DEFAULT_KERNELS } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	metricLabel,
	sourceHeadlineMetric,
} from "#/components/terminal/kernel-meta";
import { cn } from "#/lib/utils";
import { Component } from "../ui/component";
import { Dot } from "../ui/dot";
import { Gate } from "../ui/gate";
import { Sparkline } from "../ui/sparkline";

const interactive = (compact: boolean, source: string) => {
	if (compact) {
		return () => {
			terminalStore.actions.selectSource(source);
		};
	}

	return () => {
		terminalStore.actions.inspectSource(source);
	};
};

const KernelTrace = ({ metric }: { metric: string }) => (
	<div>
		<Sparkline
			bind={`metrics.${metric}.raw`}
			title={`${metricLabel(metric)} trace`}
			className="h-6.5"
		/>
		<div className="mt-1 flex items-center gap-2">
			<div className="h-1 flex-1 overflow-hidden rounded-xs bg-(--line)">
				<div
					data-set={`metrics.${metric}.raw`}
					data-target="style.--strength"
					className="h-full bg-(--acc)"
					style={{ width: "calc(var(--strength, 0) * 100%)" }}
				/>
			</div>
			<span
				data-paint={`metrics.${metric}.raw`}
				data-paint-format=".0%"
				className="w-8 shrink-0 text-right font-mono text-[10px] text-(--f2)"
			/>
			<span
				data-paint={`metrics.${metric}.raw`}
				data-paint-format=".3f"
				className="w-16 shrink-0 truncate text-right font-mono text-[9.5px] text-(--acc)"
			/>
		</div>
	</div>
);

/*
KernelList binds each stable row directly to the raw measurements stream.
Rows select their matching frame via data-scope/data-filter and then paint
only the fields they need from that frame.
*/
export const KernelList = ({
	compact = false,
	sources = DEFAULT_KERNELS,
}: {
	compact?: boolean;
	sources?: string[];
}) => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="measurements">
			{({ ref, className }) => (
				<div ref={ref} className={cn("min-h-0 overflow-auto", className)}>
					{sources.map((source) => {
						const headline = sourceHeadlineMetric(source).slice(
							"metrics.".length,
						);

						return (
							<button
								key={source}
								type="button"
								data-scope="source,symbol"
								data-filter={`${source},${focusSymbol}`}
								onClick={interactive(compact, source)}
								className="block w-full cursor-pointer border-(--line) border-b border-l-2 border-l-transparent bg-transparent px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
							>
								<div className="flex items-center justify-between gap-2">
									<span
										className={cn("truncate font-semibold text-(--f1)", {
											"text-xs": compact,
											"text-[12.5px]": !compact,
										})}
									>
										{source}
									</span>

									{compact ? (
										/*
									A row exists only after this kernel has observed the focused
									symbol. Presence therefore raises the row's own status dot;
									global readiness belongs to the engine, not this symbol.
								*/
										<Dot
											size="m"
											data-set="symbol"
											data-set-scale="presence-color"
											data-target="style.--dot-tone"
											className="size-1.75"
										/>
									) : (
										/*
									The scoped measurement's presence is the observation verdict.
									No frontend calculation or global gate is involved.
								*/
										<Gate bind="symbol" presence className="shrink-0" />
									)}
								</div>

								{/*
								A row is a kernel's standing at a glance: where its readings
								sit on their own scales, and the symbol they were measured
							on. Both come from the same row, so the two never describe
							different observations.
						*/}
								<div className="mt-0.5 flex items-baseline gap-1.5 truncate font-mono text-[9.5px] text-(--f4)">
									<span
										data-paint={`metrics.${headline}.raw`}
										data-paint-format=".0%"
										className="text-(--acc)"
									/>
									<span data-paint="symbol" />
								</div>

								{compact ? null : (
									<div className="mt-1.5">
										<KernelTrace metric={headline} />
									</div>
								)}
							</button>
						);
					})}
				</div>
			)}
		</Component>
	);
};
