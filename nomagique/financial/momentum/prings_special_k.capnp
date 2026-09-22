@0x9d0d6f922e103d28;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface PringsSpecialK {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
