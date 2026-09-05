import { createFileRoute } from "@tanstack/react-router";
import { RegulatorPredictiveCoding } from "#/components/dashboard/regulator-graph";

export const Route = createFileRoute("/regulator")({
	component: RegulatorSurface,
});

function RegulatorSurface() {
	return <RegulatorPredictiveCoding />;
}
