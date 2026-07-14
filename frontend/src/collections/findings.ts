import { createStore } from "@tanstack/react-store";
import type { Finding } from "#/types/thesis";

const findingKey = (finding: Finding): string =>
	`${finding.symbol}:${finding.component}:${finding.condition}:${finding.estimatedEffect}`;

const asFindings = (frame: unknown): Finding[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is Finding =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as Finding).symbol === "string" &&
			typeof (row as Finding).component === "string" &&
			typeof (row as Finding).condition === "string",
	);
};

/*
findingsStore retains the backend thesis.Findings snapshot so PostMortem
evidence can be inspected without mutating or replaying the live model state.
*/
export const findingsStore = createStore(
	{
		findings: [] as Finding[],
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((state) => {
				const incoming = asFindings(frame);

				if (incoming.length === 0) {
					return state;
				}

				const seen = new Set(state.findings.map(findingKey));
				const findings = [...state.findings];

				for (const finding of incoming) {
					const key = findingKey(finding);

					if (seen.has(key)) {
						continue;
					}

					seen.add(key);
					findings.push(finding);
				}

				return { findings };
			}),
		reset: () =>
			setState(() => ({
				findings: [],
			})),
	}),
);
