import { isConcreteSymbol, resolveScopedFrame } from "./scoped-frame";

type ReadingsState = Record<string, Record<string, unknown>>;

export type TerminalKernel = {
	source: string;
	confidencePercent: number;
	surprisePercent: number;
};

const numberFrom = (output: Record<string, unknown>, key: string): number => {
	const nested = output[key];

	return typeof nested === "number" ? nested : 0;
};

/*
kernelsForFocus projects the latest measurement per source into the
TerminalKernel shape the Decision Tree and Allocation surfaces consume.
confidence/surprise come straight off the backend measurement output - no
client-side scoring, just unit-to-percent scaling.

readings is the measurementsStore state: source -> symbol -> frame. When a
concrete focus symbol is supplied, only that symbol's backend reading is used.
Fallback to the first live symbol is reserved for stream/no-focus mode.
*/
export const kernelsForFocus = (
	readings: ReadingsState,
	focusSymbol?: string,
): TerminalKernel[] => {
	const kernels: TerminalKernel[] = [];

	for (const source of Object.keys(readings)) {
		const bySymbol = readings[source] as
			| Record<string, Record<string, unknown>>
			| undefined;

		if (bySymbol === undefined) {
			continue;
		}

		const scoped = resolveScopedFrame(bySymbol, focusSymbol, source);

		if (isConcreteSymbol(focusSymbol) && scoped.mode !== "concrete") {
			continue;
		}

		const frame = scoped.frame;

		if (frame === null) {
			continue;
		}

		const output = (frame?.output ?? {}) as Record<string, unknown>;
		const confidence = numberFrom(output, "confidence");
		const surprise = numberFrom(output, "surprise");

		kernels.push({
			source,
			confidencePercent: Math.round(confidence * 100),
			surprisePercent: Math.round(Math.min(surprise, 1) * 100),
		});
	}

	return kernels;
};
