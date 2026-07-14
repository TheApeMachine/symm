import { createStore } from "@tanstack/react-store";
import type { Finding } from "#/types/thesis";
import { Circular, type CircularBuffer } from "./circular";

const FINDINGS_HISTORY_LIMIT = 256;

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
findingsList expands the retained postmortem findings buffer oldest first.
*/
export const findingsList = (buffer: CircularBuffer<Finding>): Finding[] =>
	buffer.values();

/*
findingsStore retains backend thesis findings in a bounded circular buffer so
PostMortem evidence can accumulate without growing the live model unbounded.
*/
export const findingsStore = createStore(
	{
		findings: Circular<Finding>(FINDINGS_HISTORY_LIMIT),
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((state) => {
				const incoming = asFindings(frame);

				if (incoming.length === 0) {
					return state;
				}

				const seen = new Set(state.findings.values().map(findingKey));

				for (const finding of incoming) {
					const key = findingKey(finding);

					if (seen.has(key)) {
						continue;
					}

					seen.add(key);
					state.findings.push(finding);
				}

				return { findings: state.findings };
			}),
		reset: () =>
			setState(() => ({
				findings: Circular<Finding>(FINDINGS_HISTORY_LIMIT),
			})),
	}),
);
