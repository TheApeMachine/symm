/*
hubBaseUrl locates the hub's REST endpoints.

The hub serves its reads from the same origin it serves the websocket from, so
the REST origin is derived from the websocket URL rather than configured twice:
an override names the socket, and the default reaches the hub's own port. A
browser pointed at "localhost" is sent to 127.0.0.1 so the request cannot be
resolved onto an IPv6 loopback the hub is not listening on.
*/
export const hubBaseUrl = () => {
	if (import.meta.env.VITE_SYMM_WS_URL) {
		return import.meta.env.VITE_SYMM_WS_URL.replace(/^ws/, "http").replace(
			/\/ws$/,
			"",
		);
	}

	const protocol = window.location.protocol === "https:" ? "https:" : "http:";
	const host =
		!window.location.hostname || window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;

	return `${protocol}//${host}:8765`;
};
