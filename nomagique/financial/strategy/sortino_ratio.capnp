@0xa432876860e68dfa;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface SortinoRatio {
    write @0 (
        outcome :Float64,
    ) -> stream;

    done @1 () -> (
        ratio :Float64,
    );
}
