@0xec2ff027b10c79cf;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface DonchianChannel {
    write @0 (
        high :Float64,
        low :Float64,
    ) -> stream;

    done @1 () -> (
        upper :Float64,
        middle :Float64,
        lower :Float64,
    );
}
