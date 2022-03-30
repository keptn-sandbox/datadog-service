#!/bin/bash

# set -e

# ========
# Keptn Up
# ========
# - Tested on Ubuntu 20.04
# - This script primarily focuses on Linux (untested but should work with Mac and other versions of Linux)
# Pre-requisites:
# 1. helm v3
#    $ helm version
#    version.BuildInfo{Version:"v3.7.0", GitCommit:"eeac83883cb4014fe60267ec6373570374ce770b", 
#    GitTreeState:"clean", GoVersion:"go1.16.8"}
# 2. kubectl
#    $ kubectl version
#    Client Version: version.Info{Major:"1", Minor:"23", GitVersion:"v1.23.1", GitCommit:"86ec240af8cbd1b60bcc4c03c20da9b98005b92e", GitTreeState:"clean", BuildDate:"2021-12-16T11:41:01Z", GoVersion:"go1.17.5", Compiler:"gc", Platform:"linux/amd64"}
# 3. istioctl
#    $ istioctl version
#    ...
#    1.11.2
# 4. keptn
#    $ keptn version
#    Keptn CLI and Keptn cluster version are already on the latest version ( 0.11.4 )! 
#    * Your Keptn CLI version: 0.11.4
#    ...
# 5. minikube
#    $ minikube version
#    minikube version: v1.24.0
#    commit: 76b94fb3c4e8ac5062daf70d60cf03ddcc0a741b
# 6. virtualbox
#    $ virtualbox --help
#    Oracle VM VirtualBox VM Selector v6.1.32
#    (C) 2005-2022 Oracle Corporation
#    All rights reserved.
#    ...

# Helper functions 
# ---------------------------------------------- #
function print_headline() {
  HEADLINE=$1
  
  echo ""
  echo "---------------------------------------------------------------------"
  echo $HEADLINE
  echo "---------------------------------------------------------------------"
  echo ""
}

function check_if_helm_cli_is_installed(){
 if ! command -v helm &> /dev/null
 then
    echo "Could not find helm. Please install helm to proceed further."
    exit
 fi  
}

function check_if_keptn_cli_is_installed(){
 if ! command -v keptn &> /dev/null
 then
    echo "Could not find keptn. Please install keptn to proceed further."
    exit
 fi  
}

function check_if_kubectl_cli_is_installed(){
 if ! command -v kubectl &> /dev/null
 then
    echo "Could not find kubectl. Please install kubectl to proceed further."
    exit
 fi  
}

function check_if_istioctl_cli_is_installed(){
 if ! command -v istioctl &> /dev/null
 then
    echo "Could not find istioctl. Please install istioctl to proceed further."
    exit
 fi  
}


function print_error() {
  echo "::error file=${BASH_SOURCE[1]##*/},line=${BASH_LINENO[0]}::$(timestamp) ${*}"
}

function verify_test_step() {
  if [[ $1 != '0' ]]; then
    print_error "$2"
    print_error "Keptn test step failed"
    exit 1
  fi
}

function verify_namespace_exists() {
  NAMESPACE=$1;

  NAMESPACE_LIST=$(eval "kubectl get namespaces -L istio-injection | grep ${NAMESPACE} | awk '/$NAMESPACE/'" | awk '{print $1}')

  if [[ -z "$NAMESPACE_LIST" ]]; then
    print_error "Could not find namespace ${NAMESPACE}"
    exit 2
  else
    echo "Found namespace ${NAMESPACE}"
  fi
}

# wait for a deployment to be up and running
function wait_for_deployment_in_namespace() {
  DEPLOYMENT=$1; NAMESPACE=$2;
  RETRY=0; RETRY_MAX=10;

  while [[ $RETRY -lt $RETRY_MAX ]]; do
    DEPLOYMENT_LIST=$(eval "kubectl get deployments -n ${NAMESPACE} | awk '/$DEPLOYMENT /'" | awk '{print $1}') # list of multiple deployments when starting with the same name
    if [[ -z "$DEPLOYMENT_LIST" ]]; then
      RETRY=$((RETRY+1))
      echo "Retry: ${RETRY}/${RETRY_MAX} - Deployment not found - waiting 15s for deployment ${DEPLOYMENT} in namespace ${NAMESPACE}"
      sleep 15
    else
      READY_REPLICAS=$(eval kubectl get deployments "$DEPLOYMENT" -n "$NAMESPACE" -o=jsonpath='{$.status.availableReplicas}')
      WANTED_REPLICAS=$(eval kubectl get deployments "$DEPLOYMENT"  -n "$NAMESPACE" -o=jsonpath='{$.spec.replicas}')
      UNAVAILABLE_REPLICAS=$(eval kubectl get deployments "$DEPLOYMENT"  -n "$NAMESPACE" -o=jsonpath='{$.status.unavailableReplicas}')
      if [[ "$READY_REPLICAS" = "$WANTED_REPLICAS" && "$UNAVAILABLE_REPLICAS" = "" ]]; then
        echo "Found deployment ${DEPLOYMENT} in namespace ${NAMESPACE}: ${DEPLOYMENT_LIST}"
        break
      else
          RETRY=$((RETRY+1))
          echo "Retry: ${RETRY}/${RETRY_MAX} - Unsufficient replicas for deployment - waiting 15s for deployment ${DEPLOYMENT} in namespace ${NAMESPACE}"
          sleep 15
      fi
    fi
  done

  if [[ $RETRY == "$RETRY_MAX" ]]; then
    print_error "Could not find deployment ${DEPLOYMENT} in namespace ${NAMESPACE}"
    exit 1
  fi
}
# ---------------------------------------------- #

