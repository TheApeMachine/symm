import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useMemo, useRef } from "react";
import {
  type CognitiveReading,
  cognitiveScopes,
  cognitiveStore,
} from "#/collections/cognitive";
import { appStore } from "#/collections/app";
import { instrumentsStore } from "#/collections/instruments";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import {
  clearCanvas,
  drawGrid,
  resizeCanvas,
  TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { XrayLayerRows } from "#/components/terminal/xray-layers";

type Draw = (
  context: CanvasRenderingContext2D,
  width: number,
  height: number,
) => void;

type HawkesMetrics = {
  intensity: number | null;
  branching: number | null;
  radius: number | null;
  asymmetry: number | null;
  buyIntensity: number | null;
  sellIntensity: number | null;
  exo: number | null;
};

type HawkesSample = {
  key: string;
  symbol: string;
  intensity: number;
};

type LatentPoint = {
  key: string;
  symbol: string;
  x: number;
  y: number;
  category: string;
};

const asRecord = (value: unknown): Record<string, unknown> | null =>
  value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;

const recordArray = (value: unknown): Record<string, unknown>[] =>
  Array.isArray(value)
    ? value.flatMap((item) => {
        const record = asRecord(item);
        return record === null ? [] : [record];
      })
    : [];

const finite = (value: unknown): number | null =>
  typeof value === "number" && Number.isFinite(value) ? value : null;

const numberArray = (value: unknown): number[] =>
  Array.isArray(value)
    ? value.filter((item): item is number => typeof item === "number")
    : [];

const numberMatrix = (value: unknown): number[][] =>
  Array.isArray(value)
    ? value.map((row) => numberArray(row)).filter((row) => row.length > 0)
    : [];

const stringValue = (value: unknown): string =>
  typeof value === "string" ? value.trim() : "";

const isConcreteSymbol = (symbol: string): boolean =>
  symbol !== "";

const format = (value: number | null, digits = 3): string =>
  value === null ? "—" : value.toFixed(digits);

const signed = (value: number | null, digits = 3): string => {
  if (value === null) {
    return "—";
  }

  return `${value >= 0 ? "+" : "−"}${Math.abs(value).toFixed(digits)}`;
};

const outputOf = (
  frame: unknown,
): Record<string, unknown> =>
  {
    const record = asRecord(frame);

    return {
      ...(record ?? {}),
      ...(asRecord(record?.metrics) ?? {}),
      ...(asRecord(record?.output) ?? {}),
    } as Record<string, unknown>;
  };

const outputNumber = (
  frame: unknown,
  key: string,
): number | null => {
  const output = outputOf(frame);

  return finite(output[key]);
};

const hawkesMetrics = (
  frame: Record<string, unknown> | undefined,
): HawkesMetrics => ({
  intensity:
    outputNumber(frame, "intensity") ??
    outputNumber(frame, "intensityRatio") ??
    outputNumber(frame, "strength"),
  branching:
    outputNumber(frame, "branching") ?? outputNumber(frame, "branchingRatio"),
  radius: outputNumber(frame, "radius") ?? outputNumber(frame, "spectralRadius"),
  asymmetry: outputNumber(frame, "asymmetry"),
  buyIntensity: outputNumber(frame, "buyIntensity"),
  sellIntensity: outputNumber(frame, "sellIntensity"),
  exo: outputNumber(frame, "exo") ?? outputNumber(frame, "baselineMu"),
});

const hawkesSample = (
  frame: Record<string, unknown> | undefined,
  symbol: string,
): HawkesSample | null => {
  const intensity =
    outputNumber(frame, "intensity") ??
    outputNumber(frame, "intensityRatio") ??
    outputNumber(frame, "strength");

  if (intensity === null) {
    return null;
  }

  const key = String(
    frame?.timestamp ??
      frame?.ts ??
      frame?.updated_at ??
      `${symbol}:${intensity}`,
  );

  return { key, symbol, intensity };
};

export const hawkesSamplesFromFrame = (
  frame: Record<string, unknown> | undefined,
  symbol: string,
  limit = 120,
): HawkesSample[] => {
  if (frame === undefined) {
    return [];
  }

  const samples = recordArray(frame.history)
    .map((historyFrame) => hawkesSample(historyFrame, symbol))
    .filter((sample): sample is HawkesSample => sample !== null);
  const latest = hawkesSample(frame, symbol);

  if (latest !== null && samples[samples.length - 1]?.key !== latest.key) {
    samples.push(latest);
  }

  return samples.slice(-limit);
};

export const hawkesSamplesFromFrames = (
  frames: Record<string, unknown>[],
  symbol: string,
  limit = 220,
): HawkesSample[] =>
  frames
    .flatMap((frame) => {
      const sample = hawkesSample(frame, symbol);

      return sample === null ? [] : [sample];
    })
    .slice(-limit);

export const latentPointsFromFrame = (
  frame: Record<string, unknown> | null,
): LatentPoint[] => {
  const latent = numberArray(frame?.latent);
  const symbol = stringValue(frame?.symbol);

  if (symbol !== "" && latent.length >= 2) {
    return [
      {
        key: `${symbol}:0`,
        symbol,
        x: latent[0] ?? 0,
        y: latent[1] ?? 0,
        category: stringValue(frame?.category),
      },
    ];
  }

  return recordArray(frame?.symbols).flatMap((entry, index) => {
    const latent = numberArray(entry.latent);
    const symbol = stringValue(entry.symbol);

    if (symbol === "" || latent.length < 2) {
      return [];
    }

    return [
      {
        key: `${symbol}:${index}`,
        symbol,
        x: latent[0] ?? 0,
        y: latent[1] ?? 0,
        category: stringValue(entry.category),
      },
    ];
  });
};

export const latentPointsFromFrames = (
  frames: Record<string, { values: () => Record<string, unknown>[] }>,
): LatentPoint[] =>
  Object.entries(frames).flatMap(([symbol, history]) => {
    const frame = history.values().at(-1);
    const latent = numberArray(frame?.latent);

    if (latent.length < 2) {
      return [];
    }

    return [
      {
        key: `${symbol}:${String(frame?.at ?? "")}`,
        symbol,
        x: latent[0] ?? 0,
        y: latent[1] ?? 0,
        category: stringValue(frame?.category),
      },
    ];
  });

const rowExtent = (rows: number[][]) => {
  const values = rows.flat();
  const min = Math.min(...values);
  const max = Math.max(...values);

  if (!Number.isFinite(min) || !Number.isFinite(max)) {
    return { min: 0, max: 1 };
  }

  return { min, max };
};

const resampleRow = (values: number[], cellCount: number): number[] => {
  if (values.length === 0 || cellCount <= 0) {
    return [];
  }

  if (values.length === cellCount) {
    return values;
  }

  return Array.from({ length: cellCount }, (_, index) => {
    const position = (index / Math.max(cellCount - 1, 1)) * (values.length - 1);
    const left = Math.floor(position);
    const right = Math.min(values.length - 1, left + 1);
    const ratio = position - left;

    return (values[left] ?? 0) * (1 - ratio) + (values[right] ?? 0) * ratio;
  });
};

export const xrayLayersFromManifold = (
  manifold: Record<string, unknown> | null,
  resonance: Record<string, unknown> | null,
) => {
  const rows = numberMatrix(manifold?.rho);
  const { min, max } = rowExtent(rows);

  if (rows.length === 0) {
    const latent = numberArray(resonance?.latent);

    if (latent.length === 0) {
      return [];
    }

    return latent.map((value, index) => ({
      index,
      label: `L${index} · ${["sensory", "micro", "meso", "macro"][index] ?? "latent"}`,
      state: [value],
      error_norm: Math.min(1, Math.abs(value)),
    }));
  }

  return Array.from({ length: 4 }, (_, index) => {
    const start = Math.floor((index / 4) * rows.length);
    const end = Math.max(start + 1, Math.floor(((index + 1) / 4) * rows.length));
    const group = rows.slice(start, Math.min(rows.length, end));
    const columns = Math.max(...group.map((row) => row.length));
    const averaged = Array.from({ length: columns }, (_, column) => {
      const values = group.flatMap((row) =>
        typeof row[column] === "number" ? [row[column]] : [],
      );

      return values.length === 0
        ? min
        : values.reduce((sum, value) => sum + value, 0) / values.length;
    });
    const span = max > min ? max - min : 1;
    const normalized = resampleRow(averaged, 16).map((value) =>
      max > min ? ((value - min) / span) * 2 - 1 : 0,
    );
    const error =
      normalized.reduce((sum, value) => sum + Math.abs(value), 0) /
      Math.max(1, normalized.length);

    return {
      index,
      label: `L${index} · ${["sensory", "micro", "meso", "macro"][index]}`,
      state: normalized,
      error_norm: error,
    };
  });
};

const cascadeLabel = (
  branching: number | null,
): { label: string; color: string } => {
  if (branching === null) {
    return { label: "—", color: "var(--f4)" };
  }

  if (branching > 0.85) {
    return { label: "critical", color: "var(--down)" };
  }

  if (branching > 0.6) {
    return { label: "elevated", color: "var(--warn)" };
  }

  return { label: "stable", color: "var(--up)" };
};

const cognitiveForSymbol = (
  readings: Record<string, CognitiveReading>,
  symbol: string,
): CognitiveReading | null => {
  if (isConcreteSymbol(symbol)) {
    return readings[symbol] ?? null;
  }

  const [scope] = cognitiveScopes(readings);

  return scope === undefined ? null : readings[scope];
};

const Canvas = ({ draw }: { draw: Draw }) => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;

    if (canvas === null) {
      return;
    }

    const render = () => {
      const context = resizeCanvas(canvas);

      if (context === null) {
        return;
      }

      draw(context, canvas.clientWidth, canvas.clientHeight);
    };

    render();
    const observer = new ResizeObserver(render);
    observer.observe(canvas);

    return () => observer.disconnect();
  }, [draw]);

  return (
    <canvas ref={canvasRef} className="absolute inset-0 block size-full" />
  );
};

