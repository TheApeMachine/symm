@0xfc68d5b2828ccad2;

using Go = import "/go.capnp";
$Go.package("volatility");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volatility");

interface ChandelierExit {
    write @0 (
        high :Float64,
        low :Float64,
        close :Float64,
    ) -> stream;

    done @1 () -> (
        exitLong :Float64,
        exitShort :Float64,
    );
}
