@0xdd4ba30ddb382e2a;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface BoP {
    write @0 (
        opening :Float64,
        high    :Float64,
        low     :Float64,
        close   :Float64,
    ) -> stream;

    done @1 () -> (result :Float64);
}