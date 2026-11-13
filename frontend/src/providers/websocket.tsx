import { useEffect } from "react";
import { atoms, signals } from "#/collections/app";
import { applyDiagPulse, applyDiagPulses } from "#/collections/diag-burst";
import {
	EMPTY_PIPELINE,
	mergeTopology,
	type PipelineFrame,
} from "#/collections/pipeline";
import type { Position } from "#/collections/types";
import {
	ensureLongtailRecorder,
	requestLongtailSources,
	resetLongtailSourcesRequest,
} from "#/collections/longtail";
import "#/hub/apply-s3-compact";
import { hubWsUrl } from "#/lib/hub";
import {
	connectHub,
	disconnectHub,
	sendFocus,
	sendRoute,
} from "#/lib/hub-socket";
import { resetClosedHistoryRequest } from "#/lib/lifecycle";
import { dropClosedTwins, mergePositionBook } from "#/lib/position-book";

export { sendCommand, sendRoute } from "#/lib/hub-socket";

const applyEnvelope = (payload: Record<string, unknown>) => {
	for (const key of Object.keys(payload)) {
		const value = payload[key];

		if (key === "diag_pulses") {
			applyDiagPulses(value);
			continue;
		}

		if (key === "diag_pulse") {
			applyDiagPulse(value);
			continue;
		}

		if (key === "pipeline") {
			atoms.pipeline.set(
				mergeTopology(
					(atoms.pipeline.get() as PipelineFrame) ?? EMPTY_PIPELINE,
					(value as PipelineFrame) ?? EMPTY_PIPELINE,
				),
			);
			continue;
		}

		const atom = atoms[key];

		if (atom) {
			// Tier wallets arrive as {paper|virtual|real: reading}.
			if (value && typeof value === "object" && !Array.isArray(value)) {
				if (key === "confidence") {
					const prev = atom.get() as Record<string, unknown>;
					const patch = value as Record<string, unknown>;
					const next: Record<string, unknown> = { ...prev, ...patch };

					for (const book of ["virtual", "paper", "real"] as const) {
						const prior = prev[book];
						const incoming = patch[book];

						if (
							prior &&
							typeof prior === "object" &&
							incoming &&
							typeof incoming === "object" &&
							!Array.isArray(prior) &&
							!Array.isArray(incoming)
						) {
							next[book] = {
								...(prior as Record<string, unknown>),
								...(incoming as Record<string, unknown>),
							};
						}
					}

					atom.set(next);
				} else {
					atom.set({ ...atom.get(), ...value });
				}
			} else {
				atom.set(value);
			}

			continue;
		}

		const store = signals[key];

		if (!store) {
			continue;
		}

		if (
			key === "positions" &&
			value &&
			typeof value === "object" &&
			!Array.isArray(value)
		) {
			const tier = typeof payload.tier === "string" ? payload.tier : "";

			store.setState((prev: Record<string, Position> | null) =>
				mergePositionBook(prev, value as Record<string, Position>, {
					snapshot: payload.positions_snapshot === true,
					tier,
				}),
			);
			continue;
		}

		// Symbol-grouped metrics: merge {symbol: reading} into the store.
		if (value && typeof value === "object" && !Array.isArray(value)) {
			store.setState((prev: Record<string, unknown> | null) => ({
				...(prev && typeof prev === "object" ? prev : {}),
				...(value as Record<string, unknown>),
			}));
		} else {
			store.setState(() => value);
		}
	}
};

export const WsFeed = () => {
	useEffect(() => {
		ensureLongtailRecorder();
		signals.positions.setState((prev: Record<string, Position> | null) =>
			dropClosedTwins(prev ?? {}),
		);

		connectHub(`${hubWsUrl()}/ws`, {
			onEnvelope: applyEnvelope,
			onStatus: (status) => {
				atoms.online.set(status);

				if (status === "ONLINE") {
					// Next Hello replaces the open book from a clean slate.
					signals.positions.setState(() => ({}));
					resetClosedHistoryRequest();
					resetLongtailSourcesRequest();
					requestLongtailSources();
				}
			},
			onError: (error) => {
				console.error("WS error:", error);
			},
		});

		sendRoute(atoms.route.get());

		const unsubscribeFocus = atoms.focus.subscribe((symbol: string) => {
			sendFocus(symbol);
		});

		const unsubscribeRoute = atoms.route.subscribe((route: string) => {
			sendRoute(route);
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			disconnectHub();
		};
	}, []);

	return null;
};
