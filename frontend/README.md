# SYMM Frontend

The dashboard connects to `ws://127.0.0.1:8765/ws` by default. Wallet snapshots drive the header cash balance; `equity` frames add the realistic all-cash balance after market-selling every open lot through the L2 book with taker fees (shown beside cash when positions are open). Wallet snapshots also hydrate `gauge_confidence` so gauges recover immediately after reconnect. The trades panel intentionally renders open positions only, with live `mark` events updating each card's P/L. The right rail splits trades and audit 50/50. The audit half receives realtime entry/exit action events only (`trade_*_fill`, `trade_*_error`); skips, high-volume measurement lines, and perspective lines remain in the JSONL sidecar but do not stream to the dashboard. Trade charts consume candle rows only; raw mark ticks do not become synthetic candles.

The **Signal Insight** page (`/diagnostics`) lists `runs/<name>_raw.jsonl` dumps through `/api/dumps`, runs a full-file analysis on selection through `/api/analyze`, and then streams debounced tail analyses over the websocket as `chart: "diagnostic"` frames whenever a dump grows. Dump inventory refreshes on `event: "dumps"`. Refresh still forces a full-file re-read.

The prediction chart consumes `chart: "prediction"` frames. Dashed orange `prediction` points are written at `measurement.at + story.prediction.horizon`; green `actual` and red `error` points are written to that same target time when future price catches up. The backend treats each signal confidence as a forward movement-intensity forecast over the horizon; settled error updates a per-source scale that signals consume upstream while scoring feature values.

The top-left **Confidence** tab scrolls per-source band clarity (what the gauges show, over time). The main **Surprise** tab scrolls per-source SNR — how far each signal's category standout sits above its own recent baseline. That pair separates "how clear is the reading right now" from "how unusual is this versus recent history."

## Source layout

```
src/
  components/
    dashboard/          # layout shell (DashboardLayout, PanelTabs, placeholders)
    charts/
      shared/             # SciChart theme + financial chart helpers
      confidence/         # gauges, heatmaps, confidence wire adapter
      fluid/              # 3D surface chart + field_row wire adapter
      prediction/         # forward forecast/actual/error wire adapter
      trade/              # OHLC chart + trade-chart-wire adapter
    panels/
      data/               # audit, decisions, wallet, trades panel stores
    audit.tsx, trades.tsx # panel UI (consume lib/symm hooks)
  lib/symm/               # wire protocol, layout schema, WS routing, stores
```

Chart adapters under `components/charts/*/` route websocket frames and call the mounted chart's update function. Each adapter is a thin `ingest*Wire` entry point plus a type guard; charts own SciChart state and do not mirror history in parallel stores. Trade candles additionally rely on the backend emitting numeric `sec` from `interval_begin`.

# Getting Started

To run this application:

```bash
npm install
npm run dev
```

# Building For Production

To build this application for production:

```bash
npm run build
```

## Testing

This project uses [Vitest](https://vitest.dev/) for testing. You can run the tests with:

```bash
npm run test
```

Benchmarks use the same Vitest runner:

```bash
npm run bench
```

## Styling

This project uses [Tailwind CSS](https://tailwindcss.com/) for styling.

### Removing Tailwind CSS

If you prefer not to use Tailwind CSS:

1. Remove the demo pages in `src/routes/demo/`
2. Replace the Tailwind import in `src/styles.css` with your own styles
3. Remove `tailwindcss()` from the plugins array in `vite.config.ts`
4. Uninstall the packages: `npm install @tailwindcss/vite tailwindcss -D`


## Shadcn

Add components using the latest version of [Shadcn](https://ui.shadcn.com/).

```bash
pnpm dlx shadcn@latest add button
```



## Routing

This project uses [TanStack Router](https://tanstack.com/router) with file-based routing. Routes are managed as files in `src/routes`.

### Adding A Route

To add a new route to your application just add a new file in the `./src/routes` directory.

TanStack will automatically generate the content of the route file for you.

Now that you have two routes you can use a `Link` component to navigate between them.

### Adding Links

To use SPA (Single Page Application) navigation you will need to import the `Link` component from `@tanstack/react-router`.

```tsx
import { Link } from "@tanstack/react-router";
```

Then anywhere in your JSX you can use it like so:

```tsx
<Link to="/about">About</Link>
```

This will create a link that will navigate to the `/about` route.

More information on the `Link` component can be found in the [Link documentation](https://tanstack.com/router/v1/docs/framework/react/api/router/linkComponent).

### Using A Layout

In the File Based Routing setup the layout is located in `src/routes/__root.tsx`. Anything you add to the root route will appear in all the routes. The route content will appear in the JSX where you render `{children}` in the `shellComponent`.

Here is an example layout that includes a header:

```tsx
import { HeadContent, Scripts, createRootRoute } from '@tanstack/react-router'

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: 'utf-8' },
      { name: 'viewport', content: 'width=device-width, initial-scale=1' },
      { title: 'My App' },
    ],
  }),
  shellComponent: ({ children }) => (
    <html lang="en">
      <head>
        <HeadContent />
      </head>
      <body>
        <header>
          <nav>
            <Link to="/">Home</Link>
            <Link to="/about">About</Link>
          </nav>
        </header>
        {children}
        <Scripts />
      </body>
    </html>
  ),
})
```

More information on layouts can be found in the [Layouts documentation](https://tanstack.com/router/latest/docs/framework/react/guide/routing-concepts#layouts).

## Server Functions

TanStack Start provides server functions that allow you to write server-side code that seamlessly integrates with your client components.

```tsx
import { createServerFn } from '@tanstack/react-start'

const getServerTime = createServerFn({
  method: 'GET',
}).handler(async () => {
  return new Date().toISOString()
})

// Use in a component
function MyComponent() {
  const [time, setTime] = useState('')
  
  useEffect(() => {
    getServerTime().then(setTime)
  }, [])
  
  return <div>Server time: {time}</div>
}
```

## API Routes

You can create API routes by using the `server` property in your route definitions:

```tsx
import { createFileRoute } from '@tanstack/react-router'
import { json } from '@tanstack/react-start'

export const Route = createFileRoute('/api/hello')({
  server: {
    handlers: {
      GET: () => json({ message: 'Hello, World!' }),
    },
  },
})
```

## Data Fetching

There are multiple ways to fetch data in your application. You can use TanStack Query to fetch data from a server. But you can also use the `loader` functionality built into TanStack Router to load the data for a route before it's rendered.

For example:

```tsx
import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/people')({
  loader: async () => {
    const response = await fetch('https://swapi.dev/api/people')
    return response.json()
  },
  component: PeopleComponent,
})

function PeopleComponent() {
  const data = Route.useLoaderData()
  return (
    <ul>
      {data.results.map((person) => (
        <li key={person.name}>{person.name}</li>
      ))}
    </ul>
  )
}
```

Loaders simplify your data fetching logic dramatically. Check out more information in the [Loader documentation](https://tanstack.com/router/latest/docs/framework/react/guide/data-loading#loader-parameters).

# Demo files

Files prefixed with `demo` can be safely deleted. They are there to provide a starting point for you to play around with the features you've installed.

# Learn More

You can learn more about all of the offerings from TanStack in the [TanStack documentation](https://tanstack.com).

For TanStack Start specific documentation, visit [TanStack Start](https://tanstack.com/start).
