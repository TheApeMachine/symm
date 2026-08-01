import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";

export const Positions = () => (
  <Component registerKey="positions">
    {({ ref, className, slots }) => (
      <List
        ref={ref}
        className={cn("min-h-0 flex-1 border-(--line) border-b", className)}
      >
        {slots.map((slot) => (
          <List.Item
            data-index={slot}
            key={`${slot}-position`}
            className="mb-1.25 block rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]"
          >
            <Flex.Column className="gap-0">
              <Flex.Row className="items-start justify-between gap-3">
                <Typography.Span
                  data-paint="holding.symbol"
                  className="font-semibold text-(--f1)"
                />
                <Typography.Span className="text-right font-semibold text-(--f2)">
                  P/L <span data-paint="holding.pnl" data-paint-format=".4f" />{" "}
                  USD
                </Typography.Span>
              </Flex.Row>

              <Flex.Row className="mt-1 items-center justify-between gap-3 text-[10px] text-(--f4)">
                <Typography.Span>
                  entry{" "}
                  <span
                    data-paint="holding.entry_price"
                    data-paint-format=".3f"
                  />{" "}
                  / mark{" "}
                  <span data-paint="holding.mark" data-paint-format=".3f" />
                </Typography.Span>
                <Typography.Span>
                  <span
                    data-paint="holding.return_pct"
                    data-paint-format=".4f"
                  />
                  %
                </Typography.Span>
              </Flex.Row>

              <div className="relative mt-1.5 h-0.75 overflow-visible rounded-full bg-[color-mix(in_srgb,var(--f4)_18%,transparent)]">
                <div
                  data-set="holding.progress_display"
                  data-target="style.display"
                  className="pointer-events-none absolute inset-y-0 left-[18%] w-[40%] rounded-full bg-[color-mix(in_srgb,var(--up)_18%,transparent)]"
                />
                <div
                  data-set="holding.progress_left"
                  data-target="style.left"
                  className="hidden"
                />
                <div
                  data-set="holding.progress_width"
                  data-target="style.width"
                  className="hidden"
                />
                <div
                  data-set="holding.progress_color"
                  data-target="style.background"
                  className="hidden"
                />
                <div
                  data-set="holding.stoploss.floor_pct"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 left-[18%] h-3 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-[color-mix(in_srgb,var(--down)_42%,transparent)]"
                />
                <div
                  data-set="holding.stoploss.peak_pct"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 left-[78%] h-3 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-[color-mix(in_srgb,var(--up)_42%,transparent)]"
                />
                <div
                  data-set="holding.entry_pct"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 left-[40%] h-1.5 w-px -translate-x-1/2 -translate-y-1/2 bg-[color-mix(in_srgb,var(--f4)_38%,transparent)]"
                />
                <div
                  data-set="holding.mark_pct"
                  data-target="style.left"
                  className="pointer-events-none absolute top-1/2 left-[58%] h-1.75 w-1.75 -translate-x-1/2 -translate-y-1/2 rounded-full border border-[color-mix(in_srgb,var(--surface)_70%,transparent)] bg-(--f1)"
                />
              </div>

              <Flex.Row className="mt-1 items-center justify-between gap-2 text-[9px] text-(--f4)">
                <Typography.Span className="text-(--down)">
                  floor{" "}
                  <span
                    data-paint="holding.stoploss.floor"
                    data-paint-format=".3f"
                  />
                </Typography.Span>
                <Typography.Span>
                  qty <span data-paint="holding.qty" data-paint-format=".4f" />
                </Typography.Span>
                <Typography.Span className="text-(--up)">
                  peak{" "}
                  <span
                    data-paint="holding.stoploss.peak"
                    data-paint-format=".3f"
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
