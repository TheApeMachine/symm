import { useRef, useState } from "react";
import { positionStore } from "#/collections/app";
import { Panel } from "#/components/ui/panel";
import { Section } from "#/components/ui/section";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";

type PositionEntry = {
	symbol: string;
	status: string;
};

type LifecycleQueryEntry = {
	symbol: HTMLElement | null;
	status: HTMLElement | null;
};

const lifecycleQueryCache: Record<string, LifecycleQueryEntry> = {};
const posObj = new Position();
const holdObj = new Holding();


/*
JournalSurface is the record of what the desk actually did.
*/
export const JournalSurface = () => {
	const lifecycle = useRef<HTMLDivElement>(null);
	const [openSymbols, setOpenSymbols] = useState<PositionEntry[]>([]);

	positionStore.subscribe((state) => {
		if (!lifecycle.current) return;
		const last = state.getLast();
		if (!last) return;

		const currentOpen: PositionEntry[] = [];

		for (let i = 0; i < last.rowsLength(); i++) {
			const pos = last.rows(i, posObj);
			if (!pos) continue;
			const holding = pos.holding(holdObj);
			const sym = holding?.symbol() ?? "";
			const status = holding?.status() ?? pos.status() ?? "closed";

			if (status === "open" && sym) {
				currentOpen.push({ symbol: sym, status });


				let element = lifecycleQueryCache[sym];
				if (!element) {
					const cell = lifecycle.current.querySelector<HTMLElement>(`[data-life="${sym}"]`);
					if (!cell) continue;

					element = {
						symbol: cell.querySelector<HTMLElement>("[data-lf=symbol]"),
						status: cell.querySelector<HTMLElement>("[data-lf=status]"),
					};
					lifecycleQueryCache[sym] = element;
				}

				if (element.symbol) element.symbol.textContent = sym;
				if (element.status) element.status.textContent = status;
			}
		}

		if (currentOpen.map((p) => p.symbol).join(",") !== openSymbols.map((p) => p.symbol).join(",")) {
			setOpenSymbols(currentOpen);
		}
	});

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(280px,320px)_minmax(420px,1fr)]">
			<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
				<div ref={lifecycle}>
					<Section>
						<Section.Header title="Lifecycle rail" meta={`${openSymbols.length} lots`} />
						<div className="flex flex-col gap-2 p-2">
							{openSymbols.length === 0 ? (
								<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
									no lots held
								</Panel>
							) : (
								openSymbols.map((pos) => (
									<Panel key={pos.symbol} variant="surface" size="bare" data-life={pos.symbol} className="flex items-center justify-between gap-2 px-2.5 py-2 font-mono text-[11px]">
										<span data-lf="symbol" className="truncate text-(--f1)" />
										<span data-lf="status" className="shrink-0 text-[9px] uppercase tracking-wide" />
									</Panel>
								))
							)}
						</div>
					</Section>
				</div>
			</div>

			<div className="min-h-0 overflow-auto px-4 py-4.5">
				<Section.Header size="bare" rule={false} title="Journal" meta="0 entries" />
				<div className="mt-2 flex flex-col gap-2">
					<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
						nothing traded yet this run
					</Panel>
				</div>
			</div>
		</div>
	);
};