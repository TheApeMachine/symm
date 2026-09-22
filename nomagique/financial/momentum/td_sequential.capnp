@0xf05ae9a0d6fd8e68;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface TdSequential {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        buySetup :Float64,
        sellSetup :Float64,
        buyCountdown :Float64,
        sellCountdown :Float64,
    );
}
