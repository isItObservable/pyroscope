#!/usr/bin/env bash

################################################################################
### Script deploying the Observ-K8s environment
### Parameters:
### Clustern name: name of your k8s cluster
### dttoken: Dynatrace api token with ingest metrics and otlp ingest scope
### dturl : url of your DT tenant wihtout any / at the end for example: https://dedede.live.dynatrace.com
################################################################################


### Pre-flight checks for dependencies
if ! command -v jq >/dev/null 2>&1; then
    echo "Please install jq before continuing"
    exit 1
fi

if ! command -v git >/dev/null 2>&1; then
    echo "Please install git before continuing"
    exit 1
fi


if ! command -v helm >/dev/null 2>&1; then
    echo "Please install helm before continuing"
    exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
    echo "Please install kubectl before continuing"
    exit 1
fi
echo "parsing arguments"
while [ $# -gt 0 ]; do
  case "$1" in
  --dttoken)
    DTTOKEN="$2"
   shift 2
    ;;
  --dturl)
    DTURL="$2"
   shift 2
    ;;
  --clustername)
    CLUSTERNAME="$2"
   shift 2
    ;;
  *)
    echo "Warning: skipping unsupported option: $1"
    shift
    ;;
  esac
done
echo "Checking arguments"
if [ -z "$CLUSTERNAME" ]; then
  echo "Error: clustername not set!"
  exit 1
fi
if [ -z "$DTURL" ]; then
  echo "Error: environment-url not set!"
  exit 1
fi

if [ -z "$DTTOKEN" ]; then
  echo "Error: api-token not set!"
  exit 1
fi



helm upgrade --install ingress-nginx ingress-nginx  --repo https://kubernetes.github.io/ingress-nginx  --namespace ingress-nginx --create-namespace

### get the ip adress of ingress ####
IP=""
while [ -z $IP ]; do
  echo "Waiting for external IP"
  IP=$(kubectl get svc ingress-nginx-controller -n ingress-nginx -ojson | jq -j '.status.loadBalancer.ingress[].ip')
  [ -z "$IP" ] && sleep 10
done
echo 'Found external IP: '$IP

### Update the ip of the ip adress for the ingres
#TODO to update this part to use the dns entry /ELB/ALB
sed -i "s,IP_TO_REPLACE,$IP," kubernetes-manifests/K8sdemo.yaml
sed -i "s,IP_TO_REPLACE,$IP," pyrosccope/ingress.yaml
### Depploy Prometheus

#### Deploy the cert-manager
echo "Deploying Cert Manager ( for OpenTelemetry Operator)"
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.6.1/cert-manager.yaml
# Wait for pod webhook started
kubectl wait pod -l app.kubernetes.io/component=webhook -n cert-manager --for=condition=Ready --timeout=2m
# Deploy the opentelemetry operator
echo "Deploying the OpenTelemetry Operator"
kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml


CLUSTERID=$(kubectl get namespace kube-system -o jsonpath='{.metadata.uid}')
sed -i "s,CLUSTER_ID_TOREPLACE,$CLUSTERID," kubernetes-manifests/openTelemetry-sidecar.yaml
sed -i "s,CLUSTER_NAME_TO_REPLACE,$CLUSTERNAME," kubernetes-manifests/openTelemetry-sidecar.yaml



#Deploy the OpenTelemetry Collector
echo "Deploying Otel Collector"
kubectl apply -f kubernetes-manifests/rbac.yaml
##update the collector pipeline
sed -i "s,DT_TOKEN_TO_REPLACE,$DTTOKEN," kubernetes-manifests/openTelemetry-manifest.yaml
sed -i "s,DT_URL_TO_REPLACE,$DTURL," kubernetes-manifests/openTelemetry-manifest.yaml
##Deploy the Collector DaemonSet
kubectl apply -f kubernetes-manifests/openTelemetry-manifest.yaml

## deploying Periscope
echo "Deploying Pyroscope"
helm repo add pyroscope-io https://pyroscope-io.github.io/helm-chart
helm install pyroscope pyroscope-io/pyroscope  -n pyroscope --create-namespace --set ingress.enabled=true --set ingress.hosts=[{"host":"pyroscope.$IP.nip.io","paths":[{"path":"/","pathType":"Prefix"}]}] --set rbac.create=true
## deploying Periscope ebpf
echo "Deploying Pyroscope ebpf"
helm upgrade -i pyroscope-ebpf pyroscope-io/pyroscope-ebpf --set application-name="isitobservable.ebpf" --set server-address="http://pyroscope.pyroscope.svc:4040"
## deploying otel-demo application
kubectl create ns otel-demo
kubectl apply -f kubernetes-manifests/K8sdemo.yaml -n otel-demo

# Echo environ*
echo "--------------Demo--------------------"
echo "url of the demo: "
echo "Otel demo url: http://otel-demo.$IP.nip.io"
echo "--------------Pyroscope--------------------"
echo "pyroscope url: http://pyroscope.$IP.nip.io"
echo "========================================================"


