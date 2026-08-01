import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";

/*
SignalDetail is a direct-paint shell for one backend measurement row.
The Component wrapper binds data-paint paths directly to the live
measurement payload on the top-level measurements key.
*/
export const SignalDetail = () => (
	<Component registerKey="measurements">
		{({ ref, className }) => (
			<Flex.Column
				ref={ref}
				className={className ?? "min-h-0 overflow-auto px-5 py-4.5"}
			>
				<Flex.Row className="items-start justify-between gap-3">
					<span
						data-paint="source"
						className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]"
					/>
					<span
						data-paint="validity.state"
						data-paint-class="invalid:border-(--down),bg-(--sunken),text-(--down) provisional:border-(--info),bg-(--sunken),text-(--info) valid:border-(--up),bg-(--sunken),text-(--up)"
						className="shrink-0 rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
					/>
				</Flex.Row>

				<div className="mt-4.5 grid grid-cols-2 gap-x-5.5 gap-y-3">
					<div className="flex justify-between font-mono text-xs">
						<span className="text-(--f3)">Symbol</span>
						<span data-paint="symbol" className="text-(--f1)" />
					</div>
					<div className="flex justify-between font-mono text-xs">
						<span className="text-(--f3)">Observed</span>
						<span data-paint="at" className="text-(--f1)" />
					</div>
					<div className="flex justify-between font-mono text-xs">
						<span className="text-(--f3)">Metric</span>
						<span data-paint="metric" className="text-(--f1)" />
					</div>
					<div className="flex justify-between font-mono text-xs">
						<span className="text-(--f3)">Unit</span>
						<span data-paint="unit" className="text-(--f1)" />
					</div>
				</div>

				<div className="mt-5 grid grid-cols-2 gap-x-5.5 gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
					<div className="flex justify-between">
						<span className="text-(--f3)">Raw</span>
						<span
							data-paint="raw"
							data-paint-format=".4f"
							className="text-(--f1)"
						/>
					</div>
					<div className="flex justify-between">
						<span className="text-(--f3)">Normalized</span>
						<span
							data-paint="normalized"
							data-paint-format=".4f"
							className="text-(--f1)"
						/>
					</div>
					<div className="flex justify-between">
						<span className="text-(--f3)">Readiness</span>
						<span data-paint="validity.readiness" className="text-(--f1)" />
					</div>
					<div className="flex justify-between">
						<span className="text-(--f3)">Reason</span>
						<span data-paint="validity.reason" className="text-(--f1)" />
					</div>
					<div className="flex justify-between">
						<span className="text-(--f3)">Scale kind</span>
						<span data-paint="scale.kind" className="text-(--f1)" />
					</div>
					<div className="flex justify-between">
						<span className="text-(--f3)">Confidence</span>
						<span
							data-paint="uncertainty.confidence"
							data-paint-format=".4f"
							className="text-(--f1)"
						/>
					</div>
				</div>
			</Flex.Column>
		)}
	</Component>
);
