import { createFileRoute } from "@tanstack/react-router";
import { Workbench } from "#/components/workbench/workbench";

const RouteComponent = () => <Workbench />;

export const Route = createFileRoute("/workbench")({
	component: RouteComponent,
});
