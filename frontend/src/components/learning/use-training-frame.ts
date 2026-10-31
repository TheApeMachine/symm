import { useEffect, useState } from "react";
import { signals } from "#/collections/app";
import type { Value } from "#/types/value";

const frameKey = (frame: Value | null | undefined): string => {
	if (!frame) return "";
	return `${frame.tick ?? ""}:${frame.seqIdx ?? ""}:${frame.epoch ?? ""}`;
};

/*
One training frame per symbol; ignore store fan-out when the tick did not move.
*/
export const useTrainingFrame = (symbol: string): Value | null => {
	const [latestValue, setLatestValue] = useState<Value | null>(
		() => signals.training?.state?.[symbol] ?? null,
	);

	useEffect(() => {
		const apply = (state: Record<string, Value>) => {
			const frame = state[symbol];
			if (!frame) return;

			setLatestValue((prev) =>
				frameKey(prev) === frameKey(frame) ? prev : frame,
			);
		};

		apply(signals.training?.state ?? {});

		const unsub = signals.training?.subscribe(apply);
		return () => {
			unsub?.unsubscribe?.();
		};
	}, [symbol]);

	return latestValue;
};
