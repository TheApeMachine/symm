import { strategyStore, useDecisions, useSubscribe } from "#/providers/ws-stores";
import { num } from "./number";

export const AllocationMain = () => {
	const decisions = useDecisions();

	const root = useSubscribe(strategyStore, (state) => {
		const frameDecisions = state?.decisions ?? [];

		for (let index = 0; index < frameDecisions.length; index += 1) {
			const decision = frameDecisions[index];

			if (decision === undefined) {
				continue;
			}

			const row = root.current?.querySelector<HTMLElement>(`[data-i="${index}"]`);

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
			set("thesisScore", decision.thesisScore.toFixed(5));
			set("action", decision.action);
			set("haircut", `${(decision.allocation_haircut * 100).toFixed(1)}%`);
			set("class", decision.allocationClass || "—");
			set("notional", num(decision.proposedNotional, 2));

			const bar = row.querySelector<HTMLElement>("[data-conf]");

			if (bar instanceof HTMLElement) {
				const v = decision.confidence;
				bar.style.width = `clamp(0%, calc(${typeof v === "number" ? v : 0} * 100%), 100%)`;
			}
		}
	});

	return (
		<div ref={root} className="min-h-0 overflow-auto px-4.5 py-4">
			<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
				<span className="text-(--f3)">cross-section</span>
				<span className="text-(--f4)">
					candidates{" "}
					<span className="text-(--f2)">{decisions.length}</span>
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
				{decisions.length === 0 ? (
					<div className="py-24 text-center font-mono text-[11px] text-(--f4)">
						waiting for backend decision frames
					</div>
				) : (
					decisions.map((decision, index) => (
						<div
							key={decision.id}
							data-i={index}
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