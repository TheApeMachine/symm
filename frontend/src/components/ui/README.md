# ui

A terminal-flavoured component set: dark plates, hairline rules, mono readouts,
semantic tone on a four-step foreground ramp.

## Taking it to another project

Copy this folder, then:

1. `@import "./components/ui/theme.css";` after `@import "tailwindcss";`.
2. Provide `cn` at `@/lib/utils` — `twMerge(clsx(inputs))`.
3. `pnpm add class-variance-authority clsx tailwind-merge motion`.

That is the whole contract. Nothing here imports from `#/collections`,
`#/providers`, or app types, with one deliberate exception noted below.

Re-skin by redeclaring any of `theme.css`'s custom properties under your own
`:root`. No component hardcodes a colour.

## The rule that shapes everything here

**Primitives forward their props.** Every component spreads what it does not
consume onto a real DOM node, and every value slot takes a `ReactNode`.

This is not politeness about `className`. This UI paints: a websocket frame is
written straight into the DOM through `Component`, which hands down a `ref`,
scans the subtree it wraps for `data-paint` / `data-set` / `data-append` /
`data-stream-value`, and writes those nodes directly. Nothing re-renders. A
readout updating several times a second costs one `textContent` assignment.

A primitive that swallowed `data-*` or dropped a `ref` would be unusable inside
a painted region — which is most of this app. So:

```tsx
<Component registerKey="equity">
  {({ ref }) => (
    <Stat
      ref={ref}
      label="Equity"
      value={<span data-paint="equity" data-paint-format=".2f" />}
    />
  )}
</Component>
```

`Stat` never sees the number. It renders the box; the socket writes the digits.

The corollary for authors: **do not put live values in React state.** If a value
changes with the feed, give it a slot and let it be painted.

`Sparkline` is the clearest case — it renders no data at all, only the two
shapes that `data-append` pushes points into, plus the geometry attributes the
writer needs. See its comment for why that geometry has to be declared in one
place.

## Layout

- `Flex`, `Flex.Row`, `Flex.Column`, `Flex.Center` — motion-backed, with an
  `appear` prop for presets.
- `Grid` and its presets (`Grid.Auto`, `Grid.Bento`, `Grid.Sidebar`, …).
- `Section` + `Section.Header` / `Section.Body` — a titled pane.
- `Toolbar` + `Toolbar.Group` / `Toolbar.Spacer` — a strip of peer controls.
- `Nav` + `Nav.Group` / `Nav.Item` / `Nav.Footer` — the surface rail.
  `Nav.Item` is polymorphic: `<Nav.Item as={Link} to="/graph" …/>`.
- `Panel`, `Divider`, `Canvas`, `Scanlines`.

`Section.Header` titles the region under it; `Toolbar` is a row where nothing is
the title. That is the whole difference, and it is worth keeping.

## Content

- `Typography` — `.Label` is the overline that titles a rail or a stat;
  `.Mono` is the tabular numeric readout; the rest are tags.
- `Badge` (what a thing *is*, semantic tone) vs `Chip` (what a thing
  *measures*, neutral). Reaching for a `Badge` where a `Chip` belongs is how
  status colour stops meaning anything.
- `Alert` — a full-width band saying the surface below it is not telling the
  whole truth. It shares Badge's tint recipe, so a failure is the same colour of
  event wherever it is reported. A Badge labels a thing inside a layout; an
  Alert interrupts the layout. Error and warning bands carry the `broken` glyph
  by default, so the state does not rest on colour alone.
- `Stat`, `Meter`, `Dot`, `Sparkline`, `Spinner`, `Key`.

## Controls

- `Button` — `variant` is chrome (`solid` / `outline` / `quiet` / `bare`),
  `tone` is meaning. `bare` is for clickable regions that must not look like
  buttons but still need the element's keyboard behaviour.
- `Input`, `Input.Field`, `Input.Search`.
- `List`, `List.Item` (read), `List.Option` (choose), `List.Empty`.
- `Overlay` → `Modal`. `Overlay` hides through the `hidden` attribute rather
  than unmounting, so painted nodes inside it survive open/close.

## The exception

`component.tsx` imports `registerPainter` from `#/providers/ws-stores`. It is
the painting runtime, not a widget. A project adopting this library without a
feed can delete `component.tsx` and `paint.ts` and take everything else
unchanged.
