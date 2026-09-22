@0xe2579936b99f63f9;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Stochastic {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        k :Float64,
        d :Float64,
    );
}
