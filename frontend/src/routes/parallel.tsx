import { createFileRoute } from "@tanstack/react-router";
import { ParallelDashboard } from "#/components/parallel/dashboard";

export const Route = createFileRoute("/parallel")({
	component: ParallelDashboard,
});
