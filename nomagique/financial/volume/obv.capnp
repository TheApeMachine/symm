@0xdd0ba5be70b3e9fe;

using Go = import "/go.capnp";
$Go.package("volume");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volume");

interface Obv {
    write @0 (
        close :Float64,
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
