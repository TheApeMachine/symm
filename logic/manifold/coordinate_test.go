package manifold

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

func TestOrdersForSymbol(t *testing.T) {
	source := newTestBookSource("BTC/USD")
	orders, midPrice, ok := ordersForSymbol(source, "BTC/USD")

	if !ok || len(orders) == 0 || midPrice <= 0 {
		t.Fatalf("ordersForSymbol = %d mid=%v ok=%v", len(orders), midPrice, ok)
	}
}

func TestMapOrders(t *testing.T) {
	config := testPhysicsConfig()
	at := time.Unix(100, 0)

	orders := []physicalOrder{
		{
			orderID:   "bid-1",
			side:      book.Bid,
			price:     100,
			quantity:  2,
			timestamp: at.Add(-5 * time.Second),
		},
		{
			orderID:   "ask-1",
			side:      book.Ask,
			price:     101,
			quantity:  4,
			timestamp: at.Add(-10 * time.Second),
		},
	}

	mapped, epoch, ready := mapOrders(config, orders, 100.5, at, nil)

	if !ready {
		t.Fatal("mapOrders returned not ready")
	}

	if len(mapped) != 2 {
		t.Fatalf("mapped orders = %d, want 2", len(mapped))
	}

	if epoch == nil {
		t.Fatal("epoch is nil")
	}

	if len(epoch.positions) != 2 {
		t.Fatalf("epoch positions = %d, want 2", len(epoch.positions))
	}

	if epoch.spread <= 0 || epoch.buyCapacity != 404 || epoch.sellCapacity != 200 {
		t.Fatalf("execution boundary = %+v", epoch)
	}

	if mapped[0].mass <= 0 || mapped[0].omega <= 0 {
		t.Fatalf("mapped oscillator fields are invalid: %+v", mapped[0])
	}

	if mapped[0].posX >= config.DomainX/2 || mapped[1].posX <= config.DomainX/2 {
		t.Fatalf("book sides collapsed onto one coordinate side: %+v", mapped)
	}

	if rank := survivalCoordinate(5, []float64{5, 5, 10}); rank != 2.0/3.0 {
		t.Fatalf("upper-tie survival rank = %v, want %v", rank, 2.0/3.0)
	}
}

/*
TestMapOrdersOmegaFromTouchProximity derives oscillator frequency from
distance-to-mid, not from opaque order IDs, so identical book state shares
omega and near-touch resting size runs faster than deep book.
*/
func TestMapOrdersOmegaFromTouchProximity(t *testing.T) {
	t.Parallel()

	config := testPhysicsConfig()
	at := time.Unix(100, 0)
	orders := []physicalOrder{
		{
			orderID:   "id-near-a",
			side:      book.Bid,
			price:     100.4,
			quantity:  2,
			timestamp: at.Add(-5 * time.Second),
		},
		{
			orderID:   "id-near-b",
			side:      book.Bid,
			price:     100.4,
			quantity:  2,
			timestamp: at.Add(-5 * time.Second),
		},
		{
			orderID:   "id-deep",
			side:      book.Bid,
			price:     90,
			quantity:  2,
			timestamp: at.Add(-5 * time.Second),
		},
		{
			orderID:   "ask-1",
			side:      book.Ask,
			price:     100.6,
			quantity:  2,
			timestamp: at.Add(-5 * time.Second),
		},
	}

	mapped, _, ready := mapOrders(config, orders, 100.5, at, nil)

	if !ready {
		t.Fatal("mapOrders returned not ready")
	}

	if len(mapped) != 4 {
		t.Fatalf("mapped orders = %d, want 4", len(mapped))
	}

	if mapped[0].omega != mapped[1].omega {
		t.Fatalf(
			"identical book state produced divergent omega from order IDs: %v vs %v",
			mapped[0].omega, mapped[1].omega,
		)
	}

	if mapped[0].omega <= mapped[2].omega {
		t.Fatalf(
			"near-touch omega %v should exceed deep-book omega %v",
			mapped[0].omega, mapped[2].omega,
		)
	}

	askOrder := mapped[3]
	askBase := goldenPhase(askOrder.sequence)
	askOffset := math.Mod(askOrder.phase-askBase+math.Pi, 2*math.Pi) - math.Pi

	if askOffset < 0 {
		askOffset += 2 * math.Pi
	}

	if math.Abs(askOffset-math.Pi) > 0.1 {
		t.Fatalf(
			"ask phase should be π above its golden base: phase=%v base=%v offset=%v",
			askOrder.phase, askBase, askOffset,
		)
	}

	omegaMin := config.GateWidthMin()
	omegaMax := config.GateWidthMax()

	for _, order := range mapped {
		if order.omega < omegaMin || order.omega > omegaMax {
			t.Fatalf("omega %v outside [%v, %v]", order.omega, omegaMin, omegaMax)
		}
	}
}

