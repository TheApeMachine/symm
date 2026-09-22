@0xe8ddf666503010e1;

using Go = import "/go.capnp";
$Go.package("volume");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volume");

interface Mfv {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
