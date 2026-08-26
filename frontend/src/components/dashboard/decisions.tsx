import { useSelector } from "@tanstack/react-store";
import { strategyStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	setDecisionsPendingFocus,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { Decision } from "#/providers/telemetry/telemetry/decision";

const decObj = new Decision();

export const Decisions = () => {
	const last = useSelector(strategyStore, (state) =>
		state.findLast((f) => f.decisionsLength() > 0),
	);

	const decisions: Array<{
		id: string;
		symbol: string;
		thesisScore: number;
		action: string;
		reason: string;
	}> = [];

	if (last) {
		for (let i = 0; i < last.decisionsLength(); i++) {
			const dec = last.decisions(i, decObj);
			if (!dec) continue;
			const id = dec.id() ?? `dec-${i}`;
			const symbol = dec.symbol() ?? "";
			if (!id) continue;
			decisions.push({
				id,
				symbol,
				thesisScore: dec.thesisScore(),
				action: dec.action() ?? "—",
				reason: dec.reason() ?? "No rejection reason published",
			});
		}
	}

	const inspectDecision = (symbol: string) => {
		setDecisionsScopeSymbol(symbol);
		setDecisionsPendingFocus(symbol);
		terminalStore.actions.openThesis(symbol);
	};

	return (
		<Flex.Column className="h-full min-h-0 gap-0">
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
					decisions.map((dec) => (
						<List.Item
							key={dec.id}
							className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)] hover:bg-(--raised)"
							data-decision-card="true"
							data-decision-id={dec.id}
							onClick={() => inspectDecision(dec.symbol)}
							title="Inspect MCTS / Pearl decision tree"
						>
							<Typography.Span className="truncate font-semibold text-[11px] text-(--f1)">
								{dec.symbol}
							</Typography.Span>
							<Flex.Row className="items-center gap-2">
								<Typography.Span className="text-[8.5px] text-(--f4)">
									t=
									<span className="tabular-nums text-(--f2)">
										{dec.thesisScore.toFixed(4)}
									</span>
								</Typography.Span>
								<Typography.Span className="rounded-[2px] border border-(--line) px-1.5 py-px text-[8.5px] uppercase">
									{dec.action}
								</Typography.Span>
							</Flex.Row>
							<Typography.Span className="col-span-2 mt-0.5 line-clamp-2 min-w-0 break-words text-[9px] leading-[1.25] text-(--f4)">
								{dec.reason}
							</Typography.Span>
						</List.Item>
					))
				)}
			</List>
		</Flex.Column>
	);
};