func TestCohortsFromMappedOrders(t *testing.T) {
	config := testPhysicsConfig()
	orders := []mappedOrder{
		{
			mass:  0.25,
			posX:  1,
			posY:  1,
			posZ:  1,
			omega: 2,
			phase: 2*math.Pi - 0.1,
			heat:  0.25,
		},
		{
			mass:  0.75,
			posX:  1.01,
			posY:  1.01,
			posZ:  1.01,
			omega: 4,
			phase: 0.1,
			heat:  0.75,
		},
	}

	oscillators := cohortsFromMappedOrders(config, orders)

	if len(oscillators) != 1 {
		t.Fatalf("cohorts = %d, want 1", len(oscillators))
	}

	if math.Abs(oscillators[0].Amplitude-1) > 1e-12 {
		t.Fatalf("amplitude = %v, want cohort mass 1", oscillators[0].Amplitude)
	}

	// Zero velocities → cold PIC carrier resting on the positive CV floor
	// (Amplitude·floor), well below the old Amplitude·CV √15 wall.
	wantHeat := oscillators[0].Amplitude * specificEnergyFloorFraction * config.CV

	if math.Abs(oscillators[0].Heat-wantHeat) > 1e-12 {
		t.Fatalf("heat = %v, want %v (CV floor) for coherent zero-velocity cohort", oscillators[0].Heat, wantHeat)
	}

	if math.Abs(oscillators[0].Phase) > 0.11 {
		t.Fatalf("circular phase = %v, want near zero", oscillators[0].Phase)
	}

	if torusCell(config.DomainX, config.DomainX, config.GridX) != 0 ||
		torusCell(
			-config.DomainX/float64(config.GridX), config.DomainX, config.GridX,
		) != config.GridX-1 {
		t.Fatal("toroidal cell mapping did not wrap")
	}

	limited := config
	limited.MaxModes = 1
	equalMass := cohortsFromMappedOrders(limited, []mappedOrder{
		{mass: 0.5, posX: 0.9, posY: 0.9, posZ: 0.9},
		{mass: 0.5, posX: 0.1, posY: 0.1, posZ: 0.1},
	})

	if len(equalMass) != 1 || equalMass[0].PosX != 0.1 {
		t.Fatalf("equal-mass cohort selection is not deterministic: %+v", equalMass)
	}
}

/*
TestCohortHeatFromVelocityDispersion seeds Heat from PIC rest-frame kinetic
energy so sound speed tracks book kinematics instead of Amplitude·CV.
*/
func TestCohortHeatFromVelocityDispersion(t *testing.T) {
	t.Parallel()

	config := testPhysicsConfig()
	orders := []mappedOrder{
		{
			mass: 0.5,
			posX: 0.5,
			posY: 0.5,
			posZ: 0.5,
			velX: 0,
			velY: 0,
			velZ: 0,
		},
		{
			mass: 0.5,
			posX: 0.51,
			posY: 0.51,
			posZ: 0.51,
			velX: 2,
			velY: 0,
			velZ: 0,
		},
	}

	oscillators := cohortsFromMappedOrders(config, orders)

	if len(oscillators) != 1 {
		t.Fatalf("cohorts = %d, want 1", len(oscillators))
	}

	// ⟨v⟩ = 1, ⟨v²⟩ = 2, variance = 1, dispersion energy = 0.5, plus the
	// positive CV floor that keeps the gas cell finite. Amplitude = mass = 1.
	floor := specificEnergyFloorFraction * config.CV
	want := floor + 0.5

	if math.Abs(oscillators[0].Heat-want) > 1e-12 {
		t.Fatalf("heat = %v, want %v from velocity dispersion", oscillators[0].Heat, want)
	}

	if math.Abs(oscillators[0].Heat-config.CV) < 1e-9 {
		t.Fatal("heat must not collapse back onto Amplitude·CV")
	}
}

