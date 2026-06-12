import { useSelector } from "@tanstack/react-store";
import { balanceStore } from "#/collections/balance";
import { statusStore } from "#/collections/status";
import { Flex } from "#/components/ui/flex";

const money = (value: number, symbol: string) => {
  const prefix = value < 0 ? "-" : "";
  const absolute = Math.abs(value).toFixed(2);

  if (symbol.length === 1) {
    return `${prefix}${symbol}${absolute}`;
  }

  return `${prefix}${absolute} ${symbol}`;
};

const signedMoney = (value: number, symbol: string) => {
  const sign = value >= 0 ? "+" : "-";

  return `${sign}${money(Math.abs(value), symbol)}`;
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
                    ? signedMoney(position.unrealized, symbol)
                    : "pricing"}
                </span>
              </div>
              <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                <span>
                  {position.qty.toFixed(4)} @ {money(position.avgEntry, symbol)}
                </span>
                {position.priced ? (
                  <span
                    className={positive ? "text-emerald-400" : "text-red-400"}
                  >
                    {position.unrealizedPct >= 0 ? "+" : ""}
                    {position.unrealizedPct.toFixed(2)}%
                  </span>
                ) : null}
              </div>
              <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                <span>bid</span>
                <span className="font-mono">
                  {position.priced ? money(position.mark, symbol) : "waiting"}
                </span>
              </div>
              {position.priced && position.exitFeeRate > 0 ? (
                <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                  <span>exit fee</span>
                  <span>{(position.exitFeeRate * 100).toFixed(3)}%</span>
                </div>
              ) : null}
            </div>
          );
        })
      )}
    </Flex.Column>
  );
};
