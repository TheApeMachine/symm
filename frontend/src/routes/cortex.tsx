import { createFileRoute } from "@tanstack/react-router";
import { CortexSurface } from "#/components/terminal/cortex-surface";

export const Route = createFileRoute("/cortex")({
	component: CortexSurface,
});
