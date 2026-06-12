import { useSelector } from "@tanstack/react-store";
import { balanceStore } from "#/collections/balance";
import { statusStore } from "#/collections/status";
import { Flex } from "#/components/ui/flex";

const POSITION_QUANTITY_DIGITS = 8;
const POSITION_MONEY_DIGITS = 4;
const POSITION_PERCENT_DIGITS = 4;
const SMALL_PRICE_DIGITS = 8;
const MID_PRICE_DIGITS = 6;
const LARGE_PRICE_DIGITS = 4;
const MID_PRICE_THRESHOLD = 1;
const LARGE_PRICE_THRESHOLD = 10;

export const positionQuantity = (value: number) => {
  return value.toFixed(POSITION_QUANTITY_DIGITS);
};

export const positionPriceDigits = (value: number) => {
  const absolute = Math.abs(value);

  if (absolute >= LARGE_PRICE_THRESHOLD) {
    return LARGE_PRICE_DIGITS;
  }

  if (absolute >= MID_PRICE_THRESHOLD) {
    return MID_PRICE_DIGITS;
  }

  return SMALL_PRICE_DIGITS;
};

export const positionMoney = (
  value: number,
  symbol: string,
  fractionDigits = POSITION_MONEY_DIGITS,
) => {
  const prefix = value < 0 ? "-" : "";
  const absolute = Math.abs(value).toFixed(fractionDigits);

  if (symbol.length === 1) {
    return `${prefix}${symbol}${absolute}`;
  }

  return `${prefix}${absolute} ${symbol}`;
};

export const positionPrice = (value: number, symbol: string) => {
  return positionMoney(value, symbol, positionPriceDigits(value));
};

export const signedPositionMoney = (value: number, symbol: string) => {
  const sign = value >= 0 ? "+" : "-";

  return `${sign}${positionMoney(Math.abs(value), symbol)}`;
};

export const positionPercent = (value: number) => {
  const sign = value >= 0 ? "+" : "";

  return `${sign}${value.toFixed(POSITION_PERCENT_DIGITS)}%`;
};

export const unsignedPositionPercent = (value: number) => {
  return `${value.toFixed(POSITION_PERCENT_DIGITS)}%`;
};

/*
PositionsPanel lists the open book with live expected liquidation P&L from the
bid side. Rendered as a dropdown from the wallet button.
*/
export const PositionsPanel = () => {
  const { positionViews } = useSelector(statusStore, (state) => state);
  const { symbol } = useSelector(balanceStore, (state) => state);

  return (
    <Flex.Column gap={2}>
      <p className="px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
        Open positions
      </p>

      {positionViews.length === 0 ? (
        <p className="px-1 py-2 text-xs text-muted-foreground">
          No open positions
        </p>
      ) : (
        positionViews.map((position) => {
          const positive = position.priced && position.unrealized >= 0;

          return (
            <div
              key={position.symbol}
              className="flex flex-col gap-1 rounded-lg border border-border bg-background px-3 py-2"
            >
              <div className="flex items-center justify-between">
                <span className="font-semibold text-sm">{position.symbol}</span>
                <span
                  className={`font-mono text-sm ${
                    !position.priced
                      ? "text-muted-foreground"
                      : positive
                        ? "text-emerald-400"
                        : "text-red-400"
                  }`}
                >
                  {position.priced
                    ? signedPositionMoney(position.unrealized, symbol)
                    : "pricing"}
                </span>
              </div>
              <div className="flex items-center justify-between gap-3 text-[11px] text-muted-foreground">
                <span className="font-mono tabular-nums">
                  {positionQuantity(position.qty)} @{" "}
                  {positionPrice(position.avgEntry, symbol)}
                </span>
                {position.priced ? (
                  <span
                    className={`font-mono tabular-nums ${
                      positive ? "text-emerald-400" : "text-red-400"
                    }`}
                  >
                    {positionPercent(position.unrealizedPct)}
                  </span>
                ) : null}
              </div>
              <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                <span>mark</span>
                <span className="font-mono tabular-nums">
                  {position.priced
                    ? positionPrice(position.mark, symbol)
                    : "waiting"}
                </span>
              </div>
              {position.stopPrice !== undefined && position.stopPrice > 0 ? (
                <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                  <span>stop</span>
                  <span className="font-mono tabular-nums">
                    {positionPrice(position.stopPrice, symbol)}
                  </span>
                </div>
              ) : null}
              {position.priced && position.exitFeeRate > 0 ? (
                <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                  <span>exit fee</span>
                  <span className="font-mono tabular-nums">
                    {unsignedPositionPercent(position.exitFeeRate * 100)}
                  </span>
                </div>
              ) : null}
            </div>
          );
        })
      )}
    </Flex.Column>
  );
};
