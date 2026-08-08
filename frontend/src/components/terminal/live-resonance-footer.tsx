import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { Component } from "#/components/ui/component";
/*
The resonance batch carries every settled carrier, not just the focused one, so
the row is pinned by symbol. Reading index 0 used to be the same thing when the
solver published a single row; it now names whichever carrier happens to sort
first, which is a different symbol from one frame to the next.
*/

export const LiveResonanceFooter = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="resonance">
			{({ ref, className }) => (
				<span
					ref={ref}
					data-scope="symbol"
					data-filter={focusSymbol}
					className={className}
				>
					α{" "}
					<span data-paint="alpha" data-paint-format=".4f">
						—
					</span>
					{" · surprise "}
					<span data-paint="surprise" data-paint-format=".2f">
						—
					</span>
				</span>
			)}
		</Component>
	);
};