# Actual script starts
# ---------------------------------------------- #

# This block of code
# 1. Creates a local k8s cluster
# 2. Installs keptn
# 3. Installs istio
# 4. Starts port-forwarding the Keptn API
# 5. Authenticates with Keptn API
INGRESS_HOST=localhost
INGRESS_PORT=5000

# Datadog does not work with the docker driver
minikube start --cpus='4' --memory='10g' --driver=virtualbox

check_if_keptn_cli_is_installed
# TODO: This might work without any `--use-case` flag 
# i.e., only control plane with quality gates 
# but needs to be verified
keptn install --use-case=continuous-delivery -y

# check_if_istioctl_cli_is_installed
# # Install Istio
# istioctl install -y

check_if_kubectl_cli_is_installed
# Kill an existing port-forward if it exists 
ps aux | grep 'kubectl port-forward svc/api-gateway-nginx 5000' | grep -v 'grep' | awk '{print $2}' | xargs -I{} kill -9 {}
echo "Port-forwarding Keptn API"
wait_for_deployment_in_namespace "api-gateway-nginx" "keptn"

kubectl port-forward svc/api-gateway-nginx 5000:80 -nkeptn &
# This is to wait sometime until port-forward is stable
# it throws `connection refused`` error sometimes
# when we try to authenticate
sleep 10
keptn auth --endpoint=localhost:5000

kubectl config set-context --current --namespace=keptn

# Keptn does not ask for username/password for authentication
# in the web UI if we delete the bridge secret
# If you want to keep the authentication, use
# Username: keptn
# Password: Get it by running 'kubectl get secret bridge-credentials -o=jsonpath='{.data.BASIC_AUTH_PASSWORD}' -nkeptn | base64 -d'
kubectl delete secret bridge -nkeptn

# ---------------------------------------------- #

# This block of code
# 1. Creates a Keptn project
# 2. Creates a Keptn service (with helm charts as resources)
# 3. Adds loadtests
# 4. Adds endpoints
PROJECT="podtatohead"
SERVICE="helloservice"
IMAGE="docker.io/jetzlstorfer/helloserver"
VERSION=0.1.1
SLOW_VERSION=0.1.2

check_if_helm_cli_is_installed
cd ./examples

print_headline "Create a Keptn project"
echo "keptn create project $PROJECT --shipyard=./quickstart/shipyard.yaml"
keptn create project $PROJECT --shipyard=./quickstart/shipyard.yaml
verify_test_step $? "keptn create project command failed."

print_headline "Create a Keptn service"
echo "keptn create service $SERVICE --project=${PROJECT} "
keptn create service $SERVICE --project="${PROJECT}" 

print_headline "Add Helm chart for $SERVICE"
keptn add-resource --project=$PROJECT --service=$SERVICE --all-stages --resource=./quickstart/helm/helloservice.tgz --resourceUri=helm/helloservice.tgz

print_headline "Add endpoints file"
keptn add-resource --project=$PROJECT --service=$SERVICE --stage=hardening --resource=./quickstart/helm/hardening_endpoints.yaml --resourceUri=helm/endpoints.yaml
keptn add-resource --project=$PROJECT --service=$SERVICE --stage=production --resource=./quickstart/helm/production_endpoints.yaml --resourceUri=helm/endpoints.yaml

# adding tests to the service
print_headline "Adding some load tests"
keptn add-resource --project=$PROJECT --service=$SERVICE --stage=hardening --resource=./quickstart/jmeter/jmeter.conf.yaml --resourceUri=jmeter/jmeter.conf.yaml
keptn add-resource --project=$PROJECT --service=$SERVICE --stage=hardening --resource=./quickstart/jmeter/load.jmx --resourceUri=jmeter/load.jmx

