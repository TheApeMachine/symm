import { useDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Panel } from "@/components/ui/panel";
import { causalStore, useSubscribe } from "#/providers/ws-stores";

export const LiveDecisionsEntryLine = () => {
	const scope = useDecisionsScopeSymbol();

	const root = useSubscribe(causalStore, (frames) => {
		const row = (frames ?? []).find((frame) => frame.symbol === scope) ?? null;

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		const f6 = (v: number | undefined) => (v === undefined ? "—" : v.toFixed(6));
		const pct = (v: number | undefined) => (v === undefined ? "—" : `${(v * 100).toFixed(1)}%`);

		set("baseline", f6(row?.entry_baseline));
		set("strength", f6(row?.strength));
		set("confidence", pct(row?.confidence));
	}, [scope]);

	if (scope === undefined) {
		return null;
	}

	return (
		<div ref={root}>
			<Panel className="mb-3.5 px-3 py-2 font-mono">
				<div className="mb-1.5 flex items-center justify-between gap-3">
					<span className="text-[10px] font-semibold text-(--f3) uppercase tracking-[0.13em]">
						causal evidence classification
					</span>
					<span className="text-[9px] text-(--f4)">standardized channels and classifier shares</span>
				</div>
				<div className="grid grid-cols-3 gap-2 text-[10px]">
					<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
						<div className="text-(--f4)">runner-up evidence share</div>
						<div data-f="baseline" className="mt-0.5 text-[12px] font-semibold text-(--acc)">—</div>
					</div>
					<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
						<div className="text-(--f4)">strongest standardized channel</div>
						<div data-f="strength" className="mt-0.5 text-[12px] text-(--f1)">—</div>
					</div>
					<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
						<div className="text-(--f4)">winning evidence share</div>
						<div data-f="confidence" className="mt-0.5 text-[12px] text-(--info)">—</div>
					</div>
				</div>
			</Panel>
		</div>
	);
};
