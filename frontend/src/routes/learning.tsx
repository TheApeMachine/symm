import { createFileRoute } from "@tanstack/react-router";
import { LearningDashboard } from "#/components/learning/dashboard";

export const Route = createFileRoute("/learning")({
	component: LearningDashboard,
});
