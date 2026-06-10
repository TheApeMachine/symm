import type { SignalGaugeBridge } from "#/components/charts/confidence/Gauges";

export const ingestGaugeWire = (
	bridge: SignalGaugeBridge | undefined,
	raw: Record<string, unknown>,
): void => {
	if (!bridge) {
		return;
	}

	if (bridge.ready) {
		bridge.update(raw);
		return;
	}

	bridge.pending.push(raw);
};
