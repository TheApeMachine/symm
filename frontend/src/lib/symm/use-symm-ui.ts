import { useSelector } from "@tanstack/react-store";

import { appStore } from "#/collections/app";
import { DecisionsDataProvider } from "#/components/panels/data/decisions-data-provider";
import type { EvaluationRow } from "#/lib/symm/events";
import { useStoreSnapshot } from "#/lib/symm/use-store-snapshot";

export const useSymmConnected = () =>
	useSelector(appStore, (state) => state.online);

export const useSymmDecisionTrace = () =>
	useStoreSnapshot(DecisionsDataProvider);

export const useSymmEnginePulse = () => undefined;

export const useSymmEntryLine = () => ({
	line: 0,
	median: 0,
	mad: 0,
});

export const useSymmEvaluations = (): EvaluationRow[] => [];

export const useSymmScanProgress = () => ({
	quotesReady: 0,
	symbolsTotal: undefined as number | undefined,
	fluidSampled: 0,
});
