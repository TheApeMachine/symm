@0xbb81fb824f0571c6;

using Go = import "/go.capnp";
$Go.package("volume");
$Go.import("github.com/theapemachine/symm/nomagique/financial/volume");

interface Kvo {
    write @0 (
        high :Float64,
        low :Float64,
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        kvo :Float64,
        signal :Float64,
    );
}
