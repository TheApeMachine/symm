@0xb9443d6d3a8fb309;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface AccelerationBands {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        upper :Float64,
        middle :Float64,
        lower :Float64,
    );
}
