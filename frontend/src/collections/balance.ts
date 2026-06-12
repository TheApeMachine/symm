import { createStore } from "@tanstack/react-store";

export const balanceStore = createStore(
  {
    assets: {} as Record<string, unknown>,
    balanceLabel: "Balance",
    symbol: "$",
    openPositions: 0,
    pricedPositions: 0,
    liquidationBalance: 0,
    liquidationComplete: true,
    exitBalance: 0,
    inProfit: false,
  },
  ({ setState }) => ({
    updateOpenPositions: (openPositions: number) =>
      setState((state) => ({
        ...state,
        openPositions: openPositions,
      })),
    updatePricedPositions: (pricedPositions: number) =>
      setState((state) => ({
        ...state,
        pricedPositions: pricedPositions,
      })),
    updateLiquidationBalance: (liquidationBalance: number) =>
      setState((state) => ({
        ...state,
        liquidationBalance: liquidationBalance,
      })),
    updateLiquidationComplete: (liquidationComplete: boolean) =>
      setState((state) => ({
        ...state,
        liquidationComplete: liquidationComplete,
      })),
    updateExitBalance: (exitBalance: number) =>
      setState((state) => ({
        ...state,
        exitBalance: exitBalance,
      })),
    updateInProfit: (inProfit: boolean) =>
      setState((state) => ({
        ...state,
        inProfit: inProfit,
      })),
    updateAssets: (assets: Record<string, unknown>) =>
      setState((state) => {
        const assetRows = Array.isArray(assets.asset)
          ? (assets.asset as Record<string, unknown>[])
          : [];
        const primaryAsset =
          assetRows.find(
            (row) => row.asset === "ZUSD" || row.asset === "USD",
          ) ?? assetRows[0];
        const balanceAmount =
          typeof primaryAsset?.balance === "number"
            ? primaryAsset.balance
            : null;
        const assetCode =
          typeof primaryAsset?.asset === "string" ? primaryAsset.asset : "";
        const isUsd = assetCode === "ZUSD" || assetCode === "USD";

        return {
          ...state,
          assets: { ...state.assets, ...assets },
          balanceLabel:
            balanceAmount !== null
              ? `${isUsd ? "$" : ""}${balanceAmount.toFixed(2)}${isUsd ? "" : ` ${assetCode}`}`
              : state.balanceLabel,
          symbol: isUsd ? "$" : assetCode || state.symbol,
        };
      }),
  }),
);
