import { equityStore, strategyStore, useSubscribe } from "#/providers/ws-stores";
import { num } from "./number";
export const AllocationSidePanel = () => {
	const equity = useSubscribe(equityStore, (state) => {
		const set = (which: string, value: string) => {
			const el = equity.current?.querySelector<HTMLElement>(`[data-eq=${which}]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("unrealized", num(state?.unrealized, 2));
		set("cash", num(state?.cash, 2));
		set("equity", num(state?.equity, 2));

		const cash = state?.cash;
		const equityValue = state?.equity;
		const cashN = typeof cash === "number" ? cash : Number(cash);
		const eqN = typeof equityValue === "number" ? equityValue : Number(equityValue);
		const bar = equity.current?.querySelector<HTMLElement>("[data-eq-bar]");

		if (bar instanceof HTMLElement) {
			if (Number.isFinite(cashN) && Number.isFinite(eqN) && eqN > 0) {
				bar.style.width = `${Math.min(100, Math.max(0, (cashN / eqN) * 100)).toFixed(3)}%`;
			} else {
				bar.style.width = "0%";
			}
		}
	});

	const strategy = useSubscribe(strategyStore, (state) => {
		const decisions = state?.decisions ?? [];

		for (let index = 0; index < decisions.length; index += 1) {
			const decision = decisions[index];

			if (decision === undefined) {
				continue;
			}

			const row = strategy.current?.querySelector<HTMLElement>(`[data-i="${index}"]`);

			if (row === null || row === undefined) {
				continue;
			}

			const set = (which: string, value: string) => {
				const el = row.querySelector<HTMLElement>(`[data-f=${which}]`);

				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			};

			set("symbol", decision.symbol);
			set("qty", num(decision.proposedQuantity, 6));
			set("notional", num(decision.proposedNotional, 2));
		}
	});

	return (
		<div className="flex flex-col gap-3.5">
			<div ref={equity} className="rounded-[3px] border border-(--line) bg-(--surface) p-3">
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">Capital deployment</span>
					<span data-eq="unrealized" className="font-mono text-[11px] text-(--acc)">—</span>
				</div>
				<div className="mt-1 mb-2.75 font-mono text-[9.5px] text-(--f4)">
					cash against equity if every lot were closed now
				</div>
				<div className="h-2 overflow-hidden rounded-xs bg-(--line)">
					<div data-eq-bar className="h-full bg-(--acc)" style={{ width: "0%" }} />
				</div>
				<div className="mt-1.75 flex justify-between font-mono text-[10px] text-(--f3)">
					<span>cash <span data-eq="cash">—</span></span>
					<span>equity <span data-eq="equity">—</span></span>
				</div>
			</div>

			<div ref={strategy} className="rounded-[3px] border border-(--line) bg-(--surface) p-3">
				<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">Position sizing</div>
				<div className="mb-2.75 font-mono text-[9.5px] text-(--f4)">
					notional the allocator sized, against the slots it had
				</div>
				<div className="flex flex-col gap-2.25">
					{(strategyStore.state?.decisions ?? []).length === 0 ? (
						<div className="border-(--line) border-t pt-2.75 font-mono text-[9.5px] text-(--f4)">
							no sized candidates this round
						</div>
					) : (
						(strategyStore.state?.decisions ?? []).map((decision, index) => (
							<div key={decision.id} data-i={index} className="flex items-center justify-between gap-2 font-mono text-[10px]">
								<span data-f="symbol" className="min-w-0 flex-1 truncate text-(--f1)" />
								<span data-f="qty" className="shrink-0 text-(--f3)" />
								<span data-f="notional" className="w-18 shrink-0 text-right text-(--acc)" />
							</div>
						))
					)}
				</div>
			</div>
		</div>
	);
};