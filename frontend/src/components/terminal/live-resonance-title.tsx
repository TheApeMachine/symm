import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import { asFinite } from "#/lib/format";
import { parseResonanceData, resonanceAt } from "#/lib/resonance";

export const LiveResonanceTitle = () => {
	const symbol = useSelector(atoms.focus);
	const row = useSelector(signals.resonance, (state) => resonanceAt(state, symbol));
	const res = row ? parseResonanceData(row) : undefined;

	const horizon =
		res?.supportedHorizon != null ? String(Math.trunc(res.supportedHorizon)) : "—";
	const reach =
		res && res.forwardCurve.length > 0
			? String(res.forwardCurve.length)
			: "—";
	const precision = (() => {
		const n = asFinite(res?.taskRelativePrecision);
		return n === null ? "—" : n.toFixed(3);
	})();

	return (
		<span>
			h<span data-res="horizon">{horizon}</span>
			{" · r "}
			<span data-res="reach">{reach}</span>
			{" · relative precision "}
			<span data-res="precision">{precision}</span>
		</span>
	);
};