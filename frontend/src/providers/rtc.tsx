/*
WebRTC transport has been removed. Resonance, diagnostics, and manifold
artifacts ride the hub WebSocket (see providers/websocket.tsx).

dispatchMeasurementsBuffer remains exported here so existing tests that import
from "./rtc" keep working without a second implementation.
*/

export { dispatchMeasurementsBuffer } from "#/providers/websocket";

/** @deprecated No-op: WebRTC feed removed; use WsFeed. */
export const RtcFeed = () => null;
