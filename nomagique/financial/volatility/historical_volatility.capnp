@0xc53caeddb2a8f458;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface HistoricalVolatility {
    write @0 (
        price :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
