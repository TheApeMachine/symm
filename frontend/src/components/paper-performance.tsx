import { useSelector } from "@tanstack/react-store";
import { bookAtom, bookLabel } from "#/collections/book";
import {
	type BookConfidence,
	confidenceAtom,
	confidenceFor,
} from "#/collections/confidence";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { ratePercent, selectBook, useLifecycles } from "#/lib/lifecycle";

const pick = (
	backend: BookConfidence,
	fallback: { win_rate: number | null; mean_return: number | null },
) => ({
	win_rate: backend.win_rate ?? fallback.win_rate,
	edge: backend.edge ?? fallback.mean_return,
	closed: backend.closed,
});

/*
Top-bar win rate and edge prefer the backend confidence frame (paper/real
settles that arm live trading). Lifecycle stats fill gaps until the first
hub frame arrives.
*/
export const PaperPerformance = () => {
	const book = useSelector(bookAtom, (state) => state);
	const confidence = useSelector(confidenceAtom, (state) => state);
	const { report } = useLifecycles();
	const lifecycle = selectBook(report, book).performance;
	const backend = confidenceFor(
		confidence,
		book === "virtual" ? "paper" : book,
	);
	const stats = pick(backend, lifecycle);
	const name = bookLabel(book).toLowerCase();
	const title =
		stats.closed > 0 || (lifecycle.closed ?? 0) > 0
			? `${name} settled lots · win rate ${ratePercent(stats.win_rate)} · mean edge ${ratePercent(stats.edge)}`
			: `no settled ${name} round trips yet`;

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
					{ratePercent(stats.win_rate)}
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
					{ratePercent(stats.edge)}
				</Typography.Mono>
			</Flex.Column>
		</Flex.Row>
	);
};
