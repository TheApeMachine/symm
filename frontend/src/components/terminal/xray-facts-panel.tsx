import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import { cognitionStore, useSubscribe } from "#/providers/ws-stores";

export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	const root = useSubscribe(cognitionStore, (state) => {
		const row = state.cognition[focusSymbol]?.latest() ?? null;

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("winner", row?.winner === undefined || row.winner === "" ? "none named" : String(row.winner));
		set("confidence", row?.confidence === undefined ? "—" : `${(row.confidence * 100).toFixed(1)}%`);
		set("contrast", row?.contrast === undefined ? "—" : row.contrast.toFixed(3));
		set("entropy", row?.entropyBits === undefined ? "—" : row.entropyBits.toFixed(3));
		set("ambiguous", row?.ambiguous === undefined ? "—" : String(row.ambiguous));
		set("sequence", row?.sequence === undefined || row.sequence === "" ? "none" : String(row.sequence));
	}, [focusSymbol]);

	return (
		<div ref={root} className="flex h-full flex-col">
			<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">regime class</span>
					<Typography.Span data-f="winner" className="text-right text-(--acc)">—</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">coherence</span>
					<Typography.Span data-f="confidence" className="text-right text-(--f1)">—</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">class contrast</span>
					<Typography.Span data-f="contrast" className="text-right text-(--f1)">—</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">entropy bits</span>
					<Typography.Span data-f="entropy" className="text-right text-(--f1)">—</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">ambiguous</span>
					<Typography.Span data-f="ambiguous" className="text-right">—</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">sequence</span>
					<Typography.Span data-f="sequence" className="max-w-42 truncate text-right text-(--f3) text-[10px]" title="DMT token sequence the classifier is reading">—</Typography.Span>
				</div>
			</div>
		</div>
	);
};
