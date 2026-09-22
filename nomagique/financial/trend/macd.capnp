@0xc9290871b83060a1;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Macd {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        macd :Float64,
        signal :Float64,
    );
}
