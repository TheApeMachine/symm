export type PredictionPoint = {
  x: number;
  value: number;
};

export type PredictionSeries = {
  actual: PredictionPoint[];
  prediction: PredictionPoint[];
  error: PredictionPoint[];
};

export type TerminalResonanceLayer = {
  state: number[];
  prediction: number[];
  errorNorm: number;
};

export type TerminalResonanceFrame = {
  symbol: string;
  category: string;
  surprise: number;
  energy: number;
  confidence: number;
  layers: TerminalResonanceLayer[];
};

const SERIES_LIMIT = 160;
const FLUID_COLUMNS = 64;
const FLUID_ROWS = 38;
const FLUID_METRICS = ["volume", "change_pct", "re", "div", "vort", "turb"];

const finiteNumber = (value: unknown): number | null =>
  typeof value === "number" && Number.isFinite(value) ? value : null;

const finiteArray = (value: unknown): number[] => {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.filter(
    (entry): entry is number =>
      typeof entry === "number" && Number.isFinite(entry),
  );
};

const rowNumber = (row: unknown, key: string): number | null => {
  if (typeof row !== "object" || row === null) {
    return null;
  }

  const value = (row as Record<string, unknown>)[key];

  return finiteNumber(value);
};

const predictionKind = (value: unknown): keyof PredictionSeries | null => {
  if (value === "actual" || value === "prediction" || value === "error") {
    return value;
  }

  return null;
};

const upsertPoint = (
  points: PredictionPoint[],
  point: PredictionPoint,
): PredictionPoint[] => {
  const next = points.filter((entry) => entry.x !== point.x);
  next.push(point);
  next.sort((left, right) => left.x - right.x);

  return next.slice(-SERIES_LIMIT);
};

export const appendPredictionFrame = (
  series: PredictionSeries,
  frame: Record<string, unknown>,
): PredictionSeries => {
  const kind = predictionKind(frame.kind);
  const x = finiteNumber(frame.x);
  const value = finiteNumber(frame.value);

  if (kind === null || x === null || value === null) {
    return series;
  }

  return {
    ...series,
    [kind]: upsertPoint(series[kind], { x, value }),
  };
};

export const emptyPredictionSeries = (): PredictionSeries => ({
  actual: [],
  prediction: [],
  error: [],
});

export const resetTerminalFluidMatrix = () => {
  return;
};

const emptyMatrix = (rows: number, columns: number): number[][] =>
  Array.from({ length: rows }, () => Array.from({ length: columns }, () => 0));

const rank = (value: number, sorted: number[]): number => {
  if (sorted.length <= 1) {
    return 0.5;
  }

  let below = 0;

  for (const entry of sorted) {
    if (entry < value) {
      below += 1;
    }
  }

  return below / (sorted.length - 1);
};

const clampIndex = (value: number, upper: number): number =>
  Math.max(0, Math.min(upper - 1, value));

const metricValue = (row: unknown, key: string): number | null => {
  if (key === "volume") {
    return rowNumber(row, "volume") ?? rowNumber(row, "vol");
  }

  return rowNumber(row, key);
};

const metricMaxima = (rows: unknown[]) =>
  Object.fromEntries(
    FLUID_METRICS.map((key) => [
      key,
      Math.max(
        0,
        ...rows
          .map((row) => metricValue(row, key))
          .filter((value): value is number => value !== null)
          .map(Math.abs),
      ),
    ]),
  );

const activity = (
  row: unknown,
  maxima: Record<string, number>,
): number | null => {
  const values = FLUID_METRICS.flatMap((key) => {
    const value = metricValue(row, key);
    const max = maxima[key] ?? 0;

    if (value === null || max <= 0) {
      return [];
    }

    return [Math.abs(value) / max];
  });

  if (values.length === 0) {
    return null;
  }

  return values.reduce((sum, value) => sum + value, 0) / values.length;
};

const addField = (
  matrix: number[][],
  column: number,
  row: number,
  value: number,
  radius: number,
) => {
  for (let rowOffset = -radius; rowOffset <= radius; rowOffset += 1) {
    for (
      let columnOffset = -radius;
      columnOffset <= radius;
      columnOffset += 1
    ) {
      const targetRow = row + rowOffset;
      const targetColumn = column + columnOffset;

      if (
        targetRow < 0 ||
        targetColumn < 0 ||
        targetRow >= matrix.length ||
        targetColumn >= (matrix[0]?.length ?? 0)
      ) {
        continue;
      }

      const distance = Math.hypot(rowOffset, columnOffset);

      if (distance > radius) {
        continue;
      }

      matrix[targetRow][targetColumn] += value / (1 + distance);
    }
  }
};

