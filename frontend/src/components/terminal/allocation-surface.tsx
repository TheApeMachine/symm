import { useRef } from "react";
import { equityStore } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import { AllocationMain } from "./allocation-main";
import { AllocationSidePanel } from "./allocation-side-panel";
import { num } from "./number";

export const AllocationSurface = () => {
	const root = useRef<HTMLDivElement>(null);

	equityStore.subscribe((state) => {
		if (!root.current) return;
		const last = state.getLast();
		const cashEl = root.current.querySelector<HTMLElement>('[data-eq="cash"]');
		const unrealizedEl = root.current.querySelector<HTMLElement>(
			'[data-eq="unrealized"]',
		);
		const equityEl =
			root.current.querySelector<HTMLElement>('[data-eq="equity"]');

		if (cashEl) cashEl.textContent = last ? num(last.cash(), 2) : "—";
		if (unrealizedEl)
			unrealizedEl.textContent = last ? num(last.unrealized(), 2) : "—";
		if (equityEl) equityEl.textContent = last ? num(last.equity(), 2) : "—";
	});

	return (
		<div ref={root} className="flex h-full flex-col">
			<div className="flex shrink-0 items-center gap-5.5 border-(--line) border-b bg-(--surface) px-4.5 py-3">
				<div>
					<Typography.Display size="m" className="block">
						Edge-proportional sizing
					</Typography.Display>
					<div className="mt-0.75 font-mono text-[10px] text-(--f4)">
						edge = thesis - median - mad · share = edge / (thesis + sum
						positive) · notional = free x share
					</div>
				</div>
				<div className="ml-auto flex items-center gap-5">
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Deployable
						</span>
						<span
							data-eq="cash"
							className="font-mono text-[13px] font-semibold text-(--f1)"
						>
							—
						</span>
					</div>
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Unrealized
						</span>
						<span
							data-eq="unrealized"
							className="font-mono text-[13px] font-semibold text-(--acc)"
						>
							—
						</span>
					</div>
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Equity
						</span>
						<span
							data-eq="equity"
							className="font-mono text-[13px] font-semibold text-(--f1)"
						>
							—
						</span>
					</div>
				</div>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_320px]">
				<AllocationMain />
				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<AllocationSidePanel />
				</div>
			</div>
		</div>
	);
};
