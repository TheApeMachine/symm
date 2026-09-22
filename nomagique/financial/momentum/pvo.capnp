@0xdddf2f2d9a32f908;

using Go = import "/go.capnp";
$Go.package("momentum");
$Go.import("github.com/theapemachine/symm/nomagique/financial/momentum");

interface Pvo {
    write @0 (
        volume :Float64,
    ) -> stream;

    done @1 () -> (
        pvo :Float64,
        signal :Float64,
        histogram :Float64,
    );
}
