import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { atoms } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import { confidenceAtom } from "#/collections/confidence";
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

const cashLabel = (book: Book): string => {
	switch (book) {
		case "virtual":
			return "Virtual cash";
		case "real":
			return "Live cash";
		default:
			return "Paper cash";
	}
};

const walletFor = (
	book: Book,
	wallets: {
		virtual?: BalanceReading | null;
		paper?: BalanceReading | null;
		real?: BalanceReading | null;
	},
	legacy: BalanceReading | null,
	realLive: boolean,
): BalanceReading | null => {
	switch (book) {
		case "virtual":
			return wallets.virtual ?? null;
		case "real":
			// REAL OFF / no live transport → never fall back to paper's bankroll.
			if (!realLive) return null;
			return wallets.real ?? null;
		default:
			return wallets.paper ?? legacy;
	}
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
Wallet readout follows the selected ledger tab: virtual, paper, or live.
Each figure is a primitive selector so sub-cent mark churn does not re-render.
*/
export const Balance = () => {
	const book = useSelector(bookAtom, (state) => state);
	const realLive = useSelector(
		confidenceAtom,
		(state) => state.real_ceiling === true,
	);
	const reading = (wallets: {
		virtual?: BalanceReading | null;
		paper?: BalanceReading | null;
		real?: BalanceReading | null;
	}) => walletFor(book, wallets, atoms.balance.get(), realLive);

	const cash = useSelector(atoms.balances, (wallets) =>
		fmt(field(reading(wallets), "cash")),
	);
	const unrealized = useSelector(atoms.balances, (wallets) =>
		fmt(field(reading(wallets), "unrealized")),
	);
	const equity = useSelector(atoms.balances, (wallets) =>
		fmt(field(reading(wallets), "equity")),
	);
	const profit = useSelector(atoms.balances, (wallets) =>
		inProfit(reading(wallets)),
	);
	const label =
		book === "real" && !realLive ? "Live cash (off)" : cashLabel(book);

	return (
		<Flex.Row align="center" gap={6} data-wallet="account" data-book={book}>
			<Reading
				label={label}
				tone="f1"
				weight="medium"
				which="cash"
				value={cash}
			/>
			<Reading
				label="Unrealized"
				tone="f2"
				weight="medium"
				which="unrealized"
				value={unrealized}
			/>
			<Reading
				label="Equity"
				tone="accent"
				weight="semibold"
				which="equity"
				value={equity}
			>
				{profit ? <Lambo /> : null}
			</Reading>
		</Flex.Row>
	);
};
