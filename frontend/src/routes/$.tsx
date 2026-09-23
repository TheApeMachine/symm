import { createFileRoute, useLocation } from "@tanstack/react-router";
import {
	GraphSurface,
	useGraphForPath,
} from "#/components/surface/graph-surface";
import { Alert } from "#/components/ui/alert";
import { Flex } from "#/components/ui/flex";
import { Spinner } from "#/components/ui/spinner";
import { Typography } from "#/components/ui/typography";

/*
Every surface the graph draws, at the path the graph says it draws it at.

There is no list of surfaces anywhere on this side. A URL is matched against the
ui.UIRoute nodes the hub holds, so a surface exists because a node declares it:
adding one is adding a node, and moving one is editing that node's path. This
route is what is left of routing once that is true — it claims everything, and
the surfaces still written as React files claim their own paths ahead of it
until they are deleted.
*/
const GraphRoute = () => {
	const location = useLocation();
	const resolved = useGraphForPath(location.pathname);

	if (resolved.status === "loading") {
		return (
			<Flex.Center className="h-full w-full gap-2 text-(--f3)">
				<Spinner />
				<Typography.Mono>resolving {location.pathname}</Typography.Mono>
			</Flex.Center>
		);
	}

	if (resolved.status === "absent" || !resolved.name) {
		return (
			<Flex.Column className="h-full w-full p-4">
				<Alert variant="error">
					Nothing draws {location.pathname}: no graph the hub holds declares a
					ui.UIRoute for this path.
				</Alert>
			</Flex.Column>
		);
	}

	return <GraphSurface name={resolved.name} />;
};

export const Route = createFileRoute("/$")({
	component: GraphRoute,
});
