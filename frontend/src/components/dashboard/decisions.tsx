import { useSelector } from "@tanstack/react-store";
import { terminalStore } from "#/collections/terminal";
import {
	setDecisionsPendingFocus,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { Flex } from "@/components/ui/flex";
import { strategyStore, useSubscribe } from "#/providers/ws-stores";

export const Decisions = () => {
	const root = useSubscribe(strategyStore, (state) => {
		const decisions = state?.decisions ?? [];

		for (const decision of decisions) {
			const cell = root.current?.querySelector<HTMLElement>(`[data-decision-id="${decision.id}"]`);

			if (cell === null || cell === undefined) {
				continue;
			}

			const set = (q: string, value: string) => {
				const el = cell.querySelector<HTMLElement>(`[data-df="${q}"]`);

				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			};

			set("symbol", decision.symbol);
			set("thesisScore", typeof decision.thesisScore === "number" ? decision.thesisScore.toFixed(4) : "—");
			set("action", decision.action);
			set("reason", decision.reason ?? "No rejection reason published");
		}
	});

	const decisions = useSelector(strategyStore, (state) => state?.decisions ?? []);

	const inspectDecision = (symbol: string) => {
		setDecisionsScopeSymbol(symbol);
		setDecisionsPendingFocus(symbol);
		terminalStore.actions.openThesis(symbol);
	};

	return (
		<Flex.Column ref={root} className="h-full min-h-0 gap-0">
			<Flex.Row
				align="baseline"
				justify="between"
				padding={2}
				className="border-(--line) border-b"
			>
				<Typography.Span semibold uppercase tracking="0.13em">
					DECISIONS
				</Typography.Span>
			</Flex.Row>
			<List className="min-h-0 flex-1 gap-1 overflow-auto p-2">
				{decisions.length === 0 ? (
					<List.Item className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 px-2.5 py-1.5 font-mono text-[11px] text-(--f4)">
						waiting for backend decision frames
					</List.Item>
				) : (
					decisions.map((decision) => (
						<List.Item
							key={decision.id}
							className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)] hover:bg-(--raised)"
							data-decision-card="true"
							data-decision-id={decision.id}
							onClick={() => inspectDecision(decision.symbol)}
							title="Inspect MCTS / Pearl decision tree"
						>
							<Typography.Span data-df="symbol" className="truncate font-semibold text-[11px] text-(--f1)" />
							<Flex.Row className="items-center gap-2">
								<Typography.Span className="text-[8.5px] text-(--f4)">
									t=
									<span data-df="thesisScore" className="tabular-nums text-(--f2)" />
								</Typography.Span>
								<Typography.Span data-df="action" className="rounded-[2px] border border-(--line) px-1.5 py-px text-[8.5px] uppercase" />
							</Flex.Row>
							<Typography.Span data-df="reason" className="col-span-2 mt-0.5 line-clamp-2 min-w-0 break-words text-[9px] leading-[1.25] text-(--f4)" />
						</List.Item>
					))
				)}
			</List>
		</Flex.Column>
	);
};
