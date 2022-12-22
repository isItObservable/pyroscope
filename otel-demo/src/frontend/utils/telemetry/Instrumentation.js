const opentelemetry = require("@opentelemetry/sdk-node")
const { getNodeAutoInstrumentations } = require("@opentelemetry/auto-instrumentations-node")
const { OTLPTraceExporter } =  require('@opentelemetry/exporter-trace-otlp-grpc')
const Pyroscope = require('@pyroscope/nodejs');

const sdk = new opentelemetry.NodeSDK({
  traceExporter: new OTLPTraceExporter(),
  instrumentations: [ getNodeAutoInstrumentations() ]
})

Pyroscope.init({
  serverAddress: process.env.PYROSCOPE_URL,
  appName: 'FrontendService'
});

Pyroscope.start()
Pyroscope.startCpuProfiling();
Pyroscope.startHeapProfiling();

sdk.start()
