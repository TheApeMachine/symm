import { describe, expect, it } from "vitest";
import { decodePackedArtifactWire } from "./read-artifact";

const describeLive = process.env.SYMM_LIVE_WS_TEST === "1" ? describe : describe.skip;
const liveWebsocketUrl =
	process.env.SYMM_LIVE_WS_URL ?? "ws://127.0.0.1:8765/ws";
const configuredQuote = process.env.SYMM_LIVE_QUOTE ?? "USD";
const liveRegimeTimeoutMs = 20000;
const liveMarketTimeoutMs = 60000;

const symbolQuote = (symbol: string): string | null => {
	const slash = symbol.indexOf("/");

	if (slash < 0 || slash === symbol.length - 1) {
		return null;
	}

	return symbol.slice(slash + 1).toUpperCase();
};

const assertConfiguredQuote = (symbol: unknown) => {
	if (typeof symbol !== "string") {
		return false;
	}

	const quote = symbolQuote(symbol);

	if (quote === null) {
		return false;
	}

	expect(quote).toBe(configuredQuote);

	return true;
};

const frameSymbols = (frame: Record<string, unknown>): string[] => {
	const symbols: string[] = [];

	if (typeof frame.scope === "string" && symbolQuote(frame.scope) !== null) {
		symbols.push(frame.scope);
	}

	const decisions = frame.decisions;
	if (Array.isArray(decisions)) {
		for (const decision of decisions) {
			if (
				decision !== null &&
				typeof decision === "object" &&
				typeof (decision as Record<string, unknown>).symbol === "string"
			) {
				symbols.push((decision as Record<string, unknown>).symbol as string);
			}
		}
	}

	const evaluations = frame.evaluations;
	if (evaluations !== null && typeof evaluations === "object") {
		for (const [symbol, trace] of Object.entries(
			evaluations as Record<string, unknown>,
		)) {
			if (symbolQuote(symbol) !== null) {
				symbols.push(symbol);
			}

			if (
				trace !== null &&
				typeof trace === "object" &&
				typeof (trace as Record<string, unknown>).symbol === "string"
			) {
				symbols.push((trace as Record<string, unknown>).symbol as string);
			}
		}
	}

	const readings = frame.readings;
	if (readings !== null && typeof readings === "object") {
		for (const [symbol, reading] of Object.entries(
			readings as Record<string, unknown>,
		)) {
			if (symbolQuote(symbol) !== null) {
				symbols.push(symbol);
			}

			if (
				reading !== null &&
				typeof reading === "object" &&
				typeof (reading as Record<string, unknown>).scope === "string"
			) {
				symbols.push((reading as Record<string, unknown>).scope as string);
			}
		}
	}

	return symbols;
};

describeLive("Live WebSocket invariants", () => {
	it("publishes a backend regime frame", async () => {
		const ws = new WebSocket(liveWebsocketUrl);
		ws.binaryType = "arraybuffer";

		const frame = await new Promise<Record<string, unknown>>((resolve, reject) => {
			const timeout = setTimeout(() => {
				ws.close();
				reject(new Error("timed out waiting for regime frame"));
			}, liveRegimeTimeoutMs);

			ws.onmessage = async (event) => {
				try {
					const decoded = await decodePackedArtifactWire(
						event.data as ArrayBuffer,
					);

					if (decoded?.role !== "regime") {
						return;
					}

					clearTimeout(timeout);
					ws.close();
					resolve(decoded);
				} catch (error) {
					clearTimeout(timeout);
					ws.close();
					reject(error);
				}
			};

			ws.onerror = (error) => {
				clearTimeout(timeout);
				reject(error);
			};
		});

		expect(frame.origin).toBe("regime");
		expect(frame.scope).toBe("regime");
		for (const axis of [
			"volatility",
			"trend",
			"bullish",
			"bearish",
			"choppiness",
		]) {
			expect(typeof frame[axis]).toBe("number");
		}
		expect((frame.output as Record<string, unknown>).status).toBe("measured");
	}, liveRegimeTimeoutMs + 2000);

	it("publishes ticks, configured-quote symbols, and measurement frames", async () => {
		const ws = new WebSocket(liveWebsocketUrl);
		ws.binaryType = "arraybuffer";

		const observed = await new Promise<{
			measurement: boolean;
			regime: boolean;
			symbolFrames: number;
			tick: boolean;
		}>((resolve, reject) => {
			const state = {
				measurement: false,
				regime: false,
				symbolFrames: 0,
				tick: false,
			};
			const timeout = setTimeout(() => {
				ws.close();
				reject(
					new Error(
						`timed out waiting for live invariant frames: ${JSON.stringify(state)}`,
					),
				);
			}, liveMarketTimeoutMs);
			const finishIfReady = () => {
				if (
					!state.tick ||
					!state.measurement ||
					!state.regime ||
					state.symbolFrames === 0
				) {
					return;
				}

				clearTimeout(timeout);
				ws.close();
				resolve(state);
			};

			ws.onmessage = async (event) => {
				try {
					const buffer = event.data as ArrayBuffer;
					const frame = await decodePackedArtifactWire(buffer);

					if (frame === null) {
						return;
					}

					for (const symbol of frameSymbols(frame)) {
						if (assertConfiguredQuote(symbol)) {
							state.symbolFrames += 1;
						}
					}

					if (frame.role === "tick") {
						state.tick = true;
					}

					if (frame.role === "measurement") {
						expect(typeof frame.origin).toBe("string");
						expect(typeof frame.scope).toBe("string");
						state.measurement = true;
					}

					if (frame.role === "regime") {
						expect(frame.origin).toBe("regime");
						expect(frame.scope).toBe("regime");
						expect(typeof frame.volatility).toBe("number");
						expect(typeof frame.trend).toBe("number");
						state.regime = true;
					}

					finishIfReady();
				} catch (error) {
					clearTimeout(timeout);
					ws.close();
					reject(error);
				}
			};

			ws.onerror = (error) => {
				clearTimeout(timeout);
				reject(error);
			};
		});

		expect(observed.tick).toBe(true);
		expect(observed.measurement).toBe(true);
		expect(observed.regime).toBe(true);
		expect(observed.symbolFrames).toBeGreaterThan(0);
	}, liveMarketTimeoutMs + 2000);
});
