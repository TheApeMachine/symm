@0xe03ece296ee1cd29;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface Rsi {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
