import { useRef, useState } from "react";
import { equityStore, strategyStore } from "#/collections/app";
import { Decision } from "#/providers/telemetry/telemetry/decision";
import { num } from "./number";

type CandidateEntry = {
	id: string;
	symbol: string;
};

type SizingQueryEntry = {
	symbol: HTMLElement | null;
	qty: HTMLElement | null;
	notional: HTMLElement | null;
};

const queryCache: Record<string, SizingQueryEntry> = {};
const decObj = new Decision();

export const AllocationSidePanel = () => {
	const root = useRef<HTMLDivElement>(null);
	const [candidates, setCandidates] = useState<CandidateEntry[]>([]);

	equityStore.subscribe((state) => {
		if (!root.current) return;
		const last = state.getLast();
		const unrealizedEl = root.current.querySelector<HTMLElement>('[data-eq="unrealized"]');
		const cashEl = root.current.querySelector<HTMLElement>('[data-eq="cash"]');
		const equityEl = root.current.querySelector<HTMLElement>('[data-eq="equity"]');
		const bar = root.current.querySelector<HTMLElement>("[data-eq-bar]");

		if (unrealizedEl) unrealizedEl.textContent = last ? num(last.unrealized(), 2) : "—";
		if (cashEl) cashEl.textContent = last ? num(last.cash(), 2) : "—";
		if (equityEl) equityEl.textContent = last ? num(last.equity(), 2) : "—";

		if (bar instanceof HTMLElement && last) {
			const cashN = Number(last.cash());
			const eqN = Number(last.equity());
			if (Number.isFinite(cashN) && Number.isFinite(eqN) && eqN > 0) {
				bar.style.width = `${Math.min(100, Math.max(0, (cashN / eqN) * 100)).toFixed(3)}%`;
			} else {
				bar.style.width = "0%";
			}
		}

	});

	strategyStore.subscribe((state) => {
		if (!root.current) return;
		const last = state.getLast();
		if (!last) return;

		const currentCandidates: CandidateEntry[] = [];

		for (let index = 0; index < last.decisionsLength(); index += 1) {
			const decision = last.decisions(index, decObj);
			if (!decision) continue;

			const id = decision.id() ?? "";
			const symbol = decision.symbol() ?? "";
			if (!id) continue;

			currentCandidates.push({ id, symbol });

			let element = queryCache[id];
			if (!element) {
				const row = root.current.querySelector<HTMLElement>(`[data-decision-id="${id}"]`);
				if (!row) continue;

				element = {
					symbol: row.querySelector<HTMLElement>('[data-f="symbol"]'),
					qty: row.querySelector<HTMLElement>('[data-f="qty"]'),
					notional: row.querySelector<HTMLElement>('[data-f="notional"]'),
				};
				queryCache[id] = element;
			}

			if (element.symbol) element.symbol.textContent = symbol;
			if (element.qty) element.qty.textContent = num(decision.proposedQuantity(), 6);
			if (element.notional) element.notional.textContent = num(decision.proposedNotional(), 2);
		}

		if (currentCandidates.map((c) => c.id).join(",") !== candidates.map((c) => c.id).join(",")) {
			setCandidates(currentCandidates);
		}
	});

	return (
		<div ref={root} className="flex flex-col gap-3.5">
			<div className="rounded-[3px] border border-(--line) bg-(--surface) p-3">
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

			<div className="rounded-[3px] border border-(--line) bg-(--surface) p-3">
				<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">Position sizing</div>
				<div className="mb-2.75 font-mono text-[9.5px] text-(--f4)">
					notional the allocator sized, against the slots it had
				</div>
				<div className="flex flex-col gap-2.25">
					{candidates.length === 0 ? (
						<div className="border-(--line) border-t pt-2.75 font-mono text-[9.5px] text-(--f4)">
							no sized candidates this round
						</div>
					) : (
						candidates.map((candidate) => (
							<div key={candidate.id} data-decision-id={candidate.id} className="flex items-center justify-between gap-2 font-mono text-[10px]">
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