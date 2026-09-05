import { useSelector } from "@tanstack/react-store";
import { shallow } from "@tanstack/store";
import { positionStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";

type PositionState = ReturnType<typeof positionStore.get>;

const value = (raw: string | null): string => raw || "—";

const Row = ({
	label,
	value,
	tone = "text-(--f1)",
}: {
	label: string;
	value: string;
	tone?: string;
}) => (
	<Flex.Row align="baseline" justify="between" className="gap-2">
		<Typography.Label size="xxs" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono
			size="s"
			className={cn("min-w-0 truncate text-right", tone)}
		>
			{value}
		</Typography.Mono>
	</Flex.Row>
);

const Card = ({
	title,
	caption,
	children,
}: {
	title: string;
	caption: string;
	children: React.ReactNode;
}) => (
	<Panel variant="surface" size="bare" className="px-3 py-2.5">
		<Typography.Label size="lg" tone="f1">
			{title}
		</Typography.Label>
		<p className="mt-0.5 mb-2 text-[9px] text-(--f4) leading-relaxed">
			{caption}
		</p>
		<div className="flex flex-col gap-1">{children}</div>
	</Panel>
);

const currentPosition = (state: PositionState, symbol: string) => {
	const frames = state.toArray();
	const position = new Position();
	const holding = new Holding();

	for (let frameIndex = frames.length - 1; frameIndex >= 0; frameIndex--) {
		const frame = frames[frameIndex];

		for (let rowIndex = 0; rowIndex < frame.rowsLength(); rowIndex++) {
			const row = frame.rows(rowIndex, position);
			const rowHolding = row?.holding(holding);

			if (rowHolding?.symbol() !== symbol) {
				continue;
			}

			const returnPct = rowHolding.returnPct();

			return {
				status: value(rowHolding.status() ?? row?.status() ?? null),
				quantity: value(rowHolding.qty()),
				entry: value(rowHolding.entryPrice()),
				mark: value(rowHolding.mark()),
				pnl: value(rowHolding.pnl()),
				returnPct: Number.isFinite(returnPct)
					? `${returnPct.toFixed(2)}%`
					: "—",
			};
		}
	}

	return null;
};

export const ThesisDetailRail = ({ symbol }: { symbol: string }) => {
	const position = useSelector(
		positionStore,
		(state) => currentPosition(state, symbol),
		{ compare: shallow },
	);

	return (
		<div className="flex min-h-0 flex-col gap-2 overflow-auto pr-1">
			<div className="rounded-[4px] border border-(--line2) bg-(--sunken) px-3 py-2.5">
				<Typography.Label size="xs" tone="accent">
					Live now
				</Typography.Label>
				<p className="mt-1 text-[9px] text-(--f4) leading-relaxed">
					These values change with the market. Everything in the larger entry
					panel is frozen.
				</p>
			</div>

			<Card
				title="Position now"
				caption="What the lot is worth if judged at the latest realizable sell price."
			>
				<Row label="status" value={position?.status ?? "—"} />
				<Row label="amount held" value={position?.quantity ?? "—"} />
				<Row label="bought at" value={position?.entry ?? "—"} />
				<Row label="sell price now" value={position?.mark ?? "—"} />
				<Row
					label="profit / loss"
					value={`${position?.pnl ?? "—"} USD`}
					tone="text-(--pnl)"
				/>
				<Row
					label="return since entry"
					value={position?.returnPct ?? "—"}
					tone="text-(--pnl)"
				/>
			</Card>

			<Panel variant="surface" size="bare" className="px-3 py-2.5">
				<Typography.Label size="s" tone="f2">
					A useful way to read this
				</Typography.Label>
				<p className="mt-1 text-[10px] text-(--f4) leading-relaxed">
					The entry snapshot answers “why did we buy?” This rail answers “what
					is happening to that buy now?” Keeping them apart prevents
					today&apos;s price from rewriting yesterday&apos;s reasoning.
				</p>
			</Panel>
		</div>
	);
};
