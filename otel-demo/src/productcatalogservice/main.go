// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
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
	"io/ioutil"
	"net"
	"os"
	"strings"
    "net/http"
	pb "github.com/opentelemetry/opentelemetry-demo/src/productcatalogservice/genproto/hipstershop"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/sirupsen/logrus"
    run "runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"google.golang.org/protobuf/encoding/protojson"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	 otelpyroscope "github.com/pyroscope-io/otel-profiling-go"
	 "github.com/pyroscope-io/client/pyroscope"
)

var (
	log     *logrus.Logger
	catalog []*pb.Product
)

func init() {
    run.SetMutexProfileFraction(5)
    run.SetBlockProfileRate(5)
	log = logrus.New()
	catalog = readCatalogFile()
}

func initTracerProvider() *sdktrace.TracerProvider {
	 ctx := context.Background()
     pyroscope_server:=os.Getenv("PYROSCOPE_URL")
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		log.Fatalf("OTLP Trace gRPC Creation: %v", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp,
                                       otelpyroscope.WithAppName("productcatalogservice.otel-demo"),
                                       otelpyroscope.WithPyroscopeURL(pyroscope_server),
                                       otelpyroscope.WithRootSpanOnly(true),
                                       otelpyroscope.WithAddSpanName(true),
                                       otelpyroscope.WithProfileURL(true),
                                       otelpyroscope.WithProfileBaselineURL(true),
            ))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp
}

func main() {
      go func() {
        		log.Println(http.ListenAndServe(":6060", nil))
        }()
    pyroscope_server:=os.Getenv("PYROSCOPE_URL")

    pyroscope.Start(pyroscope.Config{
            ApplicationName: "productcatalogservice.otel-demo",

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
	tp := initTracerProvider()
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Fatalf("Tracer Provider Shutdown: %v", err)
		}
	}()

	svc := &productCatalog{}
	var port string
	mustMapEnv(&port, "PRODUCT_CATALOG_SERVICE_PORT")
	mustMapEnv(&svc.featureFlagSvcAddr, "FEATURE_FLAG_GRPC_SERVICE_ADDR")

	log.Infof("ProductCatalogService gRPC server started on port: %s", port)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("TCP Listen: %v", err)
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
	)

	pb.RegisterProductCatalogServiceServer(srv, svc)
	healthpb.RegisterHealthServer(srv, svc)
	srv.Serve(ln)
}

type productCatalog struct {
	featureFlagSvcAddr string
	pb.UnimplementedProductCatalogServiceServer
}

func readCatalogFile() []*pb.Product {

	catalogJSON, err := ioutil.ReadFile("products.json")
	if err != nil {
		log.Fatalf("Reading Catalog File: %v", err)
	}

	var res pb.ListProductsResponse
	if err := protojson.Unmarshal(catalogJSON, &res); err != nil {
		log.Fatalf("Parsing Catalog JSON: %v", err)
	}

	return res.Products
}

func mustMapEnv(target *string, key string) {
	value, present := os.LookupEnv(key)
	if !present {
		log.Fatalf("Environment Variable Not Set: %q", key)
	}
	*target = value
}

func (p *productCatalog) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (p *productCatalog) Watch(req *healthpb.HealthCheckRequest, ws healthpb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "health check via Watch not implemented")
}

func (p *productCatalog) ListProducts(ctx context.Context, req *pb.Empty) (*pb.ListProductsResponse, error) {
	span := trace.SpanFromContext(ctx)

	span.SetAttributes(
		attribute.Int("app.products.count", len(catalog)),
	)
	return &pb.ListProductsResponse{Products: catalog}, nil
}

func (p *productCatalog) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.Product, error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String("app.product.id", req.Id),
	)

	// GetProduct will fail on a specific product when feature flag is enabled
	if p.checkProductFailure(ctx, req.Id) {
		msg := fmt.Sprintf("Error: ProductCatalogService Fail Feature Flag Enabled")
		span.SetStatus(otelcodes.Error, msg)
		span.AddEvent(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}

	var found *pb.Product
	for _, product := range catalog {
		if req.Id == product.Id {
			found = product
			break
		}
	}

	if found == nil {
		msg := fmt.Sprintf("Product Not Found: %s", req.Id)
		span.SetStatus(otelcodes.Error, msg)
		span.AddEvent(msg)
		return nil, status.Errorf(codes.NotFound, msg)
	}

	msg := fmt.Sprintf("Product Found - ID: %s, Name: %s", req.Id, found.Name)
	span.AddEvent(msg)
	span.SetAttributes(
		attribute.String("app.product.name", found.Name),
	)
	return found, nil
}

func (p *productCatalog) SearchProducts(ctx context.Context, req *pb.SearchProductsRequest) (*pb.SearchProductsResponse, error) {
	span := trace.SpanFromContext(ctx)

	var result []*pb.Product
	for _, product := range catalog {
		if strings.Contains(strings.ToLower(product.Name), strings.ToLower(req.Query)) ||
			strings.Contains(strings.ToLower(product.Description), strings.ToLower(req.Query)) {
			result = append(result, product)
		}
	}
	span.SetAttributes(
		attribute.Int("app.products_search.count", len(result)),
	)
	return &pb.SearchProductsResponse{Results: result}, nil
}

func (p *productCatalog) checkProductFailure(ctx context.Context, id string) bool {
	if id != "OLJCESPC7Z" {
		return false
	}

	conn, err := createClient(ctx, p.featureFlagSvcAddr)
	if err != nil {
		span := trace.SpanFromContext(ctx)
		span.AddEvent("error", trace.WithAttributes(attribute.String("message", "Feature Flag Connection Failed")))
		return false
	}
	defer conn.Close()

     flagName := "productCatalogFailure"
    ffResponse, err := pb.NewFeatureFlagServiceClient(conn).GetFlag(ctx, &pb.GetFlagRequest{
        Name: flagName,
    })


	if err != nil {
		span := trace.SpanFromContext(ctx)
		span.AddEvent("error", trace.WithAttributes(attribute.String("message", fmt.Sprintf("GetFlag Failed: %s", flagName))))
		return false
	}

	return ffResponse.GetFlag().Enabled
}

func createClient(ctx context.Context, svcAddr string) (*grpc.ClientConn, error) {
	return grpc.DialContext(ctx, svcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
	)
}
