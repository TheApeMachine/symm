import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";

const slotCount = 6;

export const Decisions = () => (
  <Component registerKey="decisions">
    {({ ref, className }) => (
      <Flex.Column ref={ref} className={cn("h-full min-h-0 gap-0", className)}>
        <Flex.Row className="items-baseline justify-between border-(--line) border-b px-3 py-2">
          <Typography.Span className="text-[10px] font-semibold uppercase tracking-[0.13em] text-(--f3)">
            DECISIONS
          </Typography.Span>
        </Flex.Row>
        <Flex.Row className="justify-between px-3 py-1.25 text-[9.5px] uppercase tracking-[0.06em] text-(--f4)">
          <Typography.Span>Symbol</Typography.Span>
          <Typography.Span>Utility</Typography.Span>
          <Typography.Span>Action</Typography.Span>
        </Flex.Row>
        <List className="min-h-0 flex-1 overflow-auto">
          {Array.from({ length: slotCount }, (_, index) => (
            <List.Item
              key={`${index}-decision`}
              className="justify-between"
              data-index={index}
            >
              <Typography.Span
                data-paint="symbol"
                className="truncate text-(--f1)"
              />
              <Typography.Span
                data-paint="utility"
                data-paint-format=".4f"
                className="shrink-0 text-(--f2) text-right"
              />
              <Typography.Span
                data-paint="action"
                className="shrink-0 text-(--acc) uppercase"
              />
            </List.Item>
          ))}
        </List>
      </Flex.Column>
    )}
  </Component>
);
