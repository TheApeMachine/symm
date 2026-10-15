package strategy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

type mockTrainerS3Backend struct {
	sync.Mutex
	objects map[string][]byte
}

func newMockTrainerS3Backend() *mockTrainerS3Backend {
	return &mockTrainerS3Backend{
		objects: make(map[string][]byte),
	}
}

func (backend *mockTrainerS3Backend) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	backend.Lock()
	defer backend.Unlock()

	path := strings.TrimPrefix(request.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	key := ""

	if len(parts) == 2 {
		key = parts[1]
	}

	if request.Method == http.MethodGet && request.URL.Query().Get("list-type") == "2" {
		prefix := request.URL.Query().Get("prefix")
		delimiter := request.URL.Query().Get("delimiter")
		var contentsXML strings.Builder

		if delimiter != "" {
			seenPrefixes := make(map[string]bool)

			for objKey := range backend.objects {
				if prefix == "" || strings.HasPrefix(objKey, prefix) {
					sub := strings.TrimPrefix(objKey, prefix)
					idx := strings.Index(sub, delimiter)

					if idx >= 0 {
						commonPrefix := prefix + sub[:idx+len(delimiter)]

						if !seenPrefixes[commonPrefix] {
							seenPrefixes[commonPrefix] = true
							contentsXML.WriteString(fmt.Sprintf("<CommonPrefixes><Prefix>%s</Prefix></CommonPrefixes>", commonPrefix))
						}
					}

					if idx < 0 {
						contentsXML.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size></Contents>", objKey, len(backend.objects[objKey])))
					}
				}
			}
		}

		if delimiter == "" {
			for objKey := range backend.objects {
				if prefix == "" || strings.HasPrefix(objKey, prefix) {
					contentsXML.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size></Contents>", objKey, len(backend.objects[objKey])))
				}
			}
		}

		xmlResponse := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
    <Name>%s</Name>
    <Prefix>%s</Prefix>
    <KeyCount>1</KeyCount>
    <MaxKeys>1000</MaxKeys>
    <IsTruncated>false</IsTruncated>
    %s
</ListBucketResult>`, parts[0], prefix, contentsXML.String())

		writer.Header().Set("Content-Type", "application/xml")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(xmlResponse))
		return
	}

	if request.Method == http.MethodPut {
		body, err := io.ReadAll(request.Body)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		backend.objects[key] = body
		writer.WriteHeader(http.StatusOK)
		return
	}

	if request.Method == http.MethodGet {
		payload, exists := backend.objects[key]

		if !exists {
			http.Error(writer, "Not found", http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.WriteHeader(http.StatusOK)
		writer.Write(payload)
		return
	}

	http.NotFound(writer, request)
}

func newTestPrice(ctx context.Context, symbol string) *broker.Price {
	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT", Decimals: 8},
			"USD": {AltName: "ZUSD", Decimals: 2},
		},
		NewPairs: map[string]spot.AssetPair{
			symbol: {
				WSName:        symbol,
				Base:          "BTC",
				Quote:         "USD",
				PairDecimals:  2,
				CostDecimals:  2,
				LotDecimals:   8,
				LotMultiplier: 1,
				TickSize:      decimal.NewFromFloat64(0.01),
			},
		},
	})

	priceSystem := broker.NewPrice(ctx, nil, nil, nil, normalizer)
	priceSystem.SetFee(symbol, kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
	priceSystem.SetReferenceCash(decimal.NewFromFloat64(200))

	return priceSystem
}

func TestTrainer(t *testing.T) {
	system.NewConfig()
	ctx := t.Context()

	Convey("Trainer", t, func() {
		symbol := "BTC/USD"
		price := newTestPrice(ctx, symbol)
		storeTee := hindsight.NewStoreTee(ctx, "test-trainer")
		storeTee.Transition(runtime.READY)

		backend := newMockTrainerS3Backend()
		server := httptest.NewServer(backend)
		defer server.Close()

		client := store.NewClient(ctx, "test-bucket", func(options *s3.Options) {
			options.BaseEndpoint = aws.String(server.URL)
			options.UsePathStyle = true
			options.Region = "us-east-1"
			options.Credentials = credentials.NewStaticCredentialsProvider("test", "test", "")
		})

		Convey("NewTrainer requires client, storeTee and price", func() {
			trainerMissingClient := NewTrainer(ctx, nil, storeTee, price)
			So(trainerMissingClient.Error(), ShouldNotBeNil)

			trainerMissingTee := NewTrainer(ctx, client, nil, price)
			So(trainerMissingTee.Error(), ShouldNotBeNil)

			trainerMissingPrice := NewTrainer(ctx, client, storeTee, nil)
			So(trainerMissingPrice.Error(), ShouldNotBeNil)

			trainerValid := NewTrainer(ctx, client, storeTee, price)
			So(trainerValid.Error(), ShouldBeNil)
		})

		Convey("Generate executes clean streaming fanout and publishes policies", func() {
			trainer := NewTrainer(ctx, client, storeTee, price)
			So(trainer.Error(), ShouldBeNil)

			now := time.Now().UTC()
			var seqIdx int64

			for step := range 10 {
				at := now.Add(time.Duration(step) * time.Millisecond)

				for _, source := range SensorySources {
					seqIdx++
					measurement := data.NewMeasurement(1, symbol, source, at, at)
					measurement.SeqIdx = seqIdx

					if source == "liquidity" {
						measurement = measurement.Write(
							data.NewMetric("best_bid_price", 60000.0, data.UnitNone, data.TimescaleTick),
							data.NewMetric("best_ask_price", 60010.0, data.UnitNone, data.TimescaleTick),
						)
					}

					if source != "liquidity" {
						measurement = measurement.Write(
							data.NewMetric("value", float64(step*10), data.UnitNone, data.TimescaleTick),
						)
					}

					payload, err := measurement.MarshalBinary()
					So(err, ShouldBeNil)

					err = client.Put(measurement.Prefix(), payload)
					So(err, ShouldBeNil)
				}
			}

			for excursionIdx := range 2 {
				seqIdx++
				ignitionAt := now.Add(time.Duration(excursionIdx*3) * time.Millisecond)
				exhaustionAt := now.Add(time.Duration(excursionIdx*3+2) * time.Millisecond)

				detectorMeasurement := data.NewMeasurement(
					1, symbol, "detector", exhaustionAt, ignitionAt,
					&data.StringEntry{Key: "type", Value: excursionUp},
				)
				detectorMeasurement.SeqIdx = seqIdx
				detectorMeasurement = detectorMeasurement.Write(
					data.NewExactMetric("b_price", decimal.NewFromFloat64(60000.0), data.UnitNone, data.TimescaleTick),
					data.NewExactMetric("c_price", decimal.NewFromFloat64(60500.0), data.UnitNone, data.TimescaleTick),
				)

				payload, err := detectorMeasurement.MarshalBinary()
				So(err, ShouldBeNil)

				err = client.Put(detectorMeasurement.Prefix(), payload)
				So(err, ShouldBeNil)
			}

			err := trainer.Generate(1)
			So(err, ShouldBeNil)
		})
	})
}
