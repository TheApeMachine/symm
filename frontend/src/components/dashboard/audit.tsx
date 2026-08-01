import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

export const AuditTrail = () => (
  <Component registerKey="positions">
    {({ ref, className, slots }) => (
      <div ref={ref} className={cn("min-h-0 flex-1", className)}>
        <Typography.Span className="block border-(--line) border-b px-1 pb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-(--f3)">
          Audit trail
        </Typography.Span>
        <List className="min-h-0 border-(--line) border-b">
          {slots.map((slot) => (
            <List.Item key={slot} className="justify-between" data-index={slot}>
              <Typography.Span data-paint="holding.status" />
              <Typography.Span data-paint="holding.symbol" />
              <Typography.Span>
                pnl <span data-paint="holding.pnl" data-paint-format=".4f" /> ·
                return{" "}
                <span data-paint="holding.return_pct" data-paint-format=".4f" />
                %
              </Typography.Span>
            </List.Item>
          ))}
        </List>
      </div>
    )}
  </Component>
);
