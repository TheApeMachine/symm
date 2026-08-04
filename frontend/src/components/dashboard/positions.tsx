import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";

/*
Positions is the open-lot list.

Each lot reads as two lines — what it is worth, and what it cost — with the
stop gauge underneath showing where the mark sits between the trailing floor
and the peak it has reached. Every value is painted straight from the wire; the
percentages that place the gauge markers are computed with the prices they
belong to, so nothing here has to know how the axis was scaled.
*/
export const Positions = () => (
  <Component registerKey="positions">
    {({ ref, className, slots }) => (
      <List
        ref={ref}
        className={cn("min-h-0 flex-1 p-1.5", className)}
      >
        {slots.map((slot) => (
          <List.Item
            data-index={slot}
            key={`${slot}-position`}
            className="mb-1.25 block rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]"
          >
            {/*
              The lot's own colour is set once as a custom property on the
              card, so the figures inside read from it instead of each having
              to work out the sign for themselves.
            */}
            <Flex.Column
              data-set="holding.pnl"
              data-set-scale="sign-color"
              data-target="style.--pnl"
              className="gap-0"
            >
              <Flex.Row className="items-center justify-between gap-3">
                <Typography.Span
                  data-paint="holding.symbol"
                  className="font-semibold text-[11.5px] text-(--f1)"
                />
                <Typography.Span
                  data-paint="holding.pnl"
                  data-paint-format=".4f"
                  data-paint-suffix=" USD"
                  className="text-right font-semibold text-[11.5px] text-(--pnl)"
                />
              </Flex.Row>

              <Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
                <Typography.Span>
                  entry{" "}
                  <span
                    data-paint="holding.entry_price"
                    data-paint-format=".6f"
                  />{" "}
                  / mark{" "}
                  <span data-paint="holding.mark" data-paint-format=".6f" />
                </Typography.Span>
                <Typography.Span
                  data-paint="holding.return_pct"
                  data-paint-format=".2%"
                  className="text-(--pnl)"
                />
              </Flex.Row>

              <div className="relative mt-1.5 h-0.75 overflow-visible rounded-full bg-[color-mix(in_srgb,var(--f4)_18%,transparent)]">
                <div
                  data-set="holding.stoploss.floor"
                  data-set-domain="holding.entry_price,holding.mark,holding.stoploss.floor,holding.stoploss.peak"
                  data-set-scale="domain-percent"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 h-3 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-[color-mix(in_srgb,var(--down)_55%,transparent)]"
                />
                <div
                  data-set="holding.stoploss.peak"
                  data-set-domain="holding.entry_price,holding.mark,holding.stoploss.floor,holding.stoploss.peak"
                  data-set-scale="domain-percent"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 h-3 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-[color-mix(in_srgb,var(--up)_55%,transparent)]"
                />
                <div
                  data-set="holding.entry_price"
                  data-set-domain="holding.entry_price,holding.mark,holding.stoploss.floor,holding.stoploss.peak"
                  data-set-scale="domain-percent"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 h-1.5 w-px -translate-x-1/2 -translate-y-1/2 bg-[color-mix(in_srgb,var(--f4)_55%,transparent)]"
                />
                <div
                  data-set="holding.mark"
                  data-set-domain="holding.entry_price,holding.mark,holding.stoploss.floor,holding.stoploss.peak"
                  data-set-scale="domain-percent"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 h-1.75 w-1.75 -translate-x-1/2 -translate-y-1/2 rounded-full border border-[color-mix(in_srgb,var(--surface)_70%,transparent)] bg-(--f1)"
                />
              </div>

              <Flex.Row className="mt-1 items-center justify-between gap-2 text-[9px]">
                <Typography.Span
                  data-set="holding.stoploss.floor"
                  data-set-threshold="holding.profit_threshold"
                  data-set-scale="above-threshold"
                  data-target="style.color"
                >
                  floor{" "}
                  <span
                    data-paint="holding.stoploss.floor"
                    data-paint-format=".6f"
                  />
                </Typography.Span>
                <Typography.Span>
                  qty <span data-paint="holding.qty" data-paint-format=".4f" />
                </Typography.Span>
                <Typography.Span
                  data-set="holding.stoploss.peak"
                  data-set-threshold="holding.profit_threshold"
                  data-set-scale="above-threshold"
                  data-target="style.color"
                >
                  peak{" "}
                  <span
                    data-paint="holding.stoploss.peak"
                    data-paint-format=".6f"
                  />
                </Typography.Span>
              </Flex.Row>
            </Flex.Column>
          </List.Item>
        ))}
      </List>
    )}
  </Component>
);
