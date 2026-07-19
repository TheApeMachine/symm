import type { CausalFrame } from "#/collections/types";
import type { ManifoldFrame } from "#/collections/types";
import type { ResonanceFrame } from "#/collections/types";
import {
	causalAssociation,
	causalCategory,
	causalConfidence,
	causalContagion,
	causalEntryBaseline,
	causalIntervention,
	causalNoise,
	causalRatio,
	causalReading,
	causalStrength,
} from "#/components/terminal/causal-view";
import { manifoldReading } from "#/components/terminal/xray-view";
import type { StrategyDecision } from "#/types/thesis";

const finiteOrNull = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const readingNumber = (
	reading: Record<string, unknown> | null,
	camel: string,
	pascal: string,
): number | null =>
	finiteOrNull(reading?.[camel]) ?? finiteOrNull(reading?.[pascal]);

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

/*
resonancePredict is the surprise-derived confidence analyzer uses when composing
forecast confidence: exp(-|surprise|). Missing surprise is not a zero confidence.
*/
export const resonancePredict = (
	frame: ResonanceFrame | undefined,
): number | null => {
	const surprise = finiteOrNull(frame?.surprise);

	if (surprise === null) {
		return null;
	}

	return Math.exp(-Math.abs(surprise));
};

/*
resonanceEdge is the return head's signed expected return, used as the predict
attribution delta on the decision waterfall.
*/
export const resonanceEdge = (
	frame: ResonanceFrame | undefined,
): number | null => finiteOrNull(frame?.expectedReturn);

/*
manifoldField is mean |ψ|² from the published manifold reading. Accepts both
camelCase (tagged wire) and PascalCase (legacy untagged wire).
*/
export const manifoldField = (
	frame: ManifoldFrame | undefined,
): number | null => {
	const reading = manifoldReading(frame);

	return readingNumber(reading, "coherenceMag2", "CoherenceMag2");
};

const finite = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const ratio = (value: unknown): number =>
	Math.min(1, Math.max(0, finite(value)));

const present = (value: number | null): number => (value === null ? 0 : value);

export type CandidateModel = {
	symbol: string;
	support: number;
	score: number;
	verdict: CandidateVerdict;
	why: string;
	inPlay: boolean;
	edge: number;
	branch: string;
	bars: Array<{ src: string; value: number }>;
	waterfall: Array<{ src: string; delta: number }>;
	probes: Array<{ label: string; value: number }>;
	hasDecision: boolean;
};

/*
buildCandidate composes ladder frames into the row model DecisionsSurface paints
through DOM refs so React only remounts when the symbol set changes.
*/
export const buildCandidate = (
	symbol: string,
	decision: StrategyDecision | undefined,
	causal: CausalFrame | undefined,
	resonance: ResonanceFrame | undefined,
	manifold: ManifoldFrame | undefined,
): CandidateModel => {
	const causalStrengthValue = causalStrength(causal);
	const causalBaselineValue = causalEntryBaseline(causal);
	const predict = resonancePredict(resonance);
	const field = manifoldField(manifold);
	const predictEdge = resonanceEdge(resonance);
	const score =
		decision?.utility ??
		Math.min(ratio(present(predict)), causalRatio(causalConfidence(causal)));
	const support = [causal, resonance, manifold].filter(Boolean).length;
	const judgement = judgeCandidate(decision, support, causalCleared(causal), {
		causal: causal === undefined,
		resonance: resonance === undefined,
		manifold: manifold === undefined,
	});
	const bars = [
		{ src: "causal", value: causalStrengthValue, ok: causal !== undefined },
		{ src: "predict", value: present(predict), ok: resonance !== undefined },
		{ src: "manifold", value: present(field), ok: manifold !== undefined },
	].filter((bar) => bar.ok);

	return {
		symbol,
		support,
		score,
		verdict: judgement.verdict,
		why: judgement.why,
		inPlay: judgement.inPlay,
		edge: pearlEdge(causal),
		hasDecision: decision !== undefined,
		branch:
			decision?.cause ??
			[
				manifold?.category,
				resonance?.category,
				causal === undefined ? "" : String(causalCategory(causal)),
			]
				.filter(Boolean)
				.join(" / "),
		bars: bars.map(({ src, value }) => ({ src, value })),
		waterfall: [
			{ src: "causal", delta: causalStrengthValue - causalBaselineValue },
			{ src: "predict", delta: present(predictEdge) },
			{ src: "field", delta: present(field) },
		],
		probes: [
			{ label: "beta", value: causalAssociation(causal) },
			{ label: "panic", value: causalContagion(causal) },
			{ label: "residual", value: causalNoise(causal) },
			{ label: "intervention", value: causalIntervention(causal) },
		],
	};
};
