export type TerminalCrossSectionTile = {
  key: string;
  label: string;
  title: string;
  value: number;
};

const CROSS_SECTION_METRICS = [
  "confidence",
  "strength",
  "surprise",
  "re",
  "div",
  "vort",
  "turb",
  "visc",
  "volume",
  "vol",
  "change_pct",
] as const;

const finiteNumber = (value: unknown): number | null =>
  typeof value === "number" && Number.isFinite(value) ? value : null;

const rowNumber = (row: Record<string, unknown>, key: string): number | null =>
  finiteNumber(row[key]);

const rowSymbol = (row: Record<string, unknown>): string =>
  typeof row.symbol === "string" ? row.symbol.trim() : "";

const metricMaxima = (rows: Record<string, unknown>[]) =>
  Object.fromEntries(
    CROSS_SECTION_METRICS.map((key) => [
      key,
      Math.max(
        0,
        ...rows
          .map((row) => Math.abs(rowNumber(row, key) ?? 0))
          .filter((value) => value > 0),
      ),
    ]),
  ) as Record<(typeof CROSS_SECTION_METRICS)[number], number>;

const rowValue = (
  row: Record<string, unknown>,
  maxima: Record<(typeof CROSS_SECTION_METRICS)[number], number>,
): number | null => {
  const values = CROSS_SECTION_METRICS.flatMap((key) => {
    const value = rowNumber(row, key);
    const max = maxima[key];

    if (value === null || max <= 0) {
      return [];
    }

    return [Math.min(1, Math.abs(value) / max)];
  });

  if (values.length === 0) {
    return null;
  }

  return values.reduce((sum, value) => sum + value, 0) / values.length;
};

const tileLabel = (symbol: string): string =>
  (symbol.split("/")[0] ?? symbol).slice(0, 4).toUpperCase();

export const terminalCrossSectionTiles = (
  frame: Record<string, unknown> | null,
): TerminalCrossSectionTile[] => {
  const rows = frame?.symbols;

  if (!Array.isArray(rows) || rows.length === 0) {
    return [];
  }

  const records = rows.filter(
    (row): row is Record<string, unknown> =>
      typeof row === "object" && row !== null && rowSymbol(row) !== "",
  );
  const maxima = metricMaxima(records);

  return records
    .flatMap((row, index) => {
      const symbol = rowSymbol(row);
      const value = rowValue(row, maxima);

      if (value === null) {
        return [];
      }

      return [
        {
          key: `${symbol}:${index}`,
          label: tileLabel(symbol),
          title: `${symbol} ${value.toFixed(3)}`,
          value,
        },
      ];
    })
    .slice(0, 24);
};
