@0xae05866fb1649048;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Vwma {
    write @0 (
        close :Float64,
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
