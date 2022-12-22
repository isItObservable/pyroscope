/*
 * Copyright 2018, Google LLC.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package hipstershop;

import com.google.common.collect.ImmutableListMultimap;
import com.google.common.collect.Iterables;
import hipstershop.Demo.Ad;
import hipstershop.Demo.AdRequest;
import hipstershop.Demo.AdResponse;
import io.grpc.Server;
import io.grpc.ServerBuilder;
import io.grpc.StatusRuntimeException;
import io.grpc.health.v1.HealthCheckResponse.ServingStatus;
import io.grpc.protobuf.services.*;
import io.grpc.stub.StreamObserver;
import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.common.AttributeKey;
import io.opentelemetry.api.common.Attributes;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.StatusCode;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.api.trace.TracerBuilder;
import io.opentelemetry.context.Scope;
import io.opentelemetry.instrumentation.annotations.SpanAttribute;
import io.opentelemetry.instrumentation.annotations.WithSpan;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Optional;
import java.util.Random;
import org.apache.logging.log4j.Level;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import io.pyroscope.javaagent.*;
import io.pyroscope.javaagent.config.Config;
import io.pyroscope.http.Format;
import io.otel.pyroscope.PyroscopeOtelConfiguration;
import io.otel.pyroscope.PyroscopeOtelSpanProcessor;
import io.otel.pyroscope.shadow.labels.Pyroscope;

public final class AdService {

  private static final Logger logger = LogManager.getLogger(AdService.class);

  @SuppressWarnings("FieldCanBeLocal")
  private static final int MAX_ADS_TO_SERVE = 2;

  private Server server;
  private HealthStatusManager healthMgr;

  private static final AdService service = new AdService();

  private void start() throws IOException {

    PyroscopeAgent.start(
            new Config.Builder()
                    .setApplicationName("adservice.oteldemo")
                    .setProfilingEvent(EventType.ITIMER)
                    .setFormat(Format.JFR)
                    .setServerAddress(System.getenv("PYROSCOPE_SERVER_ADDRESS"))
                    // Optionally, if authentication is enabled, specify the API key.
                    // .setAuthToken(System.getenv("PYROSCOPE_AUTH_TOKEN"))
                    .build()
    );

    PyroscopeOtelConfiguration pyroscopeTelemetryConfig = new PyroscopeOtelConfiguration.Builder()
            .setAppName("adservice.oteldemo." + EventType.ITIMER.id)
            .setPyroscopeEndpoint(System.getenv("PYROSCOPE_SERVER_ADDRESS"))
            .setAddProfileURL(true)
            .setAddSpanName(true)
            .setRootSpanOnly(true)
            .setAddProfileBaselineURLs(true)
            .build();



    int port = Integer.parseInt(Optional.ofNullable(System.getenv("AD_SERVICE_PORT")).orElseThrow(
        () -> new IOException(
            "environment vars: AD_SERVICE_PORT must not be null")
    ));
    healthMgr = new HealthStatusManager();

    server =
        ServerBuilder.forPort(port)
            .addService(new AdServiceImpl())
            .addService(healthMgr.getHealthService())
            .build()
            .start();
    logger.info("Ad Service started, listening on " + port);
    Runtime.getRuntime()
        .addShutdownHook(
            new Thread(
                () -> {
                  // Use stderr here since the logger may have been reset by its JVM shutdown hook.
                  System.err.println(
                      "*** shutting down gRPC ads server since JVM is shutting down");
                  AdService.this.stop();
                  System.err.println("*** server shut down");
                }));
    healthMgr.setStatus("", ServingStatus.SERVING);
  }

  private void stop() {
    if (server != null) {
      healthMgr.clearStatus("");
      server.shutdown();
    }
  }

  private static class AdServiceImpl extends hipstershop.AdServiceGrpc.AdServiceImplBase {

    /**
     * Retrieves ads based on context provided in the request {@code AdRequest}.
     *
     * @param req the request containing context.
     * @param responseObserver the stream observer which gets notified with the value of {@code
     *     AdResponse}
     */
    @Override
    public void getAds(AdRequest req, StreamObserver<AdResponse> responseObserver) {
      AdService service = AdService.getInstance();

      // get the current span in context
      Span span = Span.current();
      try {
        List<Ad> allAds = new ArrayList<>();

        span.setAttribute("app.ads.contextKeys", req.getContextKeysList().toString());
        span.setAttribute("app.ads.contextKeys.count", req.getContextKeysCount());
        logger.info("received ad request (context_words=" + req.getContextKeysList() + ")");
        if (req.getContextKeysCount() > 0) {
          for (int i = 0; i < req.getContextKeysCount(); i++) {
            Collection<Ad> ads = service.getAdsByCategory(req.getContextKeys(i));
            allAds.addAll(ads);
          }
        } else {
          allAds = service.getRandomAds();
        }
        if (allAds.isEmpty()) {
          // Serve random ads.
          allAds = service.getRandomAds();
        }
        span.setAttribute("app.ads.count", allAds.size());
        AdResponse reply = AdResponse.newBuilder().addAllAds(allAds).build();
        responseObserver.onNext(reply);
        responseObserver.onCompleted();
      } catch (StatusRuntimeException e) {
        span.addEvent(
            "Error", Attributes.of(AttributeKey.stringKey("exception.message"), e.getMessage()));
        span.setStatus(StatusCode.ERROR);
        logger.log(Level.WARN, "GetAds Failed with status {}", e.getStatus());
        responseObserver.onError(e);
      }
    }
  }

  private static final ImmutableListMultimap<String, Ad> adsMap = createAdsMap();

  @WithSpan("getAdsByCategory")
  private Collection<Ad> getAdsByCategory(@SpanAttribute("app.ads.category") String category) {
    Collection<Ad> ads = adsMap.get(category);
    Span.current().setAttribute("app.ads.count", ads.size());
    return ads;
  }

  private static final Random random = new Random();

  private List<Ad> getRandomAds() {

    List<Ad> ads = new ArrayList<>(MAX_ADS_TO_SERVE);

    // create and start a new span manually
    Tracer tracer = GlobalOpenTelemetry.getTracer("adservice");
    Span span = tracer.spanBuilder("getRandomAds").startSpan();

    // put the span into context, so if any child span is started the parent will be set properly
    try (Scope ignored = span.makeCurrent()) {

      Collection<Ad> allAds = adsMap.values();
      for (int i = 0; i < MAX_ADS_TO_SERVE; i++) {
        ads.add(Iterables.get(allAds, random.nextInt(allAds.size())));
      }
      span.setAttribute("app.ads.count", ads.size());

    } finally {
      span.end();
    }

    return ads;
  }

  private static AdService getInstance() {
    return service;
  }

  /** Await termination on the main thread since the grpc library uses daemon threads. */
  private void blockUntilShutdown() throws InterruptedException {
    if (server != null) {
      server.awaitTermination();
    }
  }

  private static ImmutableListMultimap<String, Ad> createAdsMap() {
    Ad binoculars =
        Ad.newBuilder()
            .setRedirectUrl("/product/2ZYFJ3GM2N")
            .setText("Roof Binoculars for sale. 50% off.")
            .build();
    Ad explorerTelescope =
        Ad.newBuilder()
            .setRedirectUrl("/product/66VCHSJNUP")
            .setText("Starsense Explorer Refractor Telescope for sale. 20% off.")
            .build();
    Ad colorImager =
        Ad.newBuilder()
            .setRedirectUrl("/product/0PUK6V6EV0")
            .setText("Solar System Color Imager for sale. 30% off.")
            .build();
    Ad opticalTube =
        Ad.newBuilder()
            .setRedirectUrl("/product/9SIQT8TOJO")
            .setText("Optical Tube Assembly for sale. 10% off.")
            .build();
    Ad travelTelescope =
        Ad.newBuilder()
            .setRedirectUrl("/product/1YMWWN1N4O")
            .setText(
                "Eclipsmart Travel Refractor Telescope for sale. Buy one, get second kit for free")
            .build();
    Ad solarFilter =
        Ad.newBuilder()
            .setRedirectUrl("/product/6E92ZMYYFZ")
            .setText("Solar Filter for sale. Buy two, get third one for free")
            .build();
    Ad cleaningKit =
        Ad.newBuilder()
            .setRedirectUrl("/product/L9ECAV7KIM")
            .setText("Lens Cleaning Kit for sale. Buy one, get second one for free")
            .build();
    return ImmutableListMultimap.<String, Ad>builder()
        .putAll("binoculars", binoculars)
        .putAll("telescopes", explorerTelescope)
        .putAll("accessories", colorImager, solarFilter, cleaningKit)
        .putAll("assembly", opticalTube)
        .putAll("travel", travelTelescope)
        .build();
  }

  /** Main launches the server from the command line. */
  public static void main(String[] args) throws IOException, InterruptedException {
    // Start the RPC server. You shouldn't see any output from gRPC before this.
    logger.info("AdService starting.");
    final AdService service = AdService.getInstance();
    service.start();
    service.blockUntilShutdown();
  }
}