# to tell lighthouse to use Datadog if the project is podtatohead
kubectl apply -f ./quickstart/lighthouse_config.yaml

# ---------------------------------------------- #

# This block of code
# 1. Installs Datadog
# 2. Sets it up with the API keys in a K8s Secret
# 3. Installs datadog integration for Keptn

helm repo add datadog https://helm.datadoghq.com

# Install datadog
# Uncomment this line if you want to install the Datadog operator
# helm install my-datadog-operator datadog/datadog-operator
# kubectl apply -f ~/sandbox/snippets/ddagent.yaml
# kubectl apply -f ~/sandbox/snippets/ddmonitor.yaml 
# install datadog api secret
# kubectl create secret generic datadog-secret --from-literal api-key=${DD_API_KEY} --from-literal app-key=${DD_APP_KEY}

# # Install datadog using the Datadog helm chart
# helm install datadog --set datadog.apiKey=${DD_API_KEY} datadog/datadog --set datadog.appKey=${DD_APP_KEY} --set datadog.site=${DD_SITE} --set clusterAgent.enabled=true --set clusterAgent.metricsProvider.enabled=true --set clusterAgent.createPodDisruptionBudget=true --set clusterAgent.replicas=2

helm install datadog --set datadog.apiKey=${DD_API_KEY} datadog/datadog --set datadog.appKey=${DD_APP_KEY} --set datadog.site=${DD_SITE} --set clusterAgent.enabled=true --set clusterAgent.metricsProvider.enabled=true --set clusterAgent.createPodDisruptionBudget=true --set clusterAgent.replicas=2


# Install datadog-service integration for Keptn
# kubectl apply -f ~/sandbox/datadog-service/deploy/service.yaml
helm install datadog-service ./helm --set datadogservice.ddApikey=${DD_API_KEY} --set datadogservice.ddAppKey=${DD_APP_KEY} --set datadogservice.ddSite=${DD_SITE}

# Add datadog sli and slo
keptn add-resource --project="podtatohead" --stage=hardening --service=helloservice --resource=./quickstart/sli.yaml --resourceUri=datadog/sli.yaml
keptn add-resource --project="podtatohead" --stage=hardening --service="helloservice" --resource=./quickstart/slo.yaml --resourceUri=slo.yaml


# ---------------------------------------------- #

# This block of code triggers delivery sequence for the service

print_headline "Trigger the delivery sequence with Keptn"
echo "keptn trigger delivery --project=$PROJECT --service=$SERVICE --image=$IMAGE --tag=$VERSION"
keptn trigger delivery --project=$PROJECT --service=$SERVICE --image=$IMAGE --tag=$VERSION
verify_test_step $? "Trigger delivery for helloservice failed"

print_headline "Trigger a new delivery sequence with Keptn"
echo "keptn trigger delivery --project=$PROJECT --service=$SERVICE --image=$IMAGE --tag=$SLOW_VERSION"
keptn trigger delivery --project=$PROJECT --service=$SERVICE --image=$IMAGE --tag=$SLOW_VERSION
verify_test_step $? "Trigger delivery for helloservice failed"


echo "Following the multi stage delivery in Keptn Bridge here: http://$INGRESS_HOST:$INGRESS_PORT/bridge/project/$PROJECT/sequence"


print_headline "Have a look at the Keptn Bridge and explore the demo project"
echo "You can run a new delivery sequence with the following command"
echo "keptn trigger delivery --project=$PROJECT --service=$SERVICE --image=$IMAGE --tag=$VERSION"

print_headline "Multi-stage delviery demo with SLO-based quality gates for Datadog Keptn integration has been successfully set up"

echo "You can run a new delivery sequence with the following command"
echo "keptn trigger delivery --project=$PROJECT --service=$SERVICE --image=$IMAGE --tag=$VERSION"
echo "or by deploying a slow version that will not pass the quality gate"
echo "keptn trigger delivery --project=$PROJECT --service=$SERVICE --image=$IMAGE --tag=$SLOW_VERSION"

# ---------------------------------------------- #

# Cleanup

# Kill the port-forward started in the background
# If you want to port-forward again, just run
# kubectl port-forward svc/api-gateway-nginx 5000:80 -nkeptn
ps aux | grep 'kubectl port-forward svc/api-gateway-nginx 5000' | grep -v 'grep' | awk '{print $2}' | xargs -I{} kill -9 {}