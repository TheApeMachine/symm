import type { Position } from "#/collections/types";
import { Panel } from "@/components/ui/panel";
import { Section } from "@/components/ui/section";
import { journalStore, positionsStore, useSubscribe } from "#/providers/ws-stores";

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : "—";
const time = (value: unknown): string =>
	typeof value === "string" && value !== "" ? new Date(value).toISOString().slice(11, 19) : "—";

const openPositions = (state: typeof positionsStore.state): Position[] =>
	Object.values(state.positions)
		.map((buffer) => buffer.latest())
		.filter((row): row is Position => row !== undefined && row.status === "open");

/*
JournalSurface is the record of what the desk actually did.
*/
export const JournalSurface = () => {
	const lifecycle = useSubscribe(positionsStore, (state) => {
		const open = openPositions(state);

		for (const position of open) {
			const cell = lifecycle.current?.querySelector<HTMLElement>(`[data-life="${position.holding.symbol}"]`);

			if (cell === null || cell === undefined) {
				continue;
			}

			const symbol = cell.querySelector<HTMLElement>("[data-lf=symbol]");
			const status = cell.querySelector<HTMLElement>("[data-lf=status]");

			if (symbol instanceof HTMLElement) symbol.textContent = position.holding.symbol;
			if (status instanceof HTMLElement) status.textContent = position.holding.status ?? "—";
		}
	});

	const journal = useSubscribe(journalStore, (state) => {
		const entries = state.journal.values();

		for (const entry of entries) {
			const identity = entry.decision?.id ?? entry.holding.symbol;
			const cell = journal.current?.querySelector<HTMLElement>(`[data-entry="${identity}"]`);

			if (cell === null || cell === undefined) {
				continue;
			}

			const set = (q: string, value: string) => {
				const el = cell.querySelector<HTMLElement>(`[data-jf="${q}"]`);

				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			};

			set("symbol", entry.holding.symbol);
			set("pnl", `${fmt(entry.holding.pnl, 4)} USD`);
			set("opened", time(entry.holding.entry_at));
			set("closed", time(entry.holding.exit_at));
			set("entry", fmt(entry.holding.entry_price, 6));
			set("exit", fmt(entry.holding.exit_price, 6));
			set("qty", fmt(entry.holding.qty, 6));
			set("return", fmt(entry.holding.return_pct, 2));
			set("entryFee", fmt(entry.holding.entry_fee, 4));
			set("exitFee", fmt(entry.holding.exit_fee, 4));
			set("trigger", entry.holding.stoploss?.trigger_reason ?? "—");
			set("cause", entry.decision?.cause ?? "—");
			set("floor", fmt(entry.holding.stoploss?.floor, 6));
			set("peak", fmt(entry.holding.stoploss?.peak, 6));
			set("profit", fmt(entry.holding.stoploss?.profit_line, 6));
			set("locked", String(entry.holding.stoploss?.locked ?? false));
			set("thesis", fmt(entry.decision?.thesisScore, 4));
			set("graph", fmt(entry.decision?.graphScore, 4));
			set("sims", String(entry.decision?.trace?.mcts?.iterations ?? "—"));
			set("recommended", entry.decision?.trace?.mcts?.recommendedAction ?? "—");
		}
	});

	const open = openPositions(positionsStore.state);
	const entries = journalStore.state.journal.values();

	return (
		<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(280px,320px)_minmax(420px,1fr)]">
			<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
				<div ref={lifecycle}>
					<Section>
						<Section.Header title="Lifecycle rail" meta={`${open.length} lots`} />
						<div className="flex flex-col gap-2 p-2">
							{open.length === 0 ? (
								<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
									no lots held
								</Panel>
							) : (
								open.map((position) => (
									<Panel key={position.holding.symbol} variant="surface" size="bare" data-life={position.holding.symbol} className="flex items-center justify-between gap-2 px-2.5 py-2 font-mono text-[11px]">
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
				<div ref={journal}>
					<Section.Header size="bare" rule={false} title="Journal" meta={`${entries.length} entries`} />
					<div className="mt-2 flex flex-col gap-2">
						{entries.length === 0 ? (
							<Panel variant="surface" size="bare" className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
								nothing traded yet this run
							</Panel>
						) : (
							entries.map((entry) => {
								const identity = entry.decision?.id ?? entry.holding.symbol;

								return (
									<Panel key={identity} variant="surface" size="bare" data-entry={identity} className="px-3 py-2.5 font-mono text-[11px]">
										<div className="flex items-center justify-between gap-2">
											<span data-jf="symbol" className="truncate font-semibold text-(--f1)" />
											<span data-jf="pnl" className="shrink-0 font-semibold text-(--pnl)" />
										</div>
										<div className="mt-1 grid grid-cols-2 gap-x-3 gap-y-0.5 text-[9.5px] text-(--f4)">
											<span>opened <b className="font-normal text-(--f3)" data-jf="opened" /></span>
											<span className="text-right">closed <b className="font-normal text-(--f3)" data-jf="closed" /></span>
											<span>entry <b className="font-normal text-(--f3)" data-jf="entry" /></span>
											<span className="text-right">exit <b className="font-normal text-(--f3)" data-jf="exit" /></span>
											<span>qty <b className="font-normal text-(--f3)" data-jf="qty" /></span>
											<span className="text-right">return <b className="font-normal text-(--pnl)" data-jf="return" />%</span>
											<span>entry fee <b className="font-normal text-(--f3)" data-jf="entryFee" /></span>
											<span className="text-right">exit fee <b className="font-normal text-(--f3)" data-jf="exitFee" /></span>
										</div>
										<div className="mt-1.5 flex items-center justify-between gap-2 border-t border-(--line) pt-1.5 text-[9.5px]">
											<span className="text-(--f4)">exit <b data-jf="trigger" className="font-semibold" /></span>
											<span className="text-(--f4)">cause <b data-jf="cause" className="font-normal text-(--f3)" /></span>
										</div>
										<div className="mt-1 grid grid-cols-4 gap-x-3 text-[9px] text-(--f4)">
											<span>floor <b className="font-normal text-(--f3)" data-jf="floor" /></span>
											<span>peak <b className="font-normal text-(--f3)" data-jf="peak" /></span>
											<span>profit <b className="font-normal text-(--f3)" data-jf="profit" /></span>
											<span>locked <b data-jf="locked" className="font-normal" /></span>
										</div>
										<div className="mt-1.5 border-t border-(--line) pt-1.5">
											<div className="flex items-center justify-between text-[9px] text-(--f4)">
												<span>thesis <b className="font-normal text-(--f2)" data-jf="thesis" /></span>
												<span>graph <b className="font-normal text-(--f2)" data-jf="graph" /></span>
												<span><b className="font-normal text-(--f3)" data-jf="sims" /> sims → <b data-jf="recommended" className="font-semibold text-(--acc)" /></span>
											</div>
										</div>
									</Panel>
								);
							})
						)}
					</div>
				</div>
			</div>
		</div>
	);
};