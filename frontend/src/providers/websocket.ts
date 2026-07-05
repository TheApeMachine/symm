import { useEffect } from "react";
import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { cognitiveStore } from "#/collections/cognitive";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { tickStore } from "#/collections/tick";

const socketUrl =
  import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

const RECONNECT_BASE_MS = 500;
const RECONNECT_MAX_MS = 5000;

type DashboardMessage = {
  tick?: Record<string, unknown>;
  instrument?: Record<string, unknown>;
  manifold?: Record<string, unknown>;
  measurement?: Record<string, unknown>;
  regime?: Record<string, unknown>;
  resonance?: Record<string, unknown>;
  balances?: Record<string, unknown>;
  orders?: Record<string, unknown>;
  executions?: Record<string, unknown>;
  positions?: Record<string, unknown>;
  decision?: Record<string, unknown>;
  diagnostic?: Record<string, unknown>;
  cognitive?: Record<string, unknown>;
};

const frameRows = (frame: Record<string, unknown> | undefined) => {
  const rows = frame?.rows;

  return Array.isArray(rows)
    ? rows.filter(
        (row): row is Record<string, unknown> =>
          row !== null && typeof row === "object" && !Array.isArray(row),
      )
    : [];
};

export const routeMessage = (message: DashboardMessage) => {
  if (message.tick) {
    tickStore.actions.updateFrame(message.tick);
    decisionStore.actions.observeTick(message.tick.count);
  }

  if (message.instrument) {
    instrumentsStore.actions.updateFrame(message.instrument);
  }

  if (message.manifold) {
    manifoldStore.actions.updateFrame(message.manifold);
  }

  if (message.measurement) {
    measurementsStore.actions.updateFrame(message.measurement);
  }

  if (message.regime) {
    measurementsStore.actions.updateFrame({
      ...message.regime,
      source: "regime",
      symbol: "regime",
      category: "regime",
      status: "measured",
    });
  }

  if (message.resonance) {
    resonanceStore.actions.updateFrame(message.resonance);
  }

  if (message.balances) {
    balancesStore.actions.updateFrame(message.balances);
  }

  if (message.executions) {
    const rows = frameRows(message.executions);

    if (rows.length > 0) {
      executionsStore.actions.updateFrames(rows);
    }
  }

  if (message.orders) {
    ordersStore.actions.updateFrame(message.orders);
  }

  if (message.positions) {
    positionsStore.actions.updateFrame(message.positions);
  }

  if (message.decision) {
    decisionStore.actions.updateFrame(message.decision);
  }

  if (message.diagnostic) {
    diagnosticsStore.actions.updateFrame(message.diagnostic);
  }

  if (message.cognitive) {
    cognitiveStore.actions.updateFrame(message.cognitive);
  }
};

export const WsFeed = () => {
  const { updateOnline, updateError } = appStore.actions;

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

      socket.addEventListener("open", () => {
        if (closedByUnmount) {
          socket?.close();
          return;
        }

        attempt = 0;
        updateOnline(true);
      });

      socket.addEventListener("close", () => {
        if (closedByUnmount) {
          return;
        }

        updateOnline(false);
        scheduleReconnect();
      });

      socket.addEventListener("error", () => {
        if (socket?.readyState === WebSocket.OPEN) {
          socket.close();
        }
      });

      socket.addEventListener("message", (event) => {
        try {
          routeMessage(JSON.parse(String(event.data)) as DashboardMessage);
        } catch (err) {
          console.error(err);
          updateError({ err: err });
        }
      });
    };

    connect();

    return () => {
      closedByUnmount = true;

      if (reconnectTimer !== null) {
        clearTimeout(reconnectTimer);
      }

      if (socket?.readyState === WebSocket.OPEN) {
        socket.close();
      }
    };
  }, [updateOnline, updateError]);

  return null;
};
