import { useSelector } from "@tanstack/react-store";
import { equityStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

const fmt = (value: unknown): string =>
	typeof value === "number"
		? value.toFixed(2)
		: typeof value === "string" && value.trim() !== "" && Number.isFinite(Number(value))
			? Number(value).toFixed(2)
			: "—";

const Reading = ({
	label,
	tone,
	weight,
	which,
	value,
}: {
	label: string;
	tone: "f1" | "f2" | "accent";
	weight: "medium" | "semibold";
	which: string;
	value: string;
}) => (
	<Flex.Column className="items-end gap-px">
		<Typography.Label size="s" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono size="lg" tone={tone} weight={weight} data-balance={which}>
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
			/>
		</Flex.Row>
	);
};


