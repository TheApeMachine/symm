@0xab50a3363cebd46c;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface BuyAndHold {
    write @0 (
        open :Float64,
        high :Float64,
        low :Float64,
        close :Float64,
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        action :Int64,
    );
}
