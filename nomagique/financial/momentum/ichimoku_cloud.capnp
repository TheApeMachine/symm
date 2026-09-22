@0xc57d1e7d84983e2c;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface IchimokuCloud {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        conversionLine :Float64,
        baseLine :Float64,
        leadingSpanA :Float64,
        leadingSpanB :Float64,
        laggingLine :Float64,
    );
}
