import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { cashAtom, equityAtom, unrealizedAtom } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

const fmt = (value: unknown): string => {
	if (typeof value === "number") {
		return value.toFixed(2);
	}

	if (
		typeof value === "string" &&
		value.trim() !== "" &&
		Number.isFinite(Number(value))
	) {
		return Number(value).toFixed(2);
	}

	return "—";
};

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
		alt="when moon?"
		title="when lambo?"
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
	tone: "f1" | "f2" | "f3" | "accent";
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
	const cash = useSelector(cashAtom, (state) => state);
	const unrealized = useSelector(unrealizedAtom, (state) => state);
	const equity = useSelector(equityAtom, (state) => state);

	const unrealizedVal = Number(unrealized);
	const inProfit = Number.isFinite(unrealizedVal) && unrealizedVal > 0;

	return (
		<Flex.Row
			align="center"
			gap={6}
			data-wallet="account"
		>
			<Reading
				label="Cash"
				tone="f1"
				weight="medium"
				which="cash"
				value={fmt(cash)}
			/>
			<Reading
				label="Unrealized"
				tone="f2"
				weight="medium"
				which="unrealized"
				value={fmt(unrealized)}
			/>
			<Reading
				label="Equity"
				tone="accent"
				weight="semibold"
				which="equity"
				value={fmt(equity)}
			>
				{inProfit ? <Lambo /> : null}
			</Reading>
		</Flex.Row>
	);
};
