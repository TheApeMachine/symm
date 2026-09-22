@0xad5f429240733254;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface AnnualizedHistoricalVolatility {
    write @0 (
        price :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
