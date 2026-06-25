import { createFileRoute } from "@tanstack/react-router";
import { SymmTerminal } from "#/components/terminal/symm-terminal";

export const Route = createFileRoute("/")({
	validateSearch: (search: Record<string, unknown>) => ({
		surface: typeof search.surface === "string" ? search.surface : undefined,
	}),
	component: SymmTerminal,
});