const drawWaiting = (
  context: CanvasRenderingContext2D,
  width: number,
  height: number,
  message: string,
) => {
  clearCanvas(context, width, height);
  drawGrid(context, width, height);
  context.fillStyle = TERMINAL_COLORS.muted;
  context.font = "11px JetBrains Mono, monospace";
  context.fillText(message, 18, Math.max(52, height * 0.34));
};

const categoryColor = (category: string, focus: boolean): string => {
  const normalized = category.toLowerCase();

  if (focus) {
    return TERMINAL_COLORS.amber;
  }

  if (normalized.includes("stress") || normalized.includes("turbulent")) {
    return TERMINAL_COLORS.red;
  }

  if (normalized.includes("flow") || normalized.includes("laminar")) {
    return TERMINAL_COLORS.green;
  }

  if (normalized.includes("coupling") || normalized.includes("equilibrium")) {
    return TERMINAL_COLORS.cyan;
  }

  return TERMINAL_COLORS.muted;
};

const latentRange = (
  points: LatentPoint[],
  key: "x" | "y",
): { min: number; span: number } => {
  const values = points.map((point) => point[key]);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min;

  if (!Number.isFinite(min) || !Number.isFinite(span)) {
    return { min: 0, span: 1 };
  }

  if (span <= 0) {
    return { min: min - 0.5, span: 1 };
  }

  return { min, span };
};

