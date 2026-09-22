@0xae70b775026804b3;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface Po {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
