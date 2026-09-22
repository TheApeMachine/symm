@0x8d60f101ad89c0ac;

using Go = import "/go.capnp";
$Go.package("volume");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volume");

interface Emv {
    write @0 (
        high :Float64,
        low :Float64,
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
