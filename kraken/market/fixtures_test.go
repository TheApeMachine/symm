package market

var sampleTickerFrame = []byte(`{
  "channel":"ticker",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "bid":49999.5,
      "bid_qty":1.2,
      "ask":50000.5,
      "ask_qty":0.8,
      "last":50000.0,
      "volume":12.5,
      "vwap":49995.0,
      "low":49800.0,
      "high":50100.0,
      "change":100.0,
      "change_pct":0.2,
      "timestamp":"2026-05-23T02:00:00.123456789Z"
    }
  ]
}`)

var sampleCandleFrame = []byte(`{
  "channel":"ohlc",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "open":49900.0,
      "high":50100.0,
      "low":49800.0,
      "close":50000.0,
      "vwap":49950.0,
      "trades":42,
      "volume":12.5,
      "interval_begin":"2026-05-23T02:00:00.000000000Z",
      "interval":1
    }
  ]
}`)

var sampleLevel3Frame = []byte(`{
  "channel":"level3",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "checksum":123456,
      "timestamp":"2026-05-23T02:00:00.123456789Z",
      "bids":[
        {
          "event":"add",
          "order_id":"OABC-123",
          "limit_price":49999.5,
          "order_qty":1.2,
          "timestamp":"2026-05-23T02:00:00.123456789Z"
        }
      ],
      "asks":[]
    }
  ]
}`)

var sampleTradeFrame = []byte(`{
  "channel":"trade",
  "type":"update",
  "data":[
    {"symbol":"BTC/EUR","side":"buy","qty":0.25,"price":50000.1,"timestamp":"2026-05-23T02:00:00.123456789Z"},
    {"symbol":"BTC/EUR","side":"sell","qty":0.10,"price":50000.2,"timestamp":"2026-05-23T02:00:00.223456789Z"}
  ]
}`)

var sampleBookFrame = []byte(`{
  "channel":"book",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "bids":[{"price":49999.5,"qty":1.2}],
      "asks":[{"price":50000.5,"qty":0.8}]
    }
  ]
}`)

var sampleInstrumentFrame = []byte(`{
  "channel":"instrument",
  "type":"snapshot",
  "data":{
    "assets":[],
    "pairs":[
      {
        "symbol":"BTC/EUR",
        "base":"BTC",
        "quote":"EUR",
        "status":"online",
        "qty_precision":8,
        "qty_increment":1e-8,
        "price_precision":1,
        "cost_precision":5,
        "marginable":true,
        "has_index":true,
        "cost_min":0.45,
        "price_increment":0.1,
        "qty_min":0.0001
      },
      {
        "symbol":"BTC/USD",
        "base":"BTC",
        "quote":"USD",
        "status":"online",
        "qty_precision":8,
        "qty_increment":1e-8,
        "price_precision":1,
        "cost_precision":5,
        "marginable":true,
        "has_index":true,
        "cost_min":0.5,
        "price_increment":0.1,
        "qty_min":0.0001
      }
    ]
  }
}`)