export const terminalFluidMatrix = (
  frame: Record<string, unknown>,
): number[][] => {
  const symbols = frame.symbols;

  if (!Array.isArray(symbols) || symbols.length === 0) {
    return [];
  }

  const matrix = emptyMatrix(FLUID_ROWS, FLUID_COLUMNS);
  const volumes = symbols
    .map((row) => metricValue(row, "volume"))
    .filter((value): value is number => value !== null)
    .sort((left, right) => left - right);
  const changes = symbols
    .map((row) => metricValue(row, "change_pct"))
    .filter((value): value is number => value !== null)
    .sort((left, right) => left - right);
  const maxima = metricMaxima(symbols);
  const radius = Math.max(
    1,
    Math.round(Math.sqrt((FLUID_COLUMNS * FLUID_ROWS) / symbols.length) / 2),
  );

  for (const symbol of symbols) {
    const volume = metricValue(symbol, "volume");
    const change = metricValue(symbol, "change_pct");
    const value = activity(symbol, maxima);

    if (volume === null || change === null || value === null) {
      continue;
    }

    const column = clampIndex(
      Math.round(rank(volume, volumes) * FLUID_COLUMNS),
      FLUID_COLUMNS,
    );
    const row = clampIndex(
      Math.round((1 - rank(change, changes)) * FLUID_ROWS),
      FLUID_ROWS,
    );
    addField(matrix, column, row, value, radius);
  }

  return matrix;
};

const numericMatrix = (value: unknown): number[][] => {
  if (!Array.isArray(value)) {
    return [];
  }

  return value
    .map((row) => (Array.isArray(row) ? finiteArray(row) : []))
    .filter((row) => row.length > 0);
};

const normalizeMatrix = (matrix: number[][]): number[][] => {
  if (matrix.length === 0) {
    return [];
  }

  const values = matrix.flat();
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max > min ? max - min : 1;

  return matrix.map((row) => row.map((value) => (value - min) / span));
};

export const terminalManifoldMatrix = (
  frame: Record<string, unknown> | null,
): number[][] => {
  if (frame === null) {
    return [];
  }

  return normalizeMatrix(numericMatrix(frame.rho));
};

const parseLayer = (value: unknown): TerminalResonanceLayer | null => {
  if (typeof value !== "object" || value === null) {
    return null;
  }

  const layer = value as Record<string, unknown>;
  const state = finiteArray(layer.state);

  if (state.length === 0) {
    return null;
  }

  return {
    state,
    prediction: finiteArray(layer.prediction),
    errorNorm:
      finiteNumber(layer.error_norm) ?? finiteNumber(layer.errorNorm) ?? 0,
  };
};

const parseResonanceXRay = (
  raw: Record<string, unknown>,
): TerminalResonanceFrame | null => {
  const symbol = typeof raw.symbol === "string" ? raw.symbol.trim() : "";
  const layersRaw = raw.layers;

  if (symbol === "" || !Array.isArray(layersRaw)) {
    return null;
  }

  const layers = layersRaw.flatMap((layer) => {
    const parsed = parseLayer(layer);

    return parsed === null ? [] : [parsed];
  });

  if (layers.length === 0) {
    return null;
  }

  return {
    symbol,
    category: typeof raw.category === "string" ? raw.category : "",
    surprise: finiteNumber(raw.surprise) ?? 0,
    energy: finiteNumber(raw.energy) ?? 0,
    confidence: finiteNumber(raw.confidence) ?? 0,
    layers,
  };
};

export const terminalResonanceFrame = (
  frame: Record<string, unknown> | null,
): TerminalResonanceFrame | null => {
  if (frame === null) {
    return null;
  }

  if (frame.type === "resonance_universe") {
    const focus = frame.focus;

    if (typeof focus !== "object" || focus === null) {
      return null;
    }

    return parseResonanceXRay(focus as Record<string, unknown>);
  }

  return parseResonanceXRay(frame);
};
