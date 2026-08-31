import type {
	EpisodeKind,
	HindsightEpisode,
	ReferenceRole,
} from "./hindsight-types";

/*
The Episode vocabulary, in one place.

Naming is part of the safety boundary of this architecture: the UI is not
allowed to say "missed profit", "capture", or "should have entered", and it is
not allowed to imply that a bigger number here means SYMM did worse. Every
label below describes market or operational reality only, so the words on the
screen stay the words the domain permits.
*/

export type EpisodeLane = "geometry" | "regime";

export type EpisodeDescriptor = {
	/* Short label for a dense ribbon. */
	label: string;
	/* Full name for a list row or tooltip. */
	name: string;
	/* What was observed, stated without any counterfactual. */
	meaning: string;
	/* Theme token the ribbon and its markers paint with. */
	color: string;
	/* Which band of the timeline the ribbon stacks into. */
	lane: EpisodeLane;
};

export const EPISODE_DESCRIPTORS: Record<EpisodeKind, EpisodeDescriptor> = {
	upward_excursion: {
		label: "UP",
		name: "Upward excursion",
		meaning: "The declared coordinate rose by the shown fraction.",
		color: "var(--up)",
		lane: "geometry",
	},
	downward_excursion: {
		label: "DOWN",
		name: "Downward excursion",
		meaning: "The declared coordinate fell by the shown fraction.",
		color: "var(--down)",
		lane: "geometry",
	},
	reversal: {
		label: "REV",
		name: "Reversal",
		meaning:
			"Two opposite qualifying legs met at a shared pivot; traversed is the ground both legs covered.",
		color: "var(--acc)",
		lane: "geometry",
	},
	volatility_expansion: {
		label: "VOL+",
		name: "Volatility expansion",
		meaning:
			"Realised dispersion of the coordinate's log returns ran above its own baseline.",
		color: "var(--warn)",
		lane: "regime",
	},
	volatility_contraction: {
		label: "VOL−",
		name: "Volatility contraction",
		meaning:
			"Realised dispersion of the coordinate's log returns ran below its own baseline.",
		color: "var(--f3)",
		lane: "regime",
	},
	spread_expansion: {
		label: "SPRD",
		name: "Spread expansion",
		meaning:
			"The quoted spread ran above its own baseline. Quoted, not an execution cost.",
		color: "var(--info)",
		lane: "regime",
	},
	liquidity_collapse: {
		label: "DEPTH",
		name: "Liquidity collapse",
		meaning:
			"Size quoted at the touch collapsed below its own baseline. Quoted size, not executable quantity.",
		color: "var(--brand)",
		lane: "regime",
	},
	arrival_cluster: {
		label: "BURST",
		name: "Arrival cluster",
		meaning: "Observations arrived at a multiple of their own baseline rate.",
		color: "var(--info)",
		lane: "regime",
	},
};

export const REFERENCE_GLYPHS: Record<ReferenceRole, string> = {
	anchor: "◤",
	peak: "▲",
	trough: "▼",
	reversal: "◆",
	exit_anchor: "◢",
	shock_onset: "✦",
};

export const REFERENCE_MEANING: Record<ReferenceRole, string> = {
	anchor: "Retrospective start of the selected excursion. Not a buy point.",
	peak: "Retrospective maximum of the selected excursion.",
	trough: "Retrospective minimum of the selected excursion.",
	reversal: "The pivot the two legs share.",
	exit_anchor: "Retrospective end of the second leg. Not a sell point.",
	shock_onset: "First observation at which the regime qualified.",
};

export const describeEpisode = (kind: EpisodeKind): EpisodeDescriptor =>
	EPISODE_DESCRIPTORS[kind] ?? {
		label: kind.slice(0, 4).toUpperCase(),
		name: kind,
		meaning: "",
		color: "var(--f3)",
		lane: "regime",
	};

/*
episodeReadout is the one-line quantity for an episode, phrased in the terms the
episode actually measured. A price episode reads as a signed fraction of its
anchor; a regime span reads as its ratio against the bar it had to clear.
*/
export const episodeReadout = (episode: HindsightEpisode): string => {
	if (episode.hasTraversed) {
		const net = (episode.observedExcursion * 100).toFixed(2);
		return `${(episode.traversed * 100).toFixed(2)}% traversed · net ${net}%`;
	}

	if (episode.hasObservedExcursion) {
		const excursion = episode.observedExcursion * 100;
		return `${excursion > 0 ? "+" : ""}${excursion.toFixed(2)}%`;
	}

	if (episode.hasRatio && episode.hasThreshold) {
		return `${episode.ratio.toFixed(2)}× vs ${episode.threshold.toFixed(2)}× bar`;
	}

	if (episode.hasRatio) {
		return `${episode.ratio.toFixed(2)}×`;
	}

	return "—";
};

/*
episodeRank orders episodes for the target list. It ranks price geometry above
regimes because a coordinate that moved is the primary thing an inspection
session is looking for, and it never ranks the two scales against each other.
*/
export const episodeRank = (episode: HindsightEpisode): number => {
	if (episode.hasTraversed) return 1000 + episode.traversed;
	if (episode.hasObservedExcursion) {
		return 1000 + Math.abs(episode.observedExcursion);
	}

	if (!episode.hasRatio || !episode.hasThreshold || episode.threshold <= 0) {
		return 0;
	}

	return episode.threshold >= 1
		? episode.ratio / episode.threshold
		: episode.threshold / Math.max(episode.ratio, 1e-12);
};
