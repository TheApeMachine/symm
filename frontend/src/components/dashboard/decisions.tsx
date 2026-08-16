import { useNavigate } from "@tanstack/react-router";
import type { MouseEvent } from "react";
import {
	setDecisionsPendingFocus,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";

const slotCount = 6;
const decisionSlotKeys = Array.from(
	{ length: slotCount },
	(_, index) => `decision-slot-${index + 1}`,
);

export const Decisions = () => {
	const navigate = useNavigate();

	/*
		The decision tree page scopes and expands by symbol, not by row identity,
		so a click here pins the symbol both ways: as the persistent side-rail
		scope and as a one-shot focus the matching DecisionChain claims and
		expands itself the moment it next paints that symbol.
	*/
	const openInDecisionTree = (event: MouseEvent<HTMLDivElement>) => {
		const symbol = event.currentTarget.querySelector<HTMLElement>(
			"[data-paint='symbol']",
		)?.textContent;

		if (!symbol) {
			return;
		}

		setDecisionsScopeSymbol(symbol);
		setDecisionsPendingFocus(symbol);
		navigate({ to: "/decisions" });
	};

	return (
		<Component registerKey="strategy" select="decisions">
			{({ ref, className }) => (
				<Flex.Column
					ref={ref}
					className={cn("h-full min-h-0 gap-0", className)}
				>
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
						{decisionSlotKeys.map((slotKey, index) => (
							<List.Item
								key={slotKey}
								className="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-0 rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)] hover:bg-(--raised)"
								data-decision-card="true"
								data-index={index}
								onClick={openInDecisionTree}
								title="Open in decision tree"
							>
								<Typography.Span
									data-paint="symbol"
									className="truncate font-semibold text-[11px] text-(--f1)"
								/>
								<Flex.Row className="items-center gap-2">
									<Typography.Span className="text-[8.5px] text-(--f4)">
										t=
										<span
											data-paint="thesisScore"
											data-paint-format=".4f"
											className="tabular-nums text-(--f2)"
										/>
									</Typography.Span>
									<Typography.Span
										data-paint="action"
										data-paint-class="enter:text-(--up) exit:text-(--down) hold:text-(--warn) nothing:text-(--acc)"
										className="rounded-[2px] border border-(--line) px-1.5 py-px text-[8.5px] uppercase"
									/>
								</Flex.Row>
								<Typography.Span
									data-paint="reason"
									data-paint-empty="No rejection reason published"
									className="col-span-2 mt-0.5 line-clamp-2 min-w-0 break-words text-[9px] leading-[1.25] text-(--f4)"
								/>
							</List.Item>
						))}
					</List>
				</Flex.Column>
			)}
		</Component>
	);
};
