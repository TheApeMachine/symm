import { createFileRoute, Link } from "@tanstack/react-router";
import { useHubGraphs } from "#/components/surface/graph-surface";
import { Alert } from "#/components/ui/alert";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { List } from "#/components/ui/list";
import { Section } from "#/components/ui/section";
import { Spinner } from "#/components/ui/spinner";
import { Typography } from "#/components/ui/typography";

/*
What the hub can draw.

A surface exists because a node declares it, which means nothing lists them
until something reads those nodes. This is that listing: every graph the hub
holds, the path its ui.UIRoute claims, and a way into it.
*/
const DynamicIndex = () => {
	const { status, graphs, reason } = useHubGraphs();
	const surfaces = graphs.filter((graph) => graph.path);
	const rest = graphs.filter((graph) => !graph.path);

	if (status === "loading") {
		return (
			<Flex.Center className="h-full w-full gap-2 text-(--f3)">
				<Spinner />
				<Typography.Mono>reading graphs from the hub</Typography.Mono>
			</Flex.Center>
		);
	}

	if (status === "failed") {
		return (
			<Flex.Column className="h-full w-full p-4">
				<Alert variant="error">
					Could not reach the hub: {reason}. Nothing can be drawn from a graph
					until it answers.
				</Alert>
			</Flex.Column>
		);
	}

	return (
		<Flex.Column className="h-full min-h-0 w-full overflow-auto">
			<Section className="min-h-0">
				<Section.Header
					title="Surfaces drawn from graphs"
					meta={`${surfaces.length} of ${graphs.length} graphs draw one`}
				/>
				<List className="p-2">
					{surfaces.length === 0 ? (
						<List.Empty>
							No graph declares a ui.UIRoute yet.
						</List.Empty>
					) : (
						surfaces.map((graph) => (
							<List.Item key={graph.name} interactive>
								<Link
									to="/dynamic/$name"
									params={{ name: graph.name }}
									className="flex w-full items-center gap-3"
								>
									<Typography.Mono className="min-w-56 text-(--f1)">
										{graph.name}
									</Typography.Mono>
									<Badge label={String(graph.path)} variant="info" />
									<Typography.Span className="text-(--f4)">
										{graph.title ?? ""}
									</Typography.Span>
								</Link>
							</List.Item>
						))
					)}
				</List>
			</Section>

			<Section className="min-h-0">
				<Section.Header
					title="Graphs that draw nothing"
					meta={`${rest.length} compute only`}
				/>
				<List className="p-2">
					{rest.map((graph) => (
						<List.Item key={graph.name}>
							<Typography.Mono className="text-(--f4)">
								{graph.name}
							</Typography.Mono>
						</List.Item>
					))}
				</List>
			</Section>
		</Flex.Column>
	);
};

export const Route = createFileRoute("/dynamic/")({
	component: DynamicIndex,
});
