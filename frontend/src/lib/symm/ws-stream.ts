import type { SymmEvent } from "#/lib/symm/events";
import { setFeedConnected } from "#/lib/symm/dashboard-store";

export type StreamEventHandler = (event: SymmEvent) => void;

export type WsStreamOptions = {
	url: string;
	onEvent: StreamEventHandler;
	onOpen?: () => void;
};

export class WsStream {
	private readonly url: string;
	private readonly onEvent: StreamEventHandler;
	private readonly onOpen?: () => void;
	private socket: WebSocket | null = null;
	private started = false;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

	constructor(options: WsStreamOptions) {
		this.url = options.url;
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

		if (this.reconnectTimer !== null) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}

		this.closeSocket();
		setFeedConnected(false);
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
			setFeedConnected(true);
			this.onOpen?.();
		};

		socket.onclose = () => {
			setFeedConnected(false);

			if (!this.started) {
				return;
			}

			this.reconnectTimer = setTimeout(() => {
				this.reconnectTimer = null;

				if (this.started) {
					this.connect();
				}
			}, 2000);
		};

		socket.onerror = () => {
			socket.close();
		};

		socket.onmessage = (message) => {
			try {
				const event = JSON.parse(String(message.data)) as SymmEvent;
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
