import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { focusAtom, signals } from "#/collections/app";
import { Typography } from "#/components/ui/typography";

export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(focusAtom);
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const updateFromState = () => {
			if (!root.current) return;

			const targetRow = signals.cognition.state[focusSymbol]?.getLast();

			const set = (q: string, value: string) => {
				const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);
				if (el) el.textContent = value;
			};

			// Reset every field first so a missing row cannot preserve the
			// previously focused symbol's readout.
			for (const field of [
				"winner",
				"confidence",
				"contrast",
				"entropy",
				"ambiguous",
				"sequence",
			]) {
				set(field, "—");
			}

			if (targetRow) {
				const metricMap = new Map<string, number>();
				for (const m of targetRow.metrics) {
					const name = typeof m.name === "string" ? m.name : "";
					if (name) {
						metricMap.set(name, m.hasNormalized ? m.normalized : m.raw);
					}
				}

				const contrastVal = metricMap.get("contrast");
				const surprisalVal = metricMap.get("surprisal");
				const stabilityVal = metricMap.get("stability");
				const ambiguityVal = metricMap.get("ambiguity");

				set("winner", focusSymbol);
				set(
					"confidence",
					stabilityVal !== undefined
						? `${(stabilityVal * 100).toFixed(1)}%`
						: "—",
				);
				set(
					"contrast",
					contrastVal !== undefined ? contrastVal.toFixed(3) : "—",
				);
				set(
					"entropy",
					surprisalVal !== undefined ? surprisalVal.toFixed(3) : "—",
				);
				set(
					"ambiguous",
					ambiguityVal !== undefined
						? ambiguityVal > 0.5
							? "true"
							: "false"
						: "—",
				);
				const sym =
					typeof targetRow.symbol === "string"
						? targetRow.symbol
						: "";
				set("sequence", sym || "none");
			}
		};

		updateFromState();
		const subscription = signals.cognition.subscribe(updateFromState);

		return () => {
			subscription.unsubscribe();
		};
	}, [focusSymbol]);

	return (
		<div ref={root} className="flex h-full flex-col">
			<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">regime class</span>
					<Typography.Span data-f="winner" className="text-right text-(--acc)">
						—
					</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">coherence</span>
					<Typography.Span
						data-f="confidence"
						className="text-right text-(--f1)"
					>
						—
					</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">class contrast</span>
					<Typography.Span data-f="contrast" className="text-right text-(--f1)">
						—
					</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">entropy bits</span>
					<Typography.Span data-f="entropy" className="text-right text-(--f1)">
						—
					</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">ambiguous</span>
					<Typography.Span data-f="ambiguous" className="text-right">
						—
					</Typography.Span>
				</div>
				<div className="flex justify-between gap-3">
					<span className="text-(--f3)">sequence</span>
					<Typography.Span
						data-f="sequence"
						className="max-w-42 truncate text-right text-(--f3) text-[10px]"
						title="DMT token sequence the classifier is reading"
					>
						—
					</Typography.Span>
				</div>
			</div>
		</div>
	);
};
