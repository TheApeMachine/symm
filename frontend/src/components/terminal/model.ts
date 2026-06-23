import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { balanceStore } from "#/collections/balance";
import { cognitiveStore, type CognitiveReading } from "#/collections/cognitive";
import { playbookStore } from "#/collections/playbook";
import {
  confidenceMeterValue,
  healthMeterValue,
  SIGNAL_SOURCES,
  type SignalHealthStatus,
  type SignalReading,
  signalHealthStatus,
  signalStore,
  surpriseMeterValue,
} from "#/collections/signals";
import { statusStore } from "#/collections/status";
import { AuditDataProvider } from "#/components/panels/data/audit-data-provider";
import { DecisionsDataProvider } from "#/components/panels/data/decisions-data-provider";
import { WalletDataProvider } from "#/components/panels/data/wallet-data-provider";
import {
  positionPercent,
  positionPrice,
  positionQuantity,
  signedPositionMoney,
} from "#/components/panels/positions";
import type { DecisionTraceEvent } from "#/lib/symm/events";
import { whyLabel } from "#/lib/symm/events";
import { useStoreSnapshot } from "#/lib/symm/use-store-snapshot";

export type TerminalSurface =
  | "dashboard"
  | "signals"
  | "decisions"
  | "xray"
  | "cortex"
  | "allocation";

export type TerminalKernel = {
  source: string;
  name: string;
  category: string;
  status: SignalHealthStatus;
  statusLabel: string;
  strengthText: string;
  confidencePercent: number;
  surprisePercent: number;
  healthPercent: number;
  confidenceText: string;
  surpriseText: string;
  samplesText: string;
  activeText: string;
  observedText: string;
  faultText: string;
};

export type TerminalDecisionRow = {
  key: string;
  symbol: string;
  source: string;
  scoreText: string;
  scoreValue: number;
  verdict: "allow" | "blocked" | "in-play";
  why: string;
  signals: Array<{
    source: string;
    confidence: number;
  }>;
};

export type TerminalPositionRow = {
  key: string;
  symbol: string;
  pnlText: string;
  pnlPercentText: string;
  priceText: string;
  quantityText: string;
  priced: boolean;
  profitable: boolean;
};

export type TerminalModel = {
  online: boolean;
  clockText: string;
  wallet: {
    cash: string;
    available: string;
    reserved: string;
    tick: string;
    openText: string;
  };
  engine: {
    phase: string;
    sequence: string;
    measurements: number;
    candidates: number;
    open: number;
    signalsText: string;
    signalsPercent: number;
    fluidText: string;
    fluidPercent: number;
  };
  health: {
    healthy: number;
    total: number;
    averageConfidence: number;
    firing: number;
    warming: number;
    degraded: number;
    label: string;
  };
  kernels: TerminalKernel[];
  decisions: TerminalDecisionRow[];
  positions: TerminalPositionRow[];
  totalPnlText: string;
  totalPnlPositive: boolean;
  auditRows: ReturnType<typeof AuditDataProvider.snapshot>;
  cognitive: CognitiveReading | null;
  cognitiveScopes: string[];
  playbookBranches: number;
  walkSymbol: string;
};

type DecisionRow = NonNullable<DecisionTraceEvent["decisions"]>[number];

const TERMINAL_KERNEL_LABELS: Record<string, string> = {
  causal: "Causal ladder",
  correlation: "Correlation field",
  cvd: "CVD pressure",
  depthflow: "Depth flow",
  exhaustion: "Exhaustion",
  fluid: "Fluid dynamics",
  hawkes: "Hawkes process",
  leadlag: "Lead-lag",
  liquidity: "Liquidity",
  manifold: "Manifold",
  pumpdump: "Pump impulse",
  sentiment: "Sentiment",
  toxicity: "Toxicity",
  prediction: "Predictive coding",
  resonance: "Resonance",
};

const percent = (value: number): number =>
  Math.round(Math.min(100, Math.max(0, value)));

const fixed = (value: number, digits: number): string => {
  if (!Number.isFinite(value)) {
    return "0";
  }

  return value.toFixed(digits);
};

const money = (symbol: string, value: number): string => {
  if (symbol.length === 1) {
    return `${symbol}${fixed(value, 2)}`;
  }

  return `${fixed(value, 2)} ${symbol}`;
};

const statusLabel = (status: SignalHealthStatus): string => {
  switch (status) {
    case "calibrating":
      return "cal";
    case "healthy":
      return "ok";
    default:
      return status;
  }
};

const observedText = (reading: SignalReading | undefined): string => {
  if (reading === undefined || reading.observedAt === null) {
    return "waiting";
  }

  if (reading.elapsed <= 0) {
    return "observed";
  }

  return `${fixed(reading.elapsed, 2)}s`;
};

export const terminalKernelsFromReadings = (
  readings: Record<string, SignalReading>,
): TerminalKernel[] =>
  SIGNAL_SOURCES.map((source) => {
    const reading = readings[source];
    const status = signalHealthStatus(reading ?? null);
    const minSamples = reading?.minSamples ?? 0;
    const samples = reading?.samples ?? 0;

    return {
      source,
      name: TERMINAL_KERNEL_LABELS[source] ?? source,
      category: reading?.category || source,
      status,
      statusLabel: statusLabel(status),
      strengthText: reading ? fixed(reading.strength, 4) : "0.0000",
      confidencePercent: percent(reading ? confidenceMeterValue(reading) : 0),
      surprisePercent: percent(reading ? surpriseMeterValue(reading) : 0),
      healthPercent: percent(reading ? healthMeterValue(reading) : 0),
      confidenceText: reading ? fixed(reading.confidence, 2) : "0.00",
      surpriseText: reading ? fixed(reading.surprise, 2) : "0.00",
      samplesText: minSamples > 0 ? `${samples}/${minSamples}` : `${samples}`,
      activeText: reading
        ? `${reading.activeReadings}/${reading.readingsCapacity}`
        : "0/0",
      observedText: observedText(reading),
      faultText:
        reading?.gapReason || (reading?.bestEffort ? "best-effort" : ""),
    };
  });

