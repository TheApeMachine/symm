import { createFileRoute } from "@tanstack/react-router";
import { MetricLineage } from "#/components/lineage/component";

const RouteComponent = () => {
	return <MetricLineage />;
};

export const Route = createFileRoute("/lineage")({
	component: RouteComponent,
});
