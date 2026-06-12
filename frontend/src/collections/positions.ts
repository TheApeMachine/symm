import { balanceStore } from "#/collections/balance";
import { statusStore } from "#/collections/status";

type BalanceFrame = Record<string, unknown>;

type PositionView = {
  symbol: string;
  qty: number;
  avgEntry: number;
  mark: number;
  exitValue: number;
  unrealized: number;
  unrealizedPct: number;
  priced: boolean;
  exitFeeRate: number;
  stopPrice?: number;
  peakPrice?: number;
  offset?: number;
  markSource?: string;
};

let monitorOwnsPositions = false;

const finiteNumber = (value: unknown): number | null => {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }

  return value;
};

const numberMap = (value: unknown): Record<string, number> => {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return {};
  }

  const output: Record<string, number> = {};

  for (const [key, raw] of Object.entries(value)) {
    const parsed = finiteNumber(raw);

    if (parsed === null) {
      continue;
    }

    output[key] = parsed;
  }

  return output;
};

const quoteCurrency = (frame: BalanceFrame): string => {
  const currency = frame.Currency;

  if (typeof currency === "string" && currency.trim() !== "") {
    return currency.trim().toUpperCase();
  }

  return "USD";
};

const cashBalance = (frame: BalanceFrame, currency: string): number => {
  const explicit = finiteNumber(frame.Balance);

  if (explicit !== null) {
    return explicit;
  }

  const assets = frame.assets as Record<string, unknown> | undefined;
  const rows = Array.isArray(assets?.asset) ? assets.asset : [];

  for (const row of rows) {
    if (typeof row !== "object" || row === null) {
      continue;
    }

    const record = row as Record<string, unknown>;
    const asset = typeof record.asset === "string" ? record.asset : "";
    const balance = finiteNumber(record.balance);
    const normalized = asset.trim().toUpperCase();

    if (balance === null) {
      continue;
    }

    if (normalized === currency || normalized === `Z${currency}`) {
      return balance;
    }
  }

  return 0;
};

const fallbackInventory = (
  frame: BalanceFrame,
  currency: string,
): Record<string, number> => {
  const assets = frame.assets as Record<string, unknown> | undefined;
  const rows = Array.isArray(assets?.asset) ? assets.asset : [];
  const inventory: Record<string, number> = {};

  for (const row of rows) {
    if (typeof row !== "object" || row === null) {
      continue;
    }

    const record = row as Record<string, unknown>;
    const asset = typeof record.asset === "string" ? record.asset : "";
    const balance = finiteNumber(record.balance);
    const normalized = asset.trim().toUpperCase();

    if (balance === null || balance <= 0) {
      continue;
    }

    if (normalized === currency || normalized === `Z${currency}`) {
      continue;
    }

    inventory[normalized] = balance;
  }

  return inventory;
};

const markForPosition = (
  symbol: string,
  frame: BalanceFrame,
): number | null => {
  const payloadMarks = numberMap(frame.Marks);
  const payloadMark = payloadMarks[symbol];

  if (payloadMark !== undefined && payloadMark > 0) {
    return payloadMark;
  }

  return null;
};

const optionalNumber = (
  record: Record<string, unknown>,
  key: string,
): number | undefined => {
  const value = finiteNumber(record[key]);

  if (value === null) {
    return undefined;
  }

  return value;
};

const positionFromMonitor = (
  value: unknown,
): PositionView | null => {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }

  const record = value as Record<string, unknown>;
  const symbol = typeof record.symbol === "string" ? record.symbol.trim() : "";
  const qty = finiteNumber(record.qty);
  const avgEntry = finiteNumber(record.avg_entry) ?? 0;
  const mark = finiteNumber(record.mark) ?? 0;
  const exitValue = finiteNumber(record.exit_value) ?? 0;
  const unrealized = finiteNumber(record.unrealized) ?? 0;
  const unrealizedPct = finiteNumber(record.unrealized_pct) ?? 0;
  const exitFeeRate = finiteNumber(record.exit_fee_rate) ?? 0;
  const priced = record.priced === true;
  const markSource =
    typeof record.mark_source === "string" ? record.mark_source.trim() : "";

  if (symbol === "" || qty === null || qty <= 0) {
    return null;
  }

  return {
    symbol,
    qty,
    avgEntry,
    mark,
    exitValue: priced ? exitValue : 0,
    unrealized: priced ? unrealized : 0,
    unrealizedPct: priced ? unrealizedPct : 0,
    priced,
    exitFeeRate,
    stopPrice: optionalNumber(record, "stop_price"),
    peakPrice: optionalNumber(record, "peak_price"),
    offset: optionalNumber(record, "offset"),
    markSource: markSource === "" ? undefined : markSource,
  };
};

