package perspectives

import "strings"

/*
DashboardGaugeGridCapacity is the maximum number of gauges in the top dashboard grid.
Additional registered sources render in the vertical strip beside the candlestick chart.
*/
const DashboardGaugeGridCapacity = 8

var dashboardPrimarySourceOrder = []string{
	"hawkes",
	"fluid",
	"pumpdump",
	"causal",
	"depthflow",
	"leadlag",
	"liquidity",
	"sentiment",
}

/*
SplitDashboardGaugeSources assigns registered sources to the top grid and side strip.
Primary sources fill the grid first in stable order; overflow sources go to the strip.
*/
func SplitDashboardGaugeSources(registered []string) (grid []string, strip []string) {
	if len(registered) == 0 {
		return nil, nil
	}

	present := make(map[string]struct{}, len(registered))

	for _, name := range registered {
		trimmed := strings.TrimSpace(name)

		if trimmed == "" {
			continue
		}

		present[trimmed] = struct{}{}
	}

	for _, name := range dashboardPrimarySourceOrder {
		if _, ok := present[name]; !ok {
			continue
		}

		grid = append(grid, name)

		if len(grid) >= DashboardGaugeGridCapacity {
			break
		}
	}

	if len(grid) < DashboardGaugeGridCapacity {
		for _, name := range registered {
			trimmed := strings.TrimSpace(name)

			if trimmed == "" || containsString(grid, trimmed) {
				continue
			}

			grid = append(grid, trimmed)

			if len(grid) >= DashboardGaugeGridCapacity {
				break
			}
		}
	}

	for _, name := range registered {
		trimmed := strings.TrimSpace(name)

		if trimmed == "" || containsString(grid, trimmed) {
			continue
		}

		strip = append(strip, trimmed)
	}

	return grid, strip
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}
