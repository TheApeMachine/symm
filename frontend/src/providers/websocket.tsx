import { atoms, signals } from "#/collections/app";
import {
	ensureLongtailRecorder,
	requestLongtailSources,
	resetLongtailSourcesRequest,
} from "#/collections/longtail";
import { hubWsUrl } from "#/lib/hub";
import type { Position } from "#/collections/types";
import { resetClosedHistoryRequest } from "#/lib/lifecycle";
import { batch } from "@tanstack/store";
import { useEffect } from "react";

type HubStatus = "ONLINE" | "OFFLINE" | "CONNECTING";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 10_000;

let socket: WebSocket | null = null;
let connectUrl = "";
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempts = 0;
let socketGeneration = 0;
let onEnvelope: ((payload: Record<string, unknown>) => void) | null = null;
let onStatus: ((status: HubStatus) => void) | null = null;
let onError: ((error: string) => void) | null = null;

let currentRoute = "dashboard";
let pendingCommands: Record<string, unknown>[] = [];

const queueCommand = (command: Record<string, unknown>) => {
	if (command.type === "hindsight") {
		pendingCommands = pendingCommands.filter(
			(item) => item.type !== "hindsight",
		);
	}

	pendingCommands.push(command);
};

const flushCommands = () => {
	if (socket === null || socket.readyState !== WebSocket.OPEN) {
		return;
	}

	for (const command of pendingCommands) {
		socket.send(JSON.stringify(command));
	}

	pendingCommands = [];
};

const teardownSocket = () => {
	socketGeneration += 1;

	if (
		socket !== null &&
		(socket.readyState === WebSocket.OPEN ||
			socket.readyState === WebSocket.CONNECTING)
	) {
		socket.close();
	}

	socket = null;
};

const scheduleReconnect = () => {
	if (reconnectTimer !== null || onEnvelope === null) {
		return;
	}

	const delay = Math.min(
		RECONNECT_BASE_MS * 2 ** reconnectAttempts,
		RECONNECT_MAX_MS,
	);

	reconnectTimer = setTimeout(() => {
		reconnectTimer = null;
		reconnectAttempts += 1;
		openSocket(connectUrl);
	}, delay);
};

const openSocket = (url: string) => {
	if (url === "" || onEnvelope === null) {
		return;
	}

	teardownSocket();

	connectUrl = url;
	const generation = socketGeneration;
	onStatus?.("CONNECTING");
	socket = new WebSocket(url);

	socket.addEventListener("open", () => {
		if (generation !== socketGeneration) {
			return;
		}

		reconnectAttempts = 0;
		onStatus?.("ONLINE");
		if (currentRoute) {
			sendRoute(currentRoute);
		}
		flushCommands();
	});

	socket.addEventListener("close", () => {
		if (generation !== socketGeneration) {
			return;
		}

		onStatus?.("OFFLINE");
		scheduleReconnect();
	});

	socket.addEventListener("error", (event) => {
		if (generation !== socketGeneration) {
			return;
		}

		onError?.(String(event));
	});

	socket.addEventListener("message", (event) => {
		if (typeof event.data !== "string" || event.data.length === 0) {
			return;
		}

		try {
			onEnvelope?.(JSON.parse(event.data) as Record<string, unknown>);
		} catch (err) {
			console.error("WS envelope parse failed:", err);
			onError?.(String(err));
		}
	});
};

const connectHub = (
	url: string,
	handlers: {
		onEnvelope: (payload: Record<string, unknown>) => void;
		onStatus: (status: HubStatus) => void;
		onError?: (error: string) => void;
	},
) => {
	onEnvelope = handlers.onEnvelope;
	onStatus = handlers.onStatus;
	onError = handlers.onError ?? null;
	openSocket(url);
};

const disconnectHub = () => {
	if (reconnectTimer !== null) {
		clearTimeout(reconnectTimer);
		reconnectTimer = null;
	}

	reconnectAttempts = 0;
	pendingCommands = [];
	onEnvelope = null;
	onStatus = null;
	onError = null;
	teardownSocket();
};

export const sendRoute = (route: string) => {
	currentRoute = route;
	if (
		socket !== null &&
		socket.readyState === WebSocket.OPEN &&
		typeof route === "string"
	) {
		socket.send(JSON.stringify({ type: "route", route }));
	}
};

export const sendFocus = (symbol: string) => {
	if (
		socket !== null &&
		socket.readyState === WebSocket.OPEN &&
		typeof symbol === "string"
	) {
		socket.send(JSON.stringify({ type: "focus", symbol }));
	}
};

export const sendCommand = (command: Record<string, unknown>) => {
	if (onEnvelope === null) {
		throw new Error(
			`hub socket is not running: ${JSON.stringify(command)} not sent`,
		);
	}

	if (!command) {
		throw new Error("hub command is empty");
	}

	if (socket !== null && socket.readyState === WebSocket.OPEN) {
		socket.send(JSON.stringify(command));
		return;
	}

	queueCommand(command);
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

const applyEnvelope = (payload: Record<string, unknown>) => {
	batch(() => {
		for (const key of Object.keys(payload)) {
			const value = payload[key];

			const atom = atoms[key];
			
			if (atom) {
				atom.set(value);
				continue;
			}

			const store = signals[key];

			if (!store) {
				continue;
			}

			if (key === "positions" && payload.positions_snapshot === true) {
				store.setState(() => value as Record<string, Position>);
				continue;
			}

			store.setState((prev: Record<string, unknown> | null) => ({
				...prev,
				...(value as Record<string, unknown>),
			}));
		}
	});
};

export const WsFeed = () => {
	useEffect(() => {
		ensureLongtailRecorder();

		connectHub(`${hubWsUrl()}/ws`, {
			onEnvelope: applyEnvelope,
			onStatus: (status) => {
				atoms.online.set(status);

				if (status === "ONLINE") {
					resetClosedHistoryRequest();
					resetLongtailSourcesRequest();
					requestLongtailSources();
				}
			},
			onError: (error) => {
				console.error("WS error:", error);
			},
		});

		sendRoute(atoms.route.get());

		const unsubscribeFocus = atoms.focus.subscribe((symbol: string) => {
			sendFocus(symbol);
		});

		const unsubscribeRoute = atoms.route.subscribe((route: string) => {
			sendRoute(route);
		});

		return () => {
			unsubscribeFocus.unsubscribe();
			unsubscribeRoute.unsubscribe();
			disconnectHub();
		};
	}, []);

	return null;
};