export const terminalDecisionRows = (
  decisions: DecisionTraceEvent["decisions"] | undefined,
): TerminalDecisionRow[] =>
  [...(decisions ?? [])]
    .sort((left, right) => right.score - left.score)
    .slice(0, 16)
    .map((decision: DecisionRow) => {
      const verdict = decision.allow
        ? decision.in_play
          ? "in-play"
          : "allow"
        : "blocked";

      return {
        key: `${decision.symbol}:${decision.source ?? "decision"}`,
        symbol: decision.symbol,
        source: decision.source ?? "decision",
        scoreText: fixed(decision.score, 3),
        scoreValue: decision.score,
        verdict,
        why: whyLabel(decision.why),
        signals: (decision.signals ?? [])
          .filter(
            (signal) =>
              Number.isFinite(signal.confidence) && signal.source.trim() !== "",
          )
          .map((signal) => ({
            source: signal.source,
            confidence: signal.confidence,
          })),
      };
    });

export const useTerminalModel = (): TerminalModel => {
  const appState = useSelector(appStore, (state) => state);
  const balanceState = useSelector(balanceStore, (state) => state);
  const signalState = useSelector(signalStore, (state) => state);
  const statusState = useSelector(statusStore, (state) => state);
  const cognitiveState = useSelector(cognitiveStore, (state) => state);
  const playbookState = useSelector(playbookStore, (state) => state);
  const walletState = useStoreSnapshot(WalletDataProvider);
  const decisionTrace = useStoreSnapshot(DecisionsDataProvider);
  const auditRows = useStoreSnapshot(AuditDataProvider);

  const kernels = terminalKernelsFromReadings(signalState.readings);
  const healthy = kernels.filter(
    (kernel) => kernel.status === "healthy",
  ).length;
  const warming = kernels.filter(
    (kernel) => kernel.status === "calibrating",
  ).length;
  const degraded = kernels.filter(
    (kernel) => kernel.status === "fault" || kernel.status === "stale",
  ).length;
  const firing = kernels.filter(
    (kernel) => kernel.surprisePercent >= 60,
  ).length;
  const averageConfidence =
    kernels.length > 0
      ? Math.round(
          kernels.reduce((sum, kernel) => sum + kernel.confidencePercent, 0) /
            kernels.length,
        )
      : 0;
  const signalCount = Object.keys(signalState.readings).length;
  const pumpReading = signalState.readings.pumpdump;
  const positions = statusState.positionViews.map((position) => {
    const profitable = position.priced && position.unrealized >= 0;

    return {
      key: position.symbol,
      symbol: position.symbol,
      pnlText: position.priced
        ? signedPositionMoney(position.unrealized, balanceState.symbol)
        : "pricing",
      pnlPercentText: position.priced
        ? positionPercent(position.unrealizedPct)
        : "waiting",
      priceText: position.priced
        ? `${positionPrice(position.avgEntry, balanceState.symbol)} -> ${positionPrice(position.mark, balanceState.symbol)}`
        : positionPrice(position.avgEntry, balanceState.symbol),
      quantityText: positionQuantity(position.qty),
      priced: position.priced,
      profitable,
    };
  });
  const totalPnl = statusState.positionViews.reduce(
    (sum, position) => sum + (position.priced ? position.unrealized : 0),
    0,
  );
  const selectedCognitive =
    cognitiveState.selectedScope !== ""
      ? (cognitiveState.readings[cognitiveState.selectedScope] ?? null)
      : null;
  const cognitiveScopes = Object.keys(cognitiveState.readings).sort();

  return {
    online: appState.online,
    clockText: new Date().toISOString().slice(11, 19),
    wallet: {
      cash:
        walletState.balance > 0
          ? money(walletState.currency, walletState.balance)
          : balanceState.balanceLabel,
      available: money(balanceState.symbol, balanceState.liquidationBalance),
      reserved: money(walletState.currency, walletState.reservedEur),
      tick: appState.storyTicks.toString(),
      openText: `${balanceState.openPositions} open positions`,
    },
    engine: {
      phase: appState.online ? (decisionTrace?.type ?? "stream") : "offline",
      sequence: `#${appState.storyTicks}`,
      measurements: Object.keys(appState.lastGaugeFrames).length,
      candidates: decisionTrace?.decisions?.length ?? 0,
      open: balanceState.openPositions,
      signalsText: `${signalCount}/${SIGNAL_SOURCES.length}`,
      signalsPercent: percent((signalCount / SIGNAL_SOURCES.length) * 100),
      fluidText: pumpReading ? pumpReading.samples.toString() : "0",
      fluidPercent: percent(
        pumpReading ? confidenceMeterValue(pumpReading) : 0,
      ),
    },
    health: {
      healthy,
      total: kernels.length,
      averageConfidence,
      firing,
      warming,
      degraded,
      label:
        degraded > 0
          ? "Degraded"
          : healthy < kernels.length / 2
            ? "Thin"
            : "Nominal",
    },
    kernels,
    decisions: terminalDecisionRows(decisionTrace?.decisions),
    positions,
    totalPnlText: signedPositionMoney(totalPnl, balanceState.symbol),
    totalPnlPositive: totalPnl >= 0,
    auditRows,
    cognitive: selectedCognitive,
    cognitiveScopes,
    playbookBranches: playbookState.branches.length,
    walkSymbol: playbookState.walkTrace?.symbol ?? "",
  };
};
