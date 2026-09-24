import type { ComponentProps, ReactNode } from "react";
import { cn } from "#/lib/utils";
import { Flex } from "./flex";
import { basis } from "./learning-format";

/*
SkillActivity is one completed decision as the activity log shows it: when it
closed, what it was, and what it did to the wallet.
*/
export interface SkillActivity {
	id: string;
	time?: string;
	message: string;
	pnl?: number;
}

export type SkillPanelProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	title?: string;
	/** Completed evaluations so far. */
	evaluated?: number;
	/** Mean completed-decision benefit as a fraction of what was committed. */
	edge?: number;
	positive?: number;
	negative?: number;
	/** The wallet as it stands, in its quote currency. */
	balance?: number;
	/** Net change of the wallet, fees included. */
	pnl?: number;
	currency?: string;
	activity?: SkillActivity[] | null;
};

const number = (value: number | string | null | undefined) => {
	if (value === undefined || value === null || value === "") return undefined;
	const parsed = typeof value === "number" ? value : Number(value);
	return Number.isFinite(parsed) ? parsed : undefined;
};

const money = (value: number | undefined, digits = 2) =>
	value === undefined
		? "—"
		: value.toLocaleString(undefined, {
				minimumFractionDigits: digits,
				maximumFractionDigits: digits,
			});

const signed = (value: number | undefined, digits = 4) =>
	value === undefined ? "—" : `${value > 0 ? "+" : ""}${value.toFixed(digits)}`;

/* clock writes a timestamp as the time of day it names; other text as given. */
const clock = (time: string) =>
	/^\d{4}-\d{2}-\d{2}T/.test(time)
		? new Date(time).toLocaleTimeString(undefined, { hour12: false })
		: time;

const tone = (value: number | undefined) =>
	value === undefined
		? "text-(--f2)"
		: value > 0
			? "text-(--up)"
			: value < 0
				? "text-(--down)"
				: "text-(--f2)";

const Block = ({
	label,
	children,
	note,
}: {
	label: string;
	children: ReactNode;
	note?: string;
}) => (
	<div>
		<div className="mb-2 text-[10px] uppercase tracking-widest text-(--f4)">
			{label}
		</div>
		{children}
		{note && (
			<div className="mt-1 text-[10px] leading-tight text-(--f4)">{note}</div>
		)}
	</div>
);

/*
SkillPanel is the learner's scorecard: how its completed decisions went, and
what they did to the one wallet it trades from.
*/
export const SkillPanel = ({
	title = "Agent Skill",
	evaluated,
	edge,
	positive,
	negative,
	balance,
	pnl,
	currency = "USD",
	activity,
	className,
	...props
}: SkillPanelProps) => {
	const wins = number(positive) ?? 0;
	const losses = number(negative) ?? 0;
	const total = wins + losses;
	const benefit = number(edge);
	const wallet = number(balance);
	const change = number(pnl);

	return (
		<Flex.Column
			className={cn(
				"min-h-0 rounded border border-(--line) bg-(--surface) font-mono text-[11px] text-(--f3)",
				className,
			)}
			{...props}
		>
			<Flex.Row className="h-8 shrink-0 items-center justify-between border-(--line) border-b bg-(--sunken) px-3">
				<span className="uppercase tracking-widest">{title}</span>
				<span>{number(evaluated)?.toLocaleString() ?? 0} evaluated</span>
			</Flex.Row>
			<Flex.Column className="min-h-0 flex-1 gap-5 overflow-y-auto p-4">
				<Block
					label="Mean completed decision benefit"
					note="Completed round trips as a fraction of what each committed. Negative outcomes remain negative."
				>
					<div className={cn("text-base", tone(benefit))}>
						{benefit === undefined ? "—" : basis(benefit)}
					</div>
				</Block>

				<Block label="Outcome signs">
					<div className="mb-2 flex items-baseline gap-2">
						<span className="text-(--up)">{wins} positive</span>
						<span className="text-(--f4)">·</span>
						<span className="text-(--down)">{losses} negative</span>
					</div>
					<div className="flex h-1.5 w-full overflow-hidden rounded-full bg-(--raised)">
						<div
							className="bg-(--up)"
							style={{ width: total ? `${(wins / total) * 100}%` : "0%" }}
						/>
						<div
							className="bg-(--down)"
							style={{ width: total ? `${(losses / total) * 100}%` : "0%" }}
						/>
					</div>
				</Block>

				<Block
					label="Wallet"
					note="The radix trie's one paper wallet, settled against the recorded book, fees included."
				>
					<div className="text-base text-(--f1)">
						{money(wallet)} <span className="text-(--f4)">{currency}</span>
					</div>
					<div className={cn("mt-1", tone(change))}>
						P&amp;L {signed(change)}
					</div>
				</Block>

				<div className="border-(--line) border-t pt-4">
					<div className="mb-3 text-[10px] uppercase tracking-widest text-(--f4)">
						Recent learning activity
					</div>
					{!activity?.length && (
						<div className="text-[10px] text-(--f4)">
							No completed decisions
						</div>
					)}
					<Flex.Column className="gap-3">
						{activity?.map((entry) => {
							const change = number(entry.pnl);
							return (
								<div key={entry.id} className="text-[10px]">
									<div className="mb-0.5 truncate text-(--f2)">
										{entry.time ? `${clock(entry.time)} · ` : ""}
										{entry.message}
									</div>
									<div className="text-(--f3)">
										Wallet P&amp;L{" "}
										<span className={tone(change)}>{signed(change)}</span>
									</div>
								</div>
							);
						})}
					</Flex.Column>
				</div>
			</Flex.Column>
		</Flex.Column>
	);
};
