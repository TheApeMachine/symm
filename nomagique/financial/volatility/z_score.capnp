@0x8af97c34434bad92;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface ZScore {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
