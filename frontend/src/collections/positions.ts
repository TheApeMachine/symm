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
};

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
    const derivedExit =
      mark !== null && entry > 0 ? quantity * mark * (1 - exitFeeRate) : null;
    const exitValue = expectedExit[base] ?? derivedExit;
    const unrealized =
      expectedUnrealized[base] ??
      (exitValue !== null ? exitValue - entryCost : 0);
    const priced = exitValue !== null && entryCost > 0;
    const unrealizedPct = priced ? (unrealized / entryCost) * 100 : 0;

    positions.push({
      symbol,
      qty: quantity,
      avgEntry: entry,
      mark: mark ?? 0,
      exitValue: exitValue ?? 0,
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
  updatePositionStores(
    positionsFromBalance(frame),
    cashBalance(frame, currency),
  );
};

export const resetPositionStateForTest = () => {
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
