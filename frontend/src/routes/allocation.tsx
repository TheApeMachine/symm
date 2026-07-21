import { createFileRoute } from "@tanstack/react-router";
import { AllocationSurface } from "#/components/terminal/allocation-surface";

export const Route = createFileRoute("/allocation")({
	component: AllocationSurface,
});
