@0xb15b62d9a5061876;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface ElderRay {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        bullPower :Float64,
        bearPower :Float64,
    );
}
