@0xc239538f12291bf5;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface Ppo {
    write @0 (
        close :Float64,
    ) -> stream;

    done @1 () -> (
        ppo :Float64,
        signal :Float64,
        histogram :Float64,
    );
}
