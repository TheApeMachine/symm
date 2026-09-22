@0x8305e8478483f714;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface Fisher {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