const LatentScatter = ({
  points,
  activeSymbol,
}: {
  points: LatentPoint[];
  activeSymbol: string;
}) => {
  const draw = useCallback<Draw>(
    (context, width, height) => {
      if (points.length === 0) {
        drawWaiting(context, width, height, "waiting for latent carriers");
        return;
      }

      clearCanvas(context, width, height);

      const pad = 28;
      const xRange = latentRange(points, "x");
      const yRange = latentRange(points, "y");

      context.strokeStyle = TERMINAL_COLORS.line;
      context.lineWidth = 1;

      for (let index = 0; index <= 4; index += 1) {
        const x = pad + index * ((width - pad * 2) / 4);
        const y = pad + index * ((height - pad * 2) / 4);

        context.beginPath();
        context.moveTo(x, pad);
        context.lineTo(x, height - pad);
        context.stroke();
        context.beginPath();
        context.moveTo(pad, y);
        context.lineTo(width - pad, y);
        context.stroke();
      }

      for (const point of points) {
        const focus = point.symbol === activeSymbol;
        const x =
          pad + ((point.x - xRange.min) / xRange.span) * (width - pad * 2);
        const y =
          height -
          pad -
          ((point.y - yRange.min) / yRange.span) * (height - pad * 2);
        const color = categoryColor(point.category, focus);

        context.fillStyle = color;
        context.globalAlpha = focus ? 1 : 0.72;
        context.shadowBlur = focus ? 12 : 4;
        context.shadowColor = color;
        context.beginPath();
        context.arc(x, y, focus ? 5 : 3.5, 0, Math.PI * 2);
        context.fill();
        context.shadowBlur = 0;
        context.globalAlpha = 1;

        if (focus) {
          context.strokeStyle = TERMINAL_COLORS.amber;
          context.lineWidth = 1.5;
          context.beginPath();
          context.arc(x, y, 9, 0, Math.PI * 2);
          context.stroke();
          context.fillStyle = TERMINAL_COLORS.foreground;
          context.font = "9px JetBrains Mono, monospace";
          context.fillText(
            point.symbol.split("/")[0] ?? point.symbol,
            x + 11,
            y + 4,
          );
        }
      }
    },
    [activeSymbol, points],
  );

  return <Canvas draw={draw} />;
};

