@0xa70c9587720c3040;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface Chop {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
