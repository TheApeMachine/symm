import { useSelector } from "@tanstack/react-store";
import { shallow } from "@tanstack/store";
import { positionStore } from "#/collections/app";
import { Callout } from "#/components/ui/callout";
import { DataRow } from "#/components/ui/data-row";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";

type PositionState = ReturnType<typeof positionStore.get>;

const value = (raw: string | null): string => raw || "—";

const currentPosition = (state: PositionState, symbol: string) => {
	const frames =
		typeof (state as any)?.toArray === "function"
			? (state as any).toArray()
			: Array.isArray(state)
				? state
				: [];
	const position = new Position();
	const holding = new Holding();

	for (let frameIndex = frames.length - 1; frameIndex >= 0; frameIndex--) {
		const frame = frames[frameIndex];
		if (!frame || typeof frame.rowsLength !== "function") continue;

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
		<Flex.Column gap={2} className="min-h-0 overflow-auto pr-1">
			<Callout tone="neutral" size="s">
				<Callout.Title tone="accent">Live now</Callout.Title>
				<Callout.Description>
					These values change with the market. Everything in the larger entry
					panel is frozen.
				</Callout.Description>
			</Callout>

			<Panel variant="surface" size="bare" className="px-3 py-2.5">
				<Typography.Label size="lg" tone="f1">
					Position now
				</Typography.Label>
				<Typography.Paragraph
					variant="f4"
					className="mt-0.5 mb-2 text-[9px] leading-relaxed"
				>
					What the lot is worth if judged at the latest realizable sell price.
				</Typography.Paragraph>
				<DataRow.Group density="bare" border={false} className="gap-1">
					<DataRow
						label="status"
						value={position?.status ?? "—"}
						density="bare"
					/>
					<DataRow
						label="amount held"
						value={position?.quantity ?? "—"}
						density="bare"
					/>
					<DataRow
						label="bought at"
						value={position?.entry ?? "—"}
						density="bare"
					/>
					<DataRow
						label="sell price now"
						value={position?.mark ?? "—"}
						density="bare"
					/>
					<DataRow
						label="profit / loss"
						value={`${position?.pnl ?? "—"} USD`}
						density="bare"
						valueClassName="text-(--pnl)"
					/>
					<DataRow
						label="return since entry"
						value={position?.returnPct ?? "—"}
						density="bare"
						valueClassName="text-(--pnl)"
					/>
				</DataRow.Group>
			</Panel>

			<Panel variant="surface" size="bare" className="px-3 py-2.5">
				<Typography.Label size="s" tone="f2">
					A useful way to read this
				</Typography.Label>
				<Typography.Paragraph
					variant="f4"
					className="mt-1 text-[10px] leading-relaxed"
				>
					The entry snapshot answers “why did we buy?” This rail answers “what
					is happening to that buy now?” Keeping them apart prevents
					today&apos;s price from rewriting yesterday&apos;s reasoning.
				</Typography.Paragraph>
			</Panel>
		</Flex.Column>
	);
};
