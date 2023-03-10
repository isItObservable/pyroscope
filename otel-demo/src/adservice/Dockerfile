# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM eclipse-temurin:17-jdk AS builder

WORKDIR /usr/src/app/

COPY ./src/adservice/ ./
COPY ./pb/ ./proto
RUN ./gradlew downloadRepos
RUN ./gradlew installDist -PprotoSourceDir=./proto

# -----------------------------------------------------------------------------

FROM eclipse-temurin:17-jdk

ARG version=1.19.1
WORKDIR /usr/src/app/


COPY --from=builder /usr/src/app/ ./
ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/download/v$version/opentelemetry-javaagent.jar /app/opentelemetry-javaagent.jar
ADD https://repo1.maven.org/maven2/io/pyroscope/otel/0.10.1.4/otel-0.10.1.4.jar /app/pyroscope-otel.jar
RUN chmod 644 /app/opentelemetry-javaagent.jar
RUN chmod 644 /app/pyroscope-otel.jar
ENV OTEL_PYROSCOPE_ADD_PROFILE_URL=true
ENV OTEL_PYROSCOPE_ADD_PROFILE_BASELINE_URL=true
ENV OTEL_PYROSCOPE_START_PROFILING=true
ENV OTEL_TRACES_EXPORTER=otlp
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
ENV PYROSCOPE_FORMAT=jfr
ENV PYROSCOPE_PROFILER_EVENT=itimer
ENV PYROSCOPE_APPLICATION_NAME=adservice.oteldemo
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope.pyroscope.svc:4040
ENV PYROSCOPE_LABELS="region=eu-north"
ENV OTEL_JAVAAGENT_EXTENSIONS=/app/pyroscope-otel.jar
ENV JAVA_TOOL_OPTIONS=-javaagent:/app/opentelemetry-javaagent.jar
ENV OTEL_PYROSCOPE_ENDPOINT=http://pyroscope.pyroscope.svc:4040
EXPOSE ${AD_SERVICE_PORT}
ENTRYPOINT [ "./build/install/hipstershop/bin/AdService" ]
