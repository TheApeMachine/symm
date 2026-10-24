let wsWorker: Worker | null = null;

export const bindHubWorker = (worker: Worker | null) => {
	wsWorker = worker;
};

export const sendRoute = (route: string) => {
	wsWorker?.postMessage({ type: "ROUTE", route });
};

export const sendFocus = (symbol: string) => {
	wsWorker?.postMessage({ type: "FOCUS", symbol });
};

export const sendCommand = (command: Record<string, unknown>) => {
	if (wsWorker === null) {
		throw new Error(
			`hub socket is not running: ${JSON.stringify(command)} not sent`,
		);
	}
	wsWorker.postMessage({ type: "COMMAND", command });
};

/** Best-effort hub command; false when the socket is not up. */
export const trySendCommand = (command: Record<string, unknown>): boolean => {
	try {
		sendCommand(command);
		return true;
	} catch {
		return false;
	}
};
