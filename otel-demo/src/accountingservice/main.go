// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
    otelpyroscope "github.com/pyroscope-io/otel-profiling-go"
	"github.com/open-telemetry/opentelemetry-demo/src/accountingservice/kafka"
	"github.com/pyroscope-io/client/pyroscope"
)

var log *logrus.Logger

func init() {
	log = logrus.New()
	log.Level = logrus.DebugLevel
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
}

func initTracerProvider() (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

    pyroscope_server:=os.Getenv("PYROSCOPE_URL")

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)

	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp,
                               otelpyroscope.WithAppName("accounting.otel-demo"),
                               otelpyroscope.WithPyroscopeURL(pyroscope_server),
                               otelpyroscope.WithRootSpanOnly(true),
                               otelpyroscope.WithAddSpanName(true),
                               otelpyroscope.WithProfileURL(true),
                               otelpyroscope.WithProfileBaselineURL(true),
                              ))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}

func main() {
    pyroscope_server:=os.Getenv("PYROSCOPE_URL")
	tp, err := initTracerProvider()


    pyroscope.Start(pyroscope.Config{
        ApplicationName: "accounting.otel-demo",

        // replace this with the address of pyroscope server
        ServerAddress:  pyroscope_server,

        // you can disable logging by setting this to nil
        Logger:          pyroscope.StandardLogger,

        // optionally, if authentication is enabled, specify the API key:
        // AuthToken:    os.Getenv("PYROSCOPE_AUTH_TOKEN"),

        // you can provide static tags via a map:
        Tags:            map[string]string{"hostname": os.Getenv("HOSTNAME")},

        ProfileTypes: []pyroscope.ProfileType{
          // these profile types are enabled by default:
          pyroscope.ProfileCPU,
          pyroscope.ProfileAllocObjects,
          pyroscope.ProfileAllocSpace,
          pyroscope.ProfileInuseObjects,
          pyroscope.ProfileInuseSpace,

          // these profile types are optional:
          pyroscope.ProfileGoroutines,
          pyroscope.ProfileMutexCount,
          pyroscope.ProfileMutexDuration,
          pyroscope.ProfileBlockCount,
          pyroscope.ProfileBlockDuration,
        },
    })

	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	var brokers string
	mustMapEnv(&brokers, "KAFKA_SERVICE_ADDR")

	brokerList := strings.Split(brokers, ",")
	log.Printf("Kafka brokers: %s", strings.Join(brokerList, ", "))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := kafka.StartConsumerGroup(ctx, brokerList, log); err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()
}

func mustMapEnv(target *string, envKey string) {
	v := os.Getenv(envKey)
	if v == "" {
		panic(fmt.Sprintf("environment variable %q not set", envKey))
	}
	*target = v
}
