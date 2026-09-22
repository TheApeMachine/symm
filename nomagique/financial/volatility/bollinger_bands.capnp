@0x9aa3ec693b33a4a4;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface BollingerBands {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        upper :Float64,
        middle :Float64,
        lower :Float64,
    );
}
