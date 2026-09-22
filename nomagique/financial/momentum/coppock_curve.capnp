@0x9df9a72749b2cc4a;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface CoppockCurve {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
