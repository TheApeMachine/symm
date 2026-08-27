import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { cognitionStore, focusStore } from "#/collections/app";
import {
	getRetainedCognition,
	retainCognitionRow,
} from "#/components/terminal/xray-view";
import { Typography } from "#/components/ui/typography";
import { Cognition } from "#/providers/telemetry/telemetry/cognition";

const cogObj = new Cognition();

export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const updateFromState = (state: typeof cognitionStore.state) => {
			if (!root.current) return;
			const last = state.getLast();
			if (last) {
				for (let i = 0; i < last.rowsLength(); i++) {
					const row = last.rows(i, cogObj);
					const symbol = row?.symbol();
					if (row && typeof symbol === "string") {
						retainCognitionRow(symbol, {
							winner: row.winner() ?? undefined,
							confidence: row.confidence(),
							contrast: row.contrast(),
							entropyBits: row.entropyBits(),
							ambiguous: row.ambiguous(),
							sequence: row.sequence() ?? undefined,
						});
					}
				}
			}

			const targetRow = getRetainedCognition(focusSymbol);

			const set = (q: string, value: string) => {
				const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);
				if (el) el.textContent = value;
			};

			if (targetRow) {
				const winnerVal = (targetRow.winner as string | undefined) || "none named";
				set("winner", winnerVal);

				const conf = targetRow.confidence as number | undefined;
				if (typeof conf === "number" && Number.isFinite(conf)) {
					set("confidence", `${(conf * 100).toFixed(1)}%`);
				}

				const contrast = targetRow.contrast as number | undefined;
				if (typeof contrast === "number" && Number.isFinite(contrast)) {
					set("contrast", contrast.toFixed(3));
				}

				const entropy = targetRow.entropyBits as number | undefined;
				if (typeof entropy === "number" && Number.isFinite(entropy)) {
					set("entropy", entropy.toFixed(3));
				}

				const amb = targetRow.ambiguous;
				if (typeof amb === "boolean") {
					set("ambiguous", String(amb));
				}

				const seq = (targetRow.sequence as string | undefined) || "none";
				set("sequence", seq);
			}
		};

		updateFromState(cognitionStore.state);
		const subscription = cognitionStore.subscribe((state) => {
			updateFromState(state);
		});

		return () => {
			subscription.unsubscribe();
		};
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

