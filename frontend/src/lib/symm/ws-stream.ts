import type { SymmEvent, SymmEventName } from "#/lib/symm/events";
import {
	setStreamConnected,
	type StreamName,
} from "#/lib/symm/stores/connection-store";

export type StreamEventHandler = (event: SymmEvent) => void;

export type WsStreamOptions = {
	url: string;
	stream: StreamName;
	accepts: ReadonlySet<SymmEventName>;
	onEvent: StreamEventHandler;
	onOpen?: () => void;
};

export class WsStream {
	private readonly url: string;
	private readonly stream: StreamName;
	private readonly accepts: ReadonlySet<SymmEventName>;
	private readonly onEvent: StreamEventHandler;
	private readonly onOpen?: () => void;
	private socket: WebSocket | null = null;
	private started = false;

	constructor(options: WsStreamOptions) {
		this.url = options.url;
		this.stream = options.stream;
		this.accepts = options.accepts;
		this.onEvent = options.onEvent;
		this.onOpen = options.onOpen;
	}

	start(): void {
		if (this.started) {
			return;
		}

		this.started = true;
		this.connect();
	}

	stop(): void {
		this.started = false;
		this.closeSocket();
		setStreamConnected(this.stream, false);
	}

	send(payload: unknown): void {
		if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
			return;
		}

		this.socket.send(JSON.stringify(payload));
	}

	private connect(): void {
		this.closeSocket();

		const socket = new WebSocket(this.url);
		this.socket = socket;

		socket.onopen = () => {
			setStreamConnected(this.stream, true);
			this.onOpen?.();
		};

		socket.onclose = () => {
			setStreamConnected(this.stream, false);

			if (this.started) {
				setTimeout(() => this.connect(), 2000);
			}
		};

		socket.onerror = () => {
			socket.close();
		};

		socket.onmessage = (message) => {
			try {
				const event = JSON.parse(String(message.data)) as SymmEvent;
				if (!this.accepts.has(event.event)) {
					return;
				}

				this.onEvent(event);
			} catch {
				// ignore malformed frames
			}
		};
	}

	private closeSocket(): void {
		if (!this.socket) {
			return;
		}

		this.socket.onopen = null;
		this.socket.onclose = null;
		this.socket.onerror = null;
		this.socket.onmessage = null;
		this.socket.close();
		this.socket = null;
	}
}
