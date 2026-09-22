@0x8c3499c6d4813427;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface UltimateOscillator {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
