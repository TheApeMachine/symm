using Go = import "/go.capnp";
@0xc04f8a6e15f52dc4;
$Go.package("data");
$Go.import("nomagique/data");

using import "../runtime/status.capnp".Status;

struct Metric {
    raw          @0 :Float64;
    normalized   @1 :Float64;
    standardized @2 :Float64;
    center       @3 :Float64;
    scale        @4 :Float64;
    unit         @5 :UnitType;
    timescale    @6 :Timescale;

    enum UnitType {
        dimensionless @0;
        count         @1;
        rate          @2;
        duration      @3;
        percent       @4;
        second        @5;
        perSecond     @6;
        nat           @7;
    }

    enum Timescale {
        instantaneous @0;
        perSecond     @1;
        perMinute     @2;
        perHour       @3;
        perDay        @4;
    }
}

interface MetricService {
    write @0 (
        raw          :Float64,
        normalized   :Float64,
        standardized :Float64,
        center       :Float64,
        scale        :Float64,
        unit         :Metric.UnitType,
        timescale    :Metric.Timescale
    ) -> stream;
    done @1 () -> (status :Status, read :Data);
}
