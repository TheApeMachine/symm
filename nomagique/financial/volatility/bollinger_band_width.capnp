@0xb2811ece94b6c32c;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface BollingerBandWidth {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
