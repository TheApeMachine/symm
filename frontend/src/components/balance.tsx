import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { equityStore, useSubscribe } from "#/providers/ws-stores";

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
}: {
	label: string;
	tone: "f1" | "f2" | "accent";
	weight: "medium" | "semibold";
	which: string;
}) => (
	<Flex.Column className="items-end gap-px">
		<Typography.Label size="s" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono size="lg" tone={tone} weight={weight} data-balance={which}>
			—
		</Typography.Mono>
	</Flex.Column>
);

export const Balance = () => {
	const root = useSubscribe(equityStore, (state) => {
		const record =
			state !== null && typeof state === "object"
				? (state as Record<string, unknown>)
				: null;

		for (const which of ["cash", "unrealized", "equity"]) {
			const el = root.current?.querySelector<HTMLElement>(
				`[data-balance="${which}"]`,
			);

			if (el instanceof HTMLElement) {
				el.textContent = fmt(record?.[which]);
			}
		}
	});

	return (
		<Flex.Row ref={root} align="center" gap={6}>
			<Reading label="Cash" tone="f1" weight="medium" which="cash" />
			<Reading label="Unrealized" tone="f2" weight="medium" which="unrealized" />
			<Reading label="Equity" tone="accent" weight="semibold" which="equity" />
		</Flex.Row>
	);
};
