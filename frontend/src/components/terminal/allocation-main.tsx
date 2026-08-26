import { useRef, useState } from "react";
import { strategyStore } from "#/collections/app";
import { Decision } from "#/providers/telemetry/telemetry/decision";
import { num } from "./number";

type CandidateEntry = {
	id: string;
	symbol: string;
};

type QueryEntry = {
	symbol: HTMLElement | null;
	conf: HTMLElement | null;
	thesisScore: HTMLElement | null;
	action: HTMLElement | null;
	haircut: HTMLElement | null;
	cls: HTMLElement | null;
	notional: HTMLElement | null;
};

const queryCache: Record<string, QueryEntry> = {};
const decObj = new Decision();

export const AllocationMain = () => {
	const root = useRef<HTMLDivElement>(null);
	const [candidates, setCandidates] = useState<CandidateEntry[]>([]);

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
				const row = root.current.querySelector<HTMLElement>(`[data-id="${id}"]`);
				if (!row) continue;

				element = {
					symbol: row.querySelector<HTMLElement>('[data-f="symbol"]'),
					conf: row.querySelector<HTMLElement>("[data-conf]"),
					thesisScore: row.querySelector<HTMLElement>('[data-f="thesisScore"]'),
					action: row.querySelector<HTMLElement>('[data-f="action"]'),
					haircut: row.querySelector<HTMLElement>('[data-f="haircut"]'),
					cls: row.querySelector<HTMLElement>('[data-f="class"]'),
					notional: row.querySelector<HTMLElement>('[data-f="notional"]'),
				};
				queryCache[id] = element;
			}

			if (element.symbol) element.symbol.textContent = symbol;
			if (element.thesisScore) element.thesisScore.textContent = decision.thesisScore().toFixed(5);
			if (element.action) element.action.textContent = decision.action() ?? "—";
			if (element.haircut) element.haircut.textContent = `${(decision.allocationHaircut() * 100).toFixed(1)}%`;
			if (element.cls) element.cls.textContent = decision.allocationClass() || "—";
			if (element.notional) element.notional.textContent = num(decision.proposedNotional(), 2);
			if (element.conf) {
				const v = decision.confidence();
				element.conf.style.width = `clamp(0%, calc(${typeof v === "number" ? v : 0} * 100%), 100%)`;
			}
		}

		if (currentCandidates.map((c) => c.id).join(",") !== candidates.map((c) => c.id).join(",")) {
			setCandidates(currentCandidates);
		}
	});

	return (
		<div ref={root} className="min-h-0 overflow-auto px-4.5 py-4">
			<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
				<span className="text-(--f3)">cross-section</span>
				<span className="text-(--f4)">
					candidates{" "}
					<span className="text-(--f2)">{candidates.length}</span>
				</span>
				<span className="ml-auto text-(--f4)">
					sized from current asks · ranked by structural thesis
				</span>
			</div>

			<div className="flex items-center gap-2.25 border-(--line) border-b pb-1.75 font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
				<span className="w-14.5 shrink-0">symbol</span>
				<span className="flex-1">structural thesis {"->"} confidence</span>
				<span className="w-13 shrink-0 text-right">haircut</span>
				<span className="w-10.5 shrink-0 text-right">class</span>
				<span className="w-18.5 shrink-0 text-right">notional</span>
			</div>

			<div className="flex flex-col">
				{candidates.length === 0 ? (
					<div className="py-24 text-center font-mono text-[11px] text-(--f4)">
						waiting for backend decision frames
					</div>
				) : (
					candidates.map((candidate) => (
						<div
							key={candidate.id}
							data-id={candidate.id}
							className="flex items-center gap-2.25 border-(--line) border-b py-1.75 font-mono text-[10px]"
						>
							<span data-f="symbol" className="w-14.5 shrink-0 truncate font-semibold text-(--f1)" />
							<div className="flex flex-1 items-center gap-2">
								<div className="h-1.25 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
									<div data-conf className="h-full bg-(--acc)" style={{ width: "0%" }} />
								</div>
								<span data-f="thesisScore" className="w-16 shrink-0 text-right text-(--f2)" />
								<span data-f="action" className="w-12 shrink-0 text-right text-[9px] uppercase" />
							</div>
							<span data-f="haircut" className="w-13 shrink-0 text-right text-(--down)" />
							<span data-f="class" className="w-10.5 shrink-0 truncate text-right text-(--f3)" />
							<span data-f="notional" className="w-18.5 shrink-0 text-right text-(--acc)" />
						</div>
					))
				)}
			</div>

			<div className="mt-2.75 flex items-center gap-4 font-mono text-[9px] text-(--f3)">
				{([
					["var(--acc)", "notional sized"],
					["var(--down)", "flow haircut"],
					["var(--f4)", "scored only"],
				] as const).map(([color, label]) => (
					<span key={label} className="inline-flex items-center gap-1.25">
						<span className="h-2 w-2 rounded-full" style={{ background: color }} />
						{label}
					</span>
				))}
			</div>
		</div>
	);
};