@0x85d3acc39d94e0f8;

using import "metric.capnp".WireMetric;

struct WireMap(Key, Value) {
    entries @0 :List(Entry);

    struct Entry {
        key   @0 :Key;
        value @1 :Value;
    }
}

struct MetadataValue {
    union {
        id    @0 :Data;
        text  @1 :Text;
        int   @2 :Int64;
        float @3 :Float64;
        bool  @4 :Bool;
    }
}

struct WireMeasurement  {
    id         @0  :Data;
    epoch      @1  :Int64;
    tick       @2  :Int64;
    timestamp  @3  :Int64;
    label      @4  :Data;
    entity     @5  :EntityType;
    source     @6  :SourceType;
    snr        @7  :Float64;
    maturity   @8  :Float64;
    separation @9  :Float64;
    metrics    @10 :List(WireMetric);
    metadata   @11 :WireMap(Text, MetadataValue);

    enum EntityType {
        ticker       @0;
        trade        @1;
        level3       @2;
        instrument   @3;
        order        @4;
        balance      @5;
        execution    @6;
        tradeHistory @7;
        openOrders   @8;
        tradeBalance @9;
        tradeVolume  @10;
    }

    enum SourceType {
        public      @0;
        private     @1;
        level3      @2;
        correlation @3;
        csv         @4;
        depthflow   @5;
        derivatives @6;
        hawkes      @7;
        leadlag     @8;
        liquidity   @9;
        morphology  @10;
        pumpdump    @11;
        sentiment   @12;
        toxicity    @13;
        category    @14;
        cognition   @15;
        resonance   @16;
        manifold    @17;
        training    @18;
    }
}