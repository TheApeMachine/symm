import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

/*
XrayFactsPanel states what the classifier currently believes about the focused
symbol.

Cognition is published per symbol, so the panel scopes to the focused row and
paints its fields. Regime class and coherence come from the classifier itself;
surprisal and entropy describe how hard the sequence was to place. A classifier
that has not named a regime yet publishes an empty class, which reads as "none"
rather than as a panel that never received data.
*/
export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="cognition">
			{({ ref, className }) => (
				<div ref={ref} className={cn("flex h-full flex-col", className)}>
					<div
						data-scope="symbol"
						data-filter={focusSymbol}
						className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]"
					>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">regime class</span>
							<Typography.Span
								data-paint="winner"
								data-paint-absent="—"
								data-paint-empty="none named"
								className="text-right text-(--acc)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">coherence</span>
							<Typography.Span
								data-paint="confidence"
								data-paint-absent="—"
								data-paint-format=".1%"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">class contrast</span>
							<Typography.Span
								data-paint="contrast"
								data-paint-absent="—"
								data-paint-format=".3f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">entropy bits</span>
							<Typography.Span
								data-paint="entropyBits"
								data-paint-absent="—"
								data-paint-format=".3f"
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">classified</span>
							<Typography.Span
								data-paint="ready"
								data-paint-absent="—"
								data-paint-class="true:text-(--warn) false:text-(--f3)"
								className="text-right"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">ambiguous</span>
							<Typography.Span
								data-paint="ambiguous"
								data-paint-absent="—"
								data-paint-class="true:text-(--warn) false:text-(--f3)"
								className="text-right"
							>
								—
							</Typography.Span>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">sequence</span>
							<Typography.Span
								data-paint="sequence"
								data-paint-absent="—"
								data-paint-empty="none"
								className="max-w-42 truncate text-right text-(--f3) text-[10px]"
								title="DMT token sequence the classifier is reading"
							>
								—
							</Typography.Span>
						</div>
					</div>
				</div>
			)}
		</Component>
	);
};
