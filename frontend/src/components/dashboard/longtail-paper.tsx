import { Link } from "@tanstack/react-router";
import { useLongtailReport, verdict } from "#/collections/longtail";
import { Flex, Section, Typography } from "#/components/ui";

export const LongtailPaperPanel = () => {
	const { samples } = useLongtailReport();
	const latest = samples.at(-1);
	const paper = latest?.paper;
	const call = latest ? verdict(samples, "paper") : null;

	return (
		<Section className="border-(--line) border-t">
			<Section.Header
				title="Paper longtail"
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
						Collecting samples from live training ticks…
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
							<Typography.Span>{paper?.closed ?? "—"}</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Win rate
							</Typography.Span>
							<Typography.Span>
								{paper?.winRate != null
									? `${(100 * paper.winRate).toFixed(1)}%`
									: "—"}
							</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Mean return
							</Typography.Span>
							<Typography.Span>
								{paper?.meanReturn != null
									? `${(100 * paper.meanReturn).toFixed(2)}%`
									: "—"}
							</Typography.Span>
						</Flex.Column>
						<Flex.Column gap={1}>
							<Typography.Span className="text-(--muted) text-xs">
								Open venue
							</Typography.Span>
							<Typography.Span>{paper?.open ?? "—"}</Typography.Span>
						</Flex.Column>
					</Flex.Row>
				)}
			</Section.Body>
		</Section>
	);
};
