@0x8512e04840dd64ee;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface ChaikinOscillator {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        co :Float64,
        ad :Float64,
    );
}