const HawkesIntensityPanel = ({
  frames,
  activeSymbol,
  metrics,
  cascade,
}: {
  frames: Record<string, unknown>[];
  activeSymbol: string;
  metrics: HawkesMetrics;
  cascade: { label: string; color: string };
}) => {
  const samples = useMemo(
    () => hawkesSamplesFromFrames(frames, activeSymbol),
    [activeSymbol, frames],
  );
  const draw = useCallback<Draw>(
    (context, width, height) => {
      if (samples.length === 0) {
        drawWaiting(context, width, height, "waiting for hawkes intensity");
        return;
      }

      clearCanvas(context, width, height);
      drawGrid(context, width, height, 18);

      const padX = 18;
      const padTop = 18;
      const padBottom = 28;
      const innerWidth = Math.max(1, width - padX * 2);
      const innerHeight = Math.max(1, height - padTop - padBottom);
      const maxIntensity = Math.max(
        1,
        ...samples.map((sample) => sample.intensity),
      );
      const xFor = (index: number): number =>
        padX +
        (samples.length <= 1
          ? innerWidth
          : (index / (samples.length - 1)) * innerWidth);
      const yFor = (intensity: number): number =>
        padTop + (1 - intensity / maxIntensity) * innerHeight;

      context.fillStyle = "rgba(232, 163, 61, 0.18)";
      context.beginPath();
      context.moveTo(padX, height - padBottom);
      samples.forEach((sample, index) => {
        context.lineTo(xFor(index), yFor(sample.intensity));
      });
      context.lineTo(width - padX, height - padBottom);
      context.closePath();
      context.fill();

      context.strokeStyle = TERMINAL_COLORS.amber;
      context.lineWidth = 1.8;
      context.beginPath();
      samples.forEach((sample, index) => {
        const x = xFor(index);
        const y = yFor(sample.intensity);

        if (index === 0) {
          context.moveTo(x, y);
          return;
        }

        context.lineTo(x, y);
      });
      context.stroke();

      const latest = samples[samples.length - 1];
      if (latest !== undefined) {
        const x = xFor(samples.length - 1);
        const y = yFor(latest.intensity);

        context.fillStyle = TERMINAL_COLORS.amber;
        context.shadowBlur = 10;
        context.shadowColor = TERMINAL_COLORS.amber;
        context.beginPath();
        context.arc(x, y, 3.5, 0, Math.PI * 2);
        context.fill();
        context.shadowBlur = 0;
        context.fillStyle = TERMINAL_COLORS.muted;
        context.font = "10px JetBrains Mono, monospace";
        context.fillText(`λ ${latest.intensity.toFixed(2)}`, 18, height - 9);
      }
    },
    [samples],
  );

  return (
    <div className="flex min-h-[210px] flex-1 flex-col border-(--line) border-t">
      <div className="flex items-start justify-between gap-3 px-[18px] pt-3 pb-2">
        <div>
          <div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
            Hawkes self-exciting intensity
          </div>
          <div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
            λ(t) = μ + Σ α·e^(-β(t-tᵢ)) · order-flow arrivals
          </div>
        </div>
        <div className="shrink-0 text-right font-mono text-[10px]">
          <div>
            <span className="text-(--f3)">η = α/β = </span>
            <span style={{ color: cascade.color }}>
              {format(metrics.branching)}
            </span>
          </div>
          <div>
            <span className="text-(--f3)">λ now </span>
            <span className="text-(--acc)">{format(metrics.intensity, 2)}</span>
          </div>
          <div style={{ color: cascade.color }}>cascade {cascade.label}</div>
        </div>
      </div>
      <div className="relative min-h-0 flex-1">
        <Canvas draw={draw} />
      </div>
    </div>
  );
};

