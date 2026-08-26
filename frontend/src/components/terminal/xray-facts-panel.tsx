import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { cognitionStore, focusStore } from "#/collections/app";
import { Typography } from "#/components/ui/typography";
import { Cognition } from "#/providers/telemetry/telemetry/cognition";

const cogObj = new Cognition();

export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	cognitionStore.subscribe((state) => {
		if (!root.current) return;
		const last = state.getLast();
		if (!last) return;

		let targetRow: Cognition | null = null;
		for (let i = 0; i < last.rowsLength(); i++) {
			const row = last.rows(i, cogObj);
			if (row && row.symbol() === focusSymbol) {
				targetRow = row;
				break;
			}
		}

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);
			if (el) el.textContent = value;
		};

		set("winner", targetRow?.winner() || "none named");
		set("confidence", targetRow ? `${(targetRow.confidence() * 100).toFixed(1)}%` : "—");
		set("contrast", targetRow ? targetRow.contrast().toFixed(3) : "—");
		set("entropy", targetRow ? targetRow.entropyBits().toFixed(3) : "—");
		set("ambiguous", targetRow ? String(targetRow.ambiguous()) : "—");
		set("sequence", targetRow?.sequence() || "none");
	});

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

