import { createFileRoute } from "@tanstack/react-router";
import { JournalSurface } from "#/components/terminal/journal-surface";

export const Route = createFileRoute("/journal")({
	component: JournalSurface,
});
