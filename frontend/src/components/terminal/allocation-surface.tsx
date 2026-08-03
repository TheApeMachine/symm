import {Component} from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { AllocationMain } from "./allocation-main";
import { AllocationSidePanel } from "./allocation-side-panel";


/*
AllocationSurface owns the mounted allocation shell and restores retained state
immediately when the user navigates to it between sparse model frames.
*/
export const AllocationSurface = () => {
	return (
		<Component registerKey="equity">
			{({ ref, className }) => (
		<div ref={ref} className={cn("flex h-full flex-col", className)}>
			<div className="flex shrink-0 items-center gap-5.5 border-(--line) border-b bg-(--surface) px-4.5 py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Edge-proportional sizing
					</div>
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
							data-paint="cash"
							data-paint-format=".2f"
							className="font-mono text-[13px] font-semibold text-(--f1)"
						/>
					</div>
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Unrealized
						</span>
						<span
							data-paint="unrealized"
							data-paint-format=".2f"
							className="font-mono text-[13px] font-semibold text-(--acc)"
						/>
					</div>
					<div className="flex flex-col items-end gap-px">
						<span className="text-[9px] text-(--f4) uppercase tracking-widest">
							Equity
						</span>
						<span
							data-paint="equity"
							data-paint-format=".2f"
							className="font-mono text-[13px] font-semibold text-(--f1)"
						/>
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
		)}
		</Component>
	);
};
