@0xd4dc2b4f1200f3b8;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface UlcerIndex {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
