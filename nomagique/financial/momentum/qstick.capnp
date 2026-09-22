@0xd9a690b4a2fdc8b6;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface Qstick {
    write @0 (
        open :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
