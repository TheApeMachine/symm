import { createFileRoute } from "@tanstack/react-router";
import { SymmTerminal } from "#/components/terminal/symm-terminal";

export const Route = createFileRoute("/")({
  component: SymmTerminal,
});
