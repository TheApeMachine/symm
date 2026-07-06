import { createFileRoute } from "@tanstack/react-router";
import { SignalsSurface } from "#/components/terminal/signals-surface";

export const Route = createFileRoute("/signals")({
	component: SignalsSurface,
});
