@0xfa1f130190b1f2a8;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface SharpeRatio {
    write @0 (
        outcome :Float64,
    ) -> stream;

    done @1 () -> (
        ratio :Float64,
    );
}
