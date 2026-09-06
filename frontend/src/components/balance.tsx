import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { equityStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

const fmt = (value: unknown): string =>
	typeof value === "number"
		? value.toFixed(2)
		: typeof value === "string" &&
				value.trim() !== "" &&
				Number.isFinite(Number(value))
			? Number(value).toFixed(2)
			: "—";

/*
The lambo rides behind the equity reading whenever the book is in unrealized
profit, and is simply absent otherwise.

It is decoration with a real job: an ambient state you catch from across the
room without reading a digit. That only works if it stays ambient — behind the
number at 60% and out of the pointer's way — so it is never allowed to compete
with the figure it sits behind. It is aria-hidden because the number is already
the accessible statement of the same fact.
*/
const Lambo = () => (
	<img
		src="/lambo.png"
		alt=""
		aria-hidden="true"
		className="pointer-events-none absolute -top-1.5 right-0 z-0 h-11 opacity-60"
	/>
);

const Reading = ({
	label,
	tone,
	weight,
	which,
	value,
	children,
}: {
	label: string;
	tone: "f1" | "f2" | "accent";
	weight: "medium" | "semibold";
	which: string;
	value: string;
	children?: ReactNode;
}) => (
	<Flex.Column className="relative items-end gap-px">
		{children}
		<Typography.Label
			size="s"
			tone="f4"
			weight="normal"
			className="relative z-1"
		>
			{label}
		</Typography.Label>
		<Typography.Mono
			size="lg"
			tone={tone}
			weight={weight}
			data-balance={which}
			className="relative z-1"
		>
			{value}
		</Typography.Mono>
	</Flex.Column>
);

export const Balance = () => {
	const lastWithCash = useSelector(equityStore, (state) =>
		state.findLast((f) => f.cash() !== null && f.cash() !== ""),
	);
	const lastWithUnrealized = useSelector(equityStore, (state) =>
		state.findLast((f) => f.unrealized() !== null && f.unrealized() !== ""),
	);
	const lastWithEquity = useSelector(equityStore, (state) =>
		state.findLast((f) => f.equity() !== null && f.equity() !== ""),
	);

	/*
		Profit is unrealized rather than equity: the ride is for the book being up
		right now, not for the account being larger than nothing. Equity is above
		zero the moment the wallet is funded, which would leave it permanently on.
	*/
	const unrealized = Number(lastWithUnrealized?.unrealized());
	const inProfit = Number.isFinite(unrealized) && unrealized > 0;

	return (
		<Flex.Row align="center" gap={6}>
			<Reading
				label="Cash"
				tone="f1"
				weight="medium"
				which="cash"
				value={fmt(lastWithCash?.cash())}
			/>
			<Reading
				label="Unrealized"
				tone="f2"
				weight="medium"
				which="unrealized"
				value={fmt(lastWithUnrealized?.unrealized())}
			/>
			<Reading
				label="Equity"
				tone="accent"
				weight="semibold"
				which="equity"
				value={fmt(lastWithEquity?.equity())}
			>
				{inProfit ? <Lambo /> : null}
			</Reading>
		</Flex.Row>
	);
};