const positionsFromMonitor = (frame: BalanceFrame): PositionView[] => {
  const rows = Array.isArray(frame.positions) ? frame.positions : [];
  const positions: PositionView[] = [];

  for (const row of rows) {
    const position = positionFromMonitor(row);

    if (position === null) {
      continue;
    }

    positions.push(position);
  }

  return positions.sort((left, right) =>
    left.symbol.localeCompare(right.symbol),
  );
};

const positionsFromBalance = (frame: BalanceFrame): PositionView[] => {
  const currency = quoteCurrency(frame);
  const inventory = {
    ...fallbackInventory(frame, currency),
    ...numberMap(frame.Inventory),
  };
  const avgEntry = numberMap(frame.AvgEntry);
  const expectedExit = numberMap(frame.ExpectedExit);
  const expectedUnrealized = numberMap(frame.Unrealized);
  const exitFeeRates = numberMap(frame.ExitFeeRate);
  const positions: PositionView[] = [];

  for (const [base, quantity] of Object.entries(inventory)) {
    if (quantity <= 0) {
      continue;
    }

    const entry = avgEntry[base] ?? 0;
    const symbol = `${base}/${currency}`;
    const mark = markForPosition(symbol, frame);
    const exitFeeRate = exitFeeRates[base] ?? 0;
    const entryCost = quantity * entry;
    const exitValue = expectedExit[base] ?? 0;
    const unrealized = expectedUnrealized[base] ?? 0;
    const priced =
      expectedExit[base] !== undefined &&
      expectedUnrealized[base] !== undefined &&
      entryCost > 0;
    const unrealizedPct = priced ? (unrealized / entryCost) * 100 : 0;

    positions.push({
      symbol,
      qty: quantity,
      avgEntry: entry,
      mark: mark ?? 0,
      exitValue,
      unrealized,
      unrealizedPct,
      priced,
      exitFeeRate,
    });
  }

  return positions.sort((left, right) =>
    left.symbol.localeCompare(right.symbol),
  );
};

const updatePositionStores = (positions: PositionView[], cash: number) => {
  const exitBalance = positions.reduce(
    (total, position) => total + (position.priced ? position.unrealized : 0),
    0,
  );
  const exitValue = positions.reduce(
    (total, position) => total + (position.priced ? position.exitValue : 0),
    0,
  );
  const pricedPositions = positions.filter(
    (position) => position.priced,
  ).length;

  statusStore.actions.updatePositionViews(positions);
  balanceStore.actions.updateOpenPositions(positions.length);
  balanceStore.actions.updatePricedPositions(pricedPositions);
  balanceStore.actions.updateLiquidationBalance(cash + exitValue);
  balanceStore.actions.updateLiquidationComplete(
    pricedPositions === positions.length,
  );
  balanceStore.actions.updateExitBalance(exitBalance);
  balanceStore.actions.updateInProfit(exitBalance >= 0);
};

export const applyBalanceFrame = (frame: BalanceFrame) => {
  const currency = quoteCurrency(frame);

  balanceStore.setState((previous) => ({ ...previous, ...frame }));
  if (monitorOwnsPositions) {
    return;
  }

  updatePositionStores(
    positionsFromBalance(frame),
    cashBalance(frame, currency),
  );
};

export const applyPositionFrame = (frame: BalanceFrame) => {
  monitorOwnsPositions = true;
  const positions = positionsFromMonitor(frame);
  const cash = finiteNumber(frame.cash) ?? 0;
  const openPositions = finiteNumber(frame.open_positions) ?? positions.length;
  const pricedPositions =
    finiteNumber(frame.priced_positions) ??
    positions.filter((position) => position.priced).length;
  const liquidationBalance = finiteNumber(frame.liquidation_balance) ?? cash;
  const exitBalance = finiteNumber(frame.exit_balance) ?? 0;
  const liquidationComplete = frame.liquidation_complete === true;
  const inProfit = frame.in_profit === true;

  statusStore.actions.updatePositionViews(positions);
  balanceStore.actions.updateOpenPositions(openPositions);
  balanceStore.actions.updatePricedPositions(pricedPositions);
  balanceStore.actions.updateLiquidationBalance(liquidationBalance);
  balanceStore.actions.updateLiquidationComplete(liquidationComplete);
  balanceStore.actions.updateExitBalance(exitBalance);
  balanceStore.actions.updateInProfit(inProfit);
};

export const resetPositionStateForTest = () => {
  monitorOwnsPositions = false;
  updatePositionStores([], 0);
  balanceStore.setState((previous) => ({
    ...previous,
    assets: {},
    balanceLabel: "Balance",
    symbol: "$",
    pricedPositions: 0,
    liquidationBalance: 0,
    liquidationComplete: true,
  }));
};
