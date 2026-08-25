import { resonanceFocusStore, useSubscribe } from "#/providers/ws-stores";

const num = (value: number | undefined): string =>
	value === undefined ? "—" : value.toString();

export const LiveResonanceTitle = () => {
	const root = useSubscribe(resonanceFocusStore, (state) => {
		const set = (which: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-res=${which}]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("horizon", num(state?.forecast?.supportedHorizon));
		set("reach", num(state?.forecast?.probeHorizon));
		set("precision", state?.taskRelativePrecision === undefined ? "—" : state.taskRelativePrecision.toFixed(3));
	});

	return (
		<span ref={root}>
			h<span data-res="horizon">—</span>
			{" · r "}
			<span data-res="reach">—</span>
			{" · relative precision "}
			<span data-res="precision">—</span>
		</span>
	);
};
