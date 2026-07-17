import type { CausalFrame } from "#/collections/causal";
import {
	causalEntryBaseline,
	causalReading,
	causalStrength,
} from "#/components/terminal/causal-view";
import type { StrategyDecision } from "#/types/thesis";

const finiteOrNull = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

/*
causalCleared is true only when both Pearl strength and entry baseline are
present and strength strictly clears the baseline. Missing zeros must not count
as cleared (0 >= 0 was the BLOCKED lie).
*/
export const causalCleared = (frame: CausalFrame | undefined): boolean => {
	const reading = causalReading(frame);
	const strength = finiteOrNull(reading?.strength);
	const baseline = finiteOrNull(reading?.entryBaseline);

	return strength !== null && baseline !== null && strength > baseline;
};

export type CandidateVerdict =
	| "allow"
	| "blocked"
	| "hold"
	| "waiting"
	| "below";

export type CandidateJudgement = {
	verdict: CandidateVerdict;
	why: string;
	inPlay: boolean;
};

/*
judgeCandidate prefers the strategy Decision wire when present, and otherwise
reports waiting / below-line instead of inventing "BLOCKED below utility".
*/
export const judgeCandidate = (
	decision: StrategyDecision | undefined,
	support: number,
	cleared: boolean,
	missing: {
		causal: boolean;
		resonance: boolean;
		manifold: boolean;
	},
): CandidateJudgement => {
	const inPlay = support >= 2 && cleared;

	if (decision?.action === "enter" || decision?.action === "exit") {
		return {
			verdict: "allow",
			why: decision.reason || decision.action,
			inPlay: true,
		};
	}

	if (decision !== undefined) {
		const hold = decision.action === "hold" || decision.action === "nothing";

		return {
			verdict: hold ? "hold" : "blocked",
			why: decision.reason || decision.action || "strategy",
			inPlay,
		};
	}

	if (missing.causal) {
		return { verdict: "waiting", why: "waiting causal", inPlay: false };
	}

	if (missing.resonance) {
		return { verdict: "waiting", why: "waiting resonance", inPlay: false };
	}

	if (missing.manifold) {
		return { verdict: "waiting", why: "waiting manifold", inPlay: false };
	}

	if (!cleared) {
		return {
			verdict: "below",
			why: "below causal line",
			inPlay: false,
		};
	}

	return {
		verdict: "waiting",
		why: "waiting strategy",
		inPlay: true,
	};
};

/*
pearlEdge is the raw Pearl strength delta. It is not a [0,1] utility and must
not be read as the combined score.
*/
export const pearlEdge = (frame: CausalFrame | undefined): number =>
	causalStrength(frame) - causalEntryBaseline(frame);
