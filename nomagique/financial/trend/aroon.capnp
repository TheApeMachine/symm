@0x94a5120b770cd4e6;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Aroon {
    write @0 (
        high :Float64,
        low :Float64,
    ) -> stream;

    done @1 () -> (
        up :Float64,
        down :Float64,
    );
}
