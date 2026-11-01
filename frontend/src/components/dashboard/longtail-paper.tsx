import { Link } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { bookAtom } from "#/collections/book";
import { tierStats, useLongtailReport, verdict } from "#/collections/longtail";
import { Flex, Section, Typography } from "#/components/ui";

export const LongtailPaperPanel = () => {
	const { samples } = useLongtailReport();
	const book = useSelector(bookAtom, (state) => state);
	const latest = samples.at(-1);
	const tier = latest ? tierStats(latest, book) : null;
	const call = latest ? verdict(samples, book) : null;

	return (
		<Section className="border-(--line) border-t">
			<Section.Header
				title={`${book} longtail`}
				meta={
					<Link
						to="/longtail"
						className="text-(--acc) hover:underline"
					>
						Open full dashboard
					</Link>
				}
			/>
			<Section.Body className="gap-3 p-3">
				{!latest ? (
					<Typography.Span className="text-(--muted)">
						Collecting samples from desk lots, confidence, and model/paths…
					</Typography.Span>
				) : (
					<Flex.Row gap={4} wrap="wrap">
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Verdict
							</Typography.Span>
							<Typography.Span>{call?.label ?? "—"}</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Closed lots
							</Typography.Span>
							<Typography.Span>{tier?.stats.closed ?? "—"}</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Win rate
							</Typography.Span>
							<Typography.Span>
								{tier?.winRate != null
									? `${(100 * tier.winRate).toFixed(1)}%`
									: "—"}
							</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Mean return
							</Typography.Span>
							<Typography.Span>
								{tier?.edge != null
									? `${(100 * tier.edge).toFixed(2)}%`
									: "—"}
							</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Open
							</Typography.Span>
							<Typography.Span>{tier?.stats.open ?? "—"}</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Paths @ floor
							</Typography.Span>
							<Typography.Span>
								{latest.paths.promoted ?? "—"}
							</Typography.Span>
						</Flex.Column>
					</Flex.Row>
				)}
			</Section.Body>
		</Section>
	);
};