/*
TestColdCohortAdmitsForcingAtProductionDeltaT proves the √15 CV wall is gone:
coherent cold carriers sit on the positive CV floor (never zero, which would
NaN the gas kernel) yet keep Courant headroom at the derived 100ms fluid step.
*/
func TestColdCohortAdmitsForcingAtProductionDeltaT(t *testing.T) {
	t.Parallel()

	config := testPhysicsConfig()
	oscillators := cohortsFromMappedOrders(config, []mappedOrder{
		{mass: 0.4, posX: 0.25, posY: 0.5, posZ: 0.5},
		{mass: 0.6, posX: 0.75, posY: 0.5, posZ: 0.5},
	})

	if len(oscillators) != 2 {
		t.Fatalf("cohorts = %d, want 2", len(oscillators))
	}

	// Coherent (zero-variance) carriers rest on the CV floor: strictly positive
	// so the gas cell stays finite, but far below the old Amplitude·CV wall.
	floor := specificEnergyFloorFraction * config.CV

	for _, oscillator := range oscillators {
		wantHeat := oscillator.Amplitude * floor

		if math.Abs(oscillator.Heat-wantHeat) > 1e-12 {
			t.Fatalf("cold cohort heat = %v, want %v (CV floor)", oscillator.Heat, wantHeat)
		}
	}

	interval := 100 * time.Millisecond
	deltaT := integrationDeltaT(config, interval)
	speedLimit := config.AdvectiveDeltaT(1) / deltaT
	baseSpeed, err := boundSpeed(config, oscillators)

	if err != nil {
		t.Fatal(err)
	}

	if baseSpeed >= speedLimit {
		t.Fatalf(
			"cold baseSpeed %v saturates speedLimit %v; forcing would still collapse",
			baseSpeed, speedLimit,
		)
	}

	_, scale, err := applyForcing(config, testOutcome(), interval, oscillators)

	if err != nil {
		t.Fatal(err)
	}

	if scale <= 0 {
		t.Fatalf("scale = %v, want positive market impulse", scale)
	}
}

func TestOrdersFromBook(t *testing.T) {
	symbolBook := book.New()
	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		ID:        "bid-1",
		Price:     decimal.NewFromFloat64(100),
		Quantity:  decimal.NewFromFloat64(1.5),
		Timestamp: time.Unix(1, 0),
	})
	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		ID:        "ask-1",
		Price:     decimal.NewFromFloat64(101),
		Quantity:  decimal.NewFromFloat64(2.5),
		Timestamp: time.Unix(2, 0),
	})

	orders := ordersFromBook(symbolBook)

	if len(orders) != 2 {
		t.Fatalf("orders = %d, want 2", len(orders))
	}

	oneSided := ordersFromBook(&book.Book{Bids: symbolBook.Bids})

	if len(oneSided) != 1 {
		t.Fatalf("one-sided orders = %d, want 1", len(oneSided))
	}
}

func BenchmarkMapOrders(b *testing.B) {
	config := testPhysicsConfig()
	at := time.Unix(100, 0)
	orders := make([]physicalOrder, 128)

	for index := range orders {
		direction := book.BookDirection(book.Bid)
		price := 100 - float64(index)*0.01

		if index%2 != 0 {
			direction = book.Ask
			price = 101 + float64(index)*0.01
		}

		orders[index] = physicalOrder{
			orderID:   "order",
			side:      direction,
			price:     price,
			quantity:  1 + float64(index)*0.1,
			timestamp: at.Add(-time.Duration(index) * time.Second),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _, ready := mapOrders(config, orders, 100.5, at, nil)

		if !ready {
			b.Fatal("mapOrders returned not ready")
		}
	}
}
