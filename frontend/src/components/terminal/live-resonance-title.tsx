import { useSelector } from "@tanstack/react-store";
import { focusStore, resonanceStore } from "#/collections/app";
import { Resonance } from "#/providers/telemetry/telemetry/resonance";
import { ResonanceForecast } from "#/providers/telemetry/telemetry/resonance-forecast";

const resObj = new Resonance();
const forecastObj = new ResonanceForecast();

export const LiveResonanceTitle = () => {
	const symbol = useSelector(focusStore, (state) => state);
	const frameWithSymbol = useSelector(resonanceStore, (state) =>
		state.findLast((frame) => {
			for (let i = 0; i < frame.rowsLength(); i++) {
				const row = frame.rows(i, resObj);
				if (row && row.symbol() === symbol) {
					return true;
				}
			}
			return false;
		}),
	);

	let targetRes: Resonance | null = null;
	if (frameWithSymbol) {
		for (let i = 0; i < frameWithSymbol.rowsLength(); i++) {
			const row = frameWithSymbol.rows(i, resObj);
			if (row && row.symbol() === symbol) {
				targetRes = row;
				break;
			}
		}
	}

	const fcast = targetRes ? targetRes.forecast(forecastObj) : null;
	const horizon = fcast ? String(fcast.supportedHorizon()) : "—";
	const reach = fcast ? String(fcast.probeHorizon()) : "—";
	const precision = targetRes
		? targetRes.taskRelativePrecision().toFixed(3)
		: "—";

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


