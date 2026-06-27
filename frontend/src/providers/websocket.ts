import { useEffect } from "react";

import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { cognitiveStore } from "#/collections/cognitive";
import { decisionsStore } from "#/collections/decisions";
import { executionsStore } from "#/collections/executions";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { positionsStore } from "#/collections/positions";
import { playbookStore } from "#/collections/playbook";
import { resonanceStore } from "#/collections/resonance";
import { tickStore } from "#/collections/tick";
import { decodePackedArtifactWire } from "#/lib/capnp/read-artifact";

const socketUrl =
  import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const WIRE_ERROR_LOG_INTERVAL_MS = 5000;
const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;

let lastWireErrorAt = 0;
let pendingMeasurements: Record<string, unknown>[] = [];
let measurementFlushScheduled = false;

const flushMeasurements = () => {
  measurementFlushScheduled = false;

  if (pendingMeasurements.length === 0) {
    return;
  }

  const frames = pendingMeasurements;
  pendingMeasurements = [];
  measurementsStore.actions.updateReadings(frames);
};

const scheduleMeasurementFlush = () => {
  if (measurementFlushScheduled) {
    return;
  }

  measurementFlushScheduled = true;

  if (typeof requestAnimationFrame === "function") {
    requestAnimationFrame(flushMeasurements);
    return;
  }

  setTimeout(flushMeasurements, 16);
};

const queueMeasurement = (frame: Record<string, unknown>) => {
  pendingMeasurements.push(frame);
  scheduleMeasurementFlush();
};

const routes: Record<string, (frame: Record<string, unknown>) => void> = {
  tick: (frame) => {
    tickStore.actions.updateFrame(frame);
    const count = frame.count;
    const phase = frame.phase;
    const candidates = frame.candidates;
    if (typeof count === "number") {
      appStore.actions.updateStoryTicks(count);
    }
    if (typeof phase === "string") {
      appStore.actions.updateEnginePhase(phase);
    }
    if (typeof candidates === "number") {
      appStore.actions.updatePlaybookEvaluations(candidates);
    }
  },
  measurement: (frame) => {
    queueMeasurement(frame);
  },
  resonance: resonanceStore.actions.updateFrame,
  balances: balancesStore.actions.updateFrame,
  executions: executionsStore.actions.updateFrame,
  fill: executionsStore.actions.updateFrame,
  quote: executionsStore.actions.updateFrame,
  orders: ordersStore.actions.updateFrame,
  order: ordersStore.actions.updateFrame,
  stoploss: ordersStore.actions.updateFrame,
  regime: appStore.actions.stashRegimeFrame,
  manifold: appStore.actions.stashManifoldFrame,
  decision: decisionsStore.actions.updateFrame,
  decisions: decisionsStore.actions.updateFrame,
  positions: positionsStore.actions.updateFrame,
  walk: playbookStore.actions.updateFrame,
  cognitive: cognitiveStore.actions.updateFrame,
};

const wireBufferFromMessage = async (
  data: MessageEvent["data"],
): Promise<ArrayBuffer | null> => {
  if (data instanceof ArrayBuffer) {
    return data;
  }

  if (data instanceof Blob) {
    return data.arrayBuffer();
  }

  return null;
};

export const WsFeed = () => {
  const { updateOnline } = appStore.actions;

  useEffect(() => {
    let closedByUnmount = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let attempt = 0;
    let socket: WebSocket | null = null;

    const scheduleReconnect = () => {
      if (closedByUnmount || reconnectTimer !== null) {
        return;
      }

      const delay = Math.min(
        RECONNECT_MAX_MS,
        RECONNECT_BASE_MS * 2 ** attempt,
      );
      attempt += 1;
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connect();
      }, delay);
    };

    const connect = () => {
      socket = new WebSocket(socketUrl);
      socket.binaryType = "arraybuffer";

      socket.addEventListener("open", () => {
        attempt = 0;
        updateOnline(true);
      });

      socket.addEventListener("close", () => {
        updateOnline(false);
        scheduleReconnect();
      });

      socket.addEventListener("error", () => {
        socket?.close();
      });

      socket.addEventListener("message", (event) => {
        void (async () => {
          try {
            const buffer = await wireBufferFromMessage(event.data);

            if (buffer === null) {
              return;
            }

            const frame = await decodePackedArtifactWire(buffer);

            if (frame === null) {
              return;
            }

            const route = routes[frame.role as string];

            route?.(frame);
          } catch (error) {
            const now = Date.now();

            if (now - lastWireErrorAt < WIRE_ERROR_LOG_INTERVAL_MS) {
              return;
            }

            lastWireErrorAt = now;
            console.error("websocket frame parse failed", error);
          }
        })();
      });
    };

    connect();

    return () => {
      closedByUnmount = true;
      if (reconnectTimer !== null) {
        clearTimeout(reconnectTimer);
      }
      socket?.close();
    };
  }, [updateOnline]);

  return null;
};
