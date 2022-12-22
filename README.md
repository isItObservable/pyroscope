# Is it Observable
<p align="center"><img src="/image/logo.png" width="40%" alt="Is It observable Logo" /></p>

## Episode : What is Continuous Profiling and What is Pyroscope
This repository contains the files utilized during the tutorial presented in the dedicated IsItObservable episode related to Pyroscope.
<p align="center"><img src="/image/pyroscope.png" width="40%" alt="pyroscope Logo" /></p>

What you will learn
* How to use the [Pyroscope](https://pyroscope.io/)

This repository showcase the usage of Pyroscope  with :
* The Otel-demo
* The OpenTelemetry Operator
* Nginx ingress controller
* Dynatrace

We will send the Telemetry data produced by the Otel-demo application Dynatrace.

## Prerequisite
The following tools need to be install on your machine :
- jq
- kubectl
- git
- gcloud ( if you are using GKE)
- Helm


## Deployment Steps in GCP

You will first need a Kubernetes cluster with 2 Nodes.
You can either deploy on Minikube or K3s or follow the instructions to create GKE cluster:
### 1.Create a Google Cloud Platform Project
```shell
PROJECT_ID="<your-project-id>"
gcloud services enable container.googleapis.com --project ${PROJECT_ID}
gcloud services enable monitoring.googleapis.com \
    cloudtrace.googleapis.com \
    clouddebugger.googleapis.com \
    cloudprofiler.googleapis.com \
    --project ${PROJECT_ID}
```
### 2.Create a GKE cluster
```shell
ZONE=europe-west3-a
NAME=isitobservable-bindplane
gcloud container clusters create "${NAME}" \
 --zone ${ZONE} --machine-type=e2-standard-2 --num-nodes=4
```


## Getting started
### Dynatrace Tenant
#### 1. Dynatrace Tenant - start a trial
If you don't have any Dyntrace tenant , then i suggest to create a trial using the following link : [Dynatrace Trial](https://bit.ly/3KxWDvY)
Once you have your Tenant save the Dynatrace (including https) tenant URL in the variable `DT_TENANT_URL` (for example : https://dedededfrf.live.dynatrace.com)
```
DT_TENANT_URL=<YOUR TENANT URL>
```


#### 2. Create the Dynatrace API Tokens
Create a Dynatrace token with the following scope ( left menu Acces Token):
* ingest metrics
* ingest OpenTelemetry traces
<p align="center"><img src="/image/data_ingest.png" width="40%" alt="data token" /></p>
Save the value of the token . We will use it later to store in a k8S secret

```
DATA_INGEST_TOKEN=<YOUR TOKEN VALUE>
```
### 3.Clone the Github Repository
```shell
https://github.com/isItObservable/pyroscope
cd pyroscope
```
### 4.Deploy most of the components
The application will deploy the otel demo v1.0.0
```shell
chmod 777 deployment.sh
./deployment.sh  --clustername "${NAME}" --dturl "${DT_TENANT_URL}" --dttoken "${DATA_INGEST_TOKEN}"
```

### 5.Look at the OtelDemo code 

#### a. Golang
Look at the followinf file: `otel-demo/src/productcatalogservice/main.go`
```shell
cat otel-demo/src/productcatalogservice/main.go
```
In this file we can see 2 importants part :
- Declaration of the agent:
```gotemplate
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
```
- add the OpenTelemetry integration
```gotemplate
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
```
#### b. Rust
Look at the followinf file: `otel-demo/src/emailservice/email_server.rb`
```shell
cat otel-demo/src/emailservice/email_server.rb
```
In this file we can see 2 importants part :
- Declaration of the agent:
```rust
Pyroscope.configure do |config|
  config.application_name = "emailservice" # replace this with some name for your application
  config.server_address   = pyroscope_url # replace this with the address of your pyroscope server
end
```
- add the OpenTelemetry integration
```rust
OpenTelemetry::SDK.configure do |c|
    c.add_span_processor Pyroscope::Otel::SpanProcessor.new(
        "emailservice.cpu", # your app name with ".cpu" suffix, for example rideshare-ruby.cpu
        pyroscope_url # link to your pyroscope server, for example "http://localhost:4040"
    )
    c.use "OpenTelemetry::Instrumentation::Sinatra"
end
```
#### c. Nodejs
Look at the followinf file: `otel-demo/src/paymentservice/opentelemetry.js`
```shell
cat otel-demo/src/paymentservice/opentelemetry.js
```
- Declaration of the agent:
```js
Pyroscope.init({
  serverAddress: process.env.PYROSCOPE_URL,
  appName: 'PaymentService'
});
Pyroscope.start()
```
#### d.Java
In the case of Java we are attaching the pyroscope to the openTelemtry instrumentation library
```shell
cat otel-demo/src/adservice/Dockerfile
```
#### e.Dotnet

In the case of dotnet we are attaching the pyroscope agent at the start of the dotnet application
```shell
cat otel-demo/src/cartservice/src/Dockerfile
```
#### f.Python
Look at the followinf file: `otel-demo/src/recommendationservice/recommendation_server.py`
```shell
cat otel-demo/src/recommendationservice/recommendation_server.py
```
- Declaration of the agent:
```python
pyroscope.configure(
      application_name    = "recommendationservice", # replace this with some name for your application
      server_address      = os.getenv("PYROSCOPE_URL"), # replace this with the address of your pyroscope server
      detect_subprocesses = True, # detect subprocesses started by the main process; default is False
      oncpu               = True, # report cpu time only; default is True
      gil_only            = True, # only include traces for threads that are holding on to the Global Interpreter Lock; default is True
)
```
### 6.Open Pyroscope and look at the profiling data
Open Pyroscope `http://pyroscope.$IP.nip.io` and look at various profile exposed on the various applications.

### 7.Enable the Pulling mode
The service profiled in go has the pulling mode enabled.
Therefore we can collect more information by updating the configuration of Pyrscope and adding a `scrape-configs`

THe configuration of the pyroscope define where to collect the
```yaml
scrape-configs:
  - job-name: pyroscope
    enabled-profiles: [cpu, mem, goroutines, mutex, block]

    static-configs:
      - application: productcatalogservice.otel-demo
        spy-name: gospy
        targets:
          - example-productcatalogservice.otel-demo.svc:6060

      - application: checkoutService.otel-demo
        spy-name: gospy
        targets:
          - example-checkoutservice.otel-demo.svc:6060
```
Let's apply the new configuration and restart the pyroscope server:
```shell
kubectl apply -f pyrosccope/pyroscope_cm.yaml -n pyroscope
kubectl rollout restart deployment pyroscope -n pyroscope
```
Let's open Pyroscope and look at the new metrics collected .