const RowFact = ({
  label,
  value,
  accent,
}: {
  label: string;
  value: unknown;
  accent?: string;
}) => (
  <div className="flex justify-between gap-3">
    <span className="text-(--f3)">{label}</span>
    <span className="text-right" style={{ color: accent ?? "var(--f1)" }}>
      {value === undefined || value === null || value === ""
        ? "—"
        : String(value)}
    </span>
  </div>
);

const RouteComponent = () => {
  const activeSymbol = useSelector(appStore, (state) => state.focusSymbol);
  const instrumentSymbols = useSelector(
    instrumentsStore,
    (state) => state.symbols,
  );
  const readings = useSelector(measurementsStore, (state) => state);
  const resonanceState = useSelector(resonanceStore, (state) => state.resonance);
  const manifoldState = useSelector(manifoldStore, (state) => state.manifold);
  const cognitiveReadings = useSelector(
    cognitiveStore,
    (state) => state.readings,
  );
  const resonance = resonanceState[activeSymbol]?.values().at(-1) ?? null;
  const manifold = manifoldState[activeSymbol]?.values().at(-1) ?? null;
  const layers = xrayLayersFromManifold(manifold, resonance);
  const hawkesHistory =
    readings.measurements[activeSymbol]?.hawkes?.values() ?? [];
  const hawkes = hawkesHistory.at(-1);
  const symbols = [
    ...new Set([
      activeSymbol,
      ...Object.keys(resonanceState),
      ...Object.keys(manifoldState),
      ...Object.keys(readings.measurements),
      ...instrumentSymbols,
    ]),
  ]
    .filter((symbol) => symbol.includes("/"))
    .slice(0, 10);
  const latentPoints = useMemo(
    () => latentPointsFromFrames(resonanceState),
    [resonanceState],
  );
  const cognitive = cognitiveForSymbol(cognitiveReadings, activeSymbol);
  const hawkesNow = hawkesMetrics(hawkes);
  const cascade = cascadeLabel(hawkesNow.branching);
  const reading = asRecord(manifold?.reading);
  const coherenceMag2 = finite(reading?.coherenceMag2);
  const coherence =
    coherenceMag2 === null
      ? "—"
      : coherenceMag2 >= 0.4
        ? "laminar"
        : "turbulent";
  const coherenceFg =
    coherence === "laminar"
      ? "var(--info)"
      : coherence === "turbulent"
        ? "var(--down)"
        : "var(--f4)";
  const freeEnergy = finite(resonance?.energy);
  const surprise = finite(resonance?.surprise);
  const oscillators = asRecord(manifold?.oscillators);
  const momentumShare =
    finite(oscillators?.coherence) ?? hawkesNow.radius ?? hawkesNow.branching ?? 0;
  const momentumFg = momentumShare >= 0.4 ? "var(--up)" : "var(--f3)";

  return (
    <div className="flex h-full min-w-[1100px] flex-col">
      <div className="flex h-[46px] shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5">
        <span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
          Inspect symbol
        </span>
        {symbols.map((symbol) => {
          const active = symbol === activeSymbol;

          return (
            <button
              key={symbol}
              type="button"
              onClick={() => appStore.actions.updateFocusSymbol(symbol)}
              className="shrink-0 cursor-pointer rounded-[3px] border px-[11px] py-1 font-mono font-medium text-[11px]"
              style={{
                borderColor: active ? "var(--acc)" : "var(--line)",
                background: active
                  ? "color-mix(in srgb,var(--acc) 14%,transparent)"
                  : "transparent",
                color: active ? "var(--acc)" : "var(--f3)",
              }}
            >
              {symbol.split("/")[0]}
            </button>
          );
        })}
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
        <div className="flex min-h-0 flex-col overflow-auto border-(--line) border-r">
          <div className="shrink-0 px-[18px] py-4">
            <div className="flex items-baseline justify-between gap-3">
              <span className="font-serif font-semibold text-[22px] text-(--f1) leading-[1.1]">
                Predictive-coding hierarchy
              </span>
              <span
                data-symbol={activeSymbol}
                className="shrink-0 cursor-pointer font-mono text-[11px] text-(--f3)"
              >
                {activeSymbol}
              </span>
            </div>
            <div className="mt-1 font-mono text-[10px] text-(--f4)">
              latent state · prediction error ε per layer · macro = abstract
              regime, sensory = raw tape
            </div>
            <div className="mt-4">
              {layers.length > 0 ? (
                <XrayLayerRows layers={layers} />
              ) : (
                <div className="font-mono text-[10px] text-(--f4)">
                  waiting for resonance layers
                </div>
              )}
            </div>
          </div>
          <HawkesIntensityPanel
            frames={hawkesHistory}
            activeSymbol={activeSymbol}
            metrics={hawkesNow}
            cascade={cascade}
          />
        </div>

        <div className="flex min-h-0 flex-col overflow-auto bg-(--surface)">
          <div className="shrink-0 px-3.5 pt-3 pb-1.5">
            <div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
              Latent manifold
            </div>
            <div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
              universe embedding · clustered by regime · focus pulses
            </div>
          </div>
          <div className="relative mx-2 h-[300px] shrink-0">
            <LatentScatter points={latentPoints} activeSymbol={activeSymbol} />
            <div className="pointer-events-none absolute bottom-1.5 left-2.5 font-mono text-[8.5px] text-(--f4)">
              latent-1 →
            </div>
            <div className="pointer-events-none absolute top-2.5 left-1.5 font-mono text-[8.5px] text-(--f4) [writing-mode:vertical-rl]">
              latent-2 →
            </div>
          </div>

          <div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
            <RowFact
              label="regime class"
              value={cognitive?.regimePrefix || stringValue(resonance?.category)}
              accent="var(--acc)"
            />
            <RowFact label="coherence" value={coherence} accent={coherenceFg} />
            <RowFact label="free energy" value={format(freeEnergy)} />
            <RowFact
              label="surprise"
              value={surprise === null ? "—" : `${surprise.toFixed(2)}× thr`}
            />
            <RowFact
              label="flow events"
              value={hawkesHistory.length === 0 ? "—" : hawkesHistory.length}
            />
            <RowFact
              label="branching η"
              value={format(hawkesNow.branching)}
              accent={cascade.color}
            />
          </div>

          <div className="flex flex-col gap-2 border-(--line) border-t px-3.5 py-3">
            <div>
              <div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
                Manifold reading
              </div>
              <div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
                navier–stokes · ρ projection · oscillator carriers
              </div>
            </div>
            <div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
              <RowFact
                label="∇·u"
                value={signed(finite(reading?.divergence))}
              />
              <RowFact
                label="|ψ|²"
                value={format(finite(reading?.coherenceMag2))}
              />
              <RowFact
                label="guide v"
                value={format(finite(reading?.guidanceSpeed))}
              />
              <RowFact
                label="viscosity"
                value={format(finite(reading?.viscosityProxy))}
              />
            </div>
            <div className="mt-0.5">
              <div className="mb-1 flex justify-between text-[10px]">
                <span className="text-(--f3)">momentum eigenmode</span>
                <span className="font-mono" style={{ color: momentumFg }}>
                  {momentumShare.toFixed(2)} / 0.40
                </span>
              </div>
              <div className="relative h-1.5 overflow-hidden rounded-sm bg-(--line)">
                <div
                  className="h-full"
                  style={{
                    width: `${Math.min(100, momentumShare * 100)}%`,
                    background: momentumFg,
                  }}
                />
              </div>
              <div className="relative h-0">
                <div className="absolute top-[-9px] left-[40%] h-3 w-0.5 bg-(--acc)" />
              </div>
              <div className="mt-1.5 font-mono text-[8.5px] text-(--f4)">
                drive playbook gate · mode share ≥ 0.40
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export const Route = createFileRoute("/xray")({
  component: RouteComponent,
});
