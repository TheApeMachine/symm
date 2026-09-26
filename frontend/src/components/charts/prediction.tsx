import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { focusAtom, signals } from "#/collections/app";
import {
	HierarchyLanes as BaseHierarchyLanes,
	type HierarchyLanesProps,
	PredictionChart as BasePredictionChart,
	type PredictionChartProps,
	ScalarDiagnostics as BaseScalarDiagnostics,
	type ScalarDiagnosticsProps,
	VerdictRow as BaseVerdictRow,
	type VerdictRowProps,
} from "#/components/ui/prediction-chart";

export * from "#/components/ui/prediction-chart";

export const useArtifact = (): any => {
	const symbol = useSelector(focusAtom, (state) => state);
	const row = useSelector(signals.resonance, (state) => {
		const ring = state[symbol];
		return ring && !ring.isEmpty() ? (ring.getLast() as any) : undefined;
	});

	const held = useRef<{
		symbol: string | undefined;
		row: any;
	}>({ symbol, row: undefined });

	if (held.current.symbol !== symbol) {
		held.current = { symbol, row: undefined };
	}
	if (row !== undefined) {
		held.current.row = row;
	}
	return held.current.row;
};

export const ScalarDiagnostics = (props: ScalarDiagnosticsProps = {}) => {
	const fallbackArtifact = useArtifact();
	const artifact = props.artifact !== undefined ? props.artifact : fallbackArtifact;
	return <BaseScalarDiagnostics {...props} artifact={artifact} />;
};

export const VerdictRow = (props: VerdictRowProps = {}) => {
	const fallbackArtifact = useArtifact();
	const artifact = props.artifact !== undefined ? props.artifact : fallbackArtifact;
	return <BaseVerdictRow {...props} artifact={artifact} />;
};

export const HierarchyLanes = (props: HierarchyLanesProps = {}) => {
	const fallbackArtifact = useArtifact();
	const artifact = props.artifact !== undefined ? props.artifact : fallbackArtifact;
	return <BaseHierarchyLanes {...props} artifact={artifact} />;
};

export const TerminalPredictionChart = (props: PredictionChartProps = {}) => {
	const fallbackArtifact = useArtifact();
	const artifact = props.artifact !== undefined ? props.artifact : fallbackArtifact;
	return <BasePredictionChart {...props} artifact={artifact} />;
};
