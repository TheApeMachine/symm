@0xa625f78f3dbd3a99;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface AwesomeOscillator {
    write @0 (
        high :Float64,
        low :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
