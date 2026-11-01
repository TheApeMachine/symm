import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { atoms } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import type { Balance as BalanceReading } from "#/collections/types";
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

const field = (balance: BalanceReading | null, key: keyof BalanceReading) =>
	balance?.[key];

const inProfit = (balance: BalanceReading | null) => {
	const unrealized = Number(field(balance, "unrealized"));
	return Number.isFinite(unrealized) && unrealized > 0;
};

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

/*
Wallet readout follows the selected ledger tab: paper wallet on paper/virtual,
live wallet on real.
*/
export const Balance = () => {
	const book = useSelector(bookAtom, (state) => state);
	const legacy = useSelector(atoms.balance, (state) => state);
	const wallets = useSelector(atoms.balances, (state) => state);
	const balance =
		book === "real"
			? (wallets.real ?? null)
			: (wallets.paper ?? legacy);
	const label = book === "real" ? "Live cash" : "Paper cash";

	return (
		<Flex.Row align="center" gap={6} data-wallet="account" data-book={book}>
			<Reading
				label={label}
				tone="f1"
				weight="medium"
				which="cash"
				value={fmt(field(balance, "cash"))}
			/>
			<Reading
				label="Unrealized"
				tone="f2"
				weight="medium"
				which="unrealized"
				value={fmt(field(balance, "unrealized"))}
			/>
			<Reading
				label="Equity"
				tone="accent"
				weight="semibold"
				which="equity"
				value={fmt(field(balance, "equity"))}
			>
				{inProfit(balance) ? <Lambo /> : null}
			</Reading>
		</Flex.Row>
	);
};
