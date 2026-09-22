@0x87333dd6bd481918;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Envelope {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        upper :Float64,
        middle :Float64,
        lower :Float64,
    );
}
