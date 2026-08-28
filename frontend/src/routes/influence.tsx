import { createFileRoute } from "@tanstack/react-router";
import { InfluenceField } from "#/components/influence/component";

const RouteComponent = () => {
	return <InfluenceField />;
};

export const Route = createFileRoute("/influence")({
	component: RouteComponent,
});
