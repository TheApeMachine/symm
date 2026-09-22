@0xd084e002c0233cf9;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface Rvi {
    write @0 (
        open :Float64,
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        rvi :Float64,
        signal :Float64,
    );
}
