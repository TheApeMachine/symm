import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import { bookReadout } from "#/components/book-readout";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { ratePercent } from "#/lib/lifecycle";

/*
PaperPerformance is the top bar's win rate and mean edge for the selected
book. Closed round trips own both figures. Open lots stand in until one closes.
*/
export const PaperPerformance = () => {
	const book = useSelector(bookAtom, (state) => state);
	const store = signals["positions"];
	const rings = useSelector(store, (state) => state);
	const positions = readPositions(rings);
	const readout = bookReadout(positions, book);
	const name = book === "virtual" ? "virtual" : "paper";
	const basis = readout.closed > 0 ? "closed" : "open";
	const title =
		readout.winRate !== null
			? `${name} ${basis} lots · win rate ${ratePercent(readout.winRate)} · mean edge ${ratePercent(readout.edge)}`
			: `no ${name} lots with a price yet`;

	return (
		<Flex.Row align="center" gap={6} data-wallet="performance" title={title}>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Win rate
				</Typography.Label>
				<Typography.Mono
					size="lg"
					tone="f2"
					weight="medium"
					data-performance="win-rate"
				>
					{ratePercent(readout.winRate)}
				</Typography.Mono>
			</Flex.Column>
			<Flex.Column className="items-end gap-px">
				<Typography.Label size="s" tone="f4" weight="normal">
					Edge / trade
				</Typography.Label>
				<Typography.Mono
					size="lg"
					tone="f2"
					weight="medium"
					data-performance="edge"
				>
					{ratePercent(readout.edge)}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};

const readPositions = (state: Record<string, any>): any[] => {
	const bySymbol = new Map<string, any>();
	for (const ring of Object.values(state)) {
		if (!ring || ring.isEmpty()) continue;
		const latest = ring.get(ring.getBufferLength() - 1);
		if (!latest) continue;
		const rows = latest.lookup("positions") as any[] | undefined;
		if (!Array.isArray(rows)) continue;
		for (const row of rows) {
			if (row && row.symbol) bySymbol.set(String(row.symbol), row);
		}
	}
	return [...bySymbol.values()];
};
