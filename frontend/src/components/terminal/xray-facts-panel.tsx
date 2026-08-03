import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

/*
XrayFactsPanel states what the classifier currently believes about the focused
symbol.

Cognition is published per symbol, so the panel selects the focused row and
paints its fields. Regime class and coherence come from the classifier itself;
surprisal and entropy describe how hard the sequence was to place.
*/
export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="cognition" select={focusSymbol}>
			{({ ref, className }) => (
				<div ref={ref} className={cn("flex h-full flex-col", className)}>
					<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">regime class</span>
							<Typography.Span
								data-paint="winnerRegime"
								className="text-right text-(--acc)"
							/>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">coherence</span>
							<Typography.Span
								data-paint="confidence"
								data-paint-format=".1%"
								className="text-right text-(--f1)"
							/>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">surprisal</span>
							<Typography.Span
								data-paint="surprisal"
								data-paint-format=".3f"
								className="text-right text-(--f1)"
							/>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">entropy bits</span>
							<Typography.Span
								data-paint="entropyBits"
								data-paint-format=".3f"
								className="text-right text-(--f1)"
							/>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">regime break</span>
							<Typography.Span
								data-paint="isBreak"
								data-paint-class="true:text-(--warn) false:text-(--f3)"
								className="text-right"
							/>
						</div>
						<div className="flex justify-between gap-3">
							<span className="text-(--f3)">ambiguous</span>
							<Typography.Span
								data-paint="ambiguity"
								data-paint-class="true:text-(--warn) false:text-(--f3)"
								className="text-right"
							/>
						</div>
					</div>
				</div>
			)}
		</Component>
	);
};
