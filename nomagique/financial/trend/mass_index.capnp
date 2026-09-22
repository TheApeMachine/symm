@0xefb0cd1254be1fcd;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface MassIndex {
    write @0 (
        high :Float64,
        low :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
