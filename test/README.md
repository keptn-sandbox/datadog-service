### Running the tests from your local machine
```bash
kubectl port-forward svc/api-gateway-nginx 5000:80 -nkeptn &
export ENABLE_E2E_TEST=true
export KEPTN_ENDPOINT=http://localhost:5000/api
export KEPTN_API_TOKEN=$(kubectl get secret keptn-api-token -n keptn -ojsonpath='{.data.keptn-api-token}' | base64 -d)
gotestsum --format standard-verbose -- -timeout=120m  ./test/e2e/...
ps aux | grep 'kubectl port-forward svc/api-gateway-nginx 5000' | grep -v 'grep' | awk '{print $2}' | xargs -I{} kill -9 {}
```
### Creating everything manually for debugging
#### While running a local cluster
password=$(date +%s | sha256sum | base64 | head -c 32)
export GITEA_ADMIN_PASSWORD=$password
export GITEA_ADMIN_USERNAME=GiteaAdmin
export GITEA_NAMESPACE=gitea
export GITEA_ENDPOINT="http://gitea-http.${GITEA_NAMESPACE}:3000"
helm repo add gitea-charts https://dl.gitea.io/charts/
helm repo update
helm install -n ${GITEA_NAMESPACE} gitea gitea-charts/gitea \
--create-namespace \
--set memcached.enabled=false \
--set postgresql.enabled=false \
--set gitea.config.database.DB_TYPE=sqlite3 \
--set gitea.admin.username=${GITEA_ADMIN_USERNAME} \
--set gitea.admin.password=${GITEA_ADMIN_PASSWORD} \
--set gitea.config.server.OFFLINE_MODE=true \
--set gitea.config.server.ROOT_URL=${GITEA_ENDPOINT}/ \
--wait 
helm install keptn-gitea-provisioner-service https://github.com/keptn-sandbox/keptn-gitea-provisioner-service/releases/download/0.1.0/keptn-gitea-provisioner-service-0.1.0.tgz \
--set gitea.endpoint=${GITEA_ENDPOINT} \
--set gitea.admin.create=true \
--set gitea.admin.username=${GITEA_ADMIN_USERNAME} \
--set gitea.admin.password=${GITEA_ADMIN_PASSWORD} \
--wait -ndefault

#### After cluster is created and Keptn has been installed
keptn create project e2e-project --shipyard=shipyard/podtatohead.deployment.yaml

keptn create service podtatoserver --project=e2e-project

keptn add-resource --project="e2e-project" --stage="staging" --service="podtatoserver" --resource=data/podtatohead.sli.yaml --resourceUri=datadog/sli.yaml

keptn add-resource --project=e2e-project --service=podtatoserver --all-stages --resource=data/podtatoserver-0.1.0.tgz --resourceUri=charts/podtatoserver.tgz

keptn add-resource --project="e2e-project" --stage="staging" --service="podtatoserver" --resource=data/podtatohead.slo.yaml --resourceUri=slo.yaml

keptn configure monitoring datadog --project=e2e-project --service=podtatoserver 

keptn add-resource --project=e2e-project --service=podtatoserver --stage="staging" --resource=data/podtatohead.jes-config.yaml --resourceUri job/config.yaml

keptn send event -f events/podtatohead.deploy-v0.1.1.triggered.json

#### To install job executor service
JES_NAMESPACE=keptn-jes
KEPTN_API_PROTOCOL=http
KEPTN_API_TOKEN=$(kubectl get secret keptn-api-token -n keptn -ojsonpath='{.data.keptn-api-token}' | base64 -d)
export KEPTN_ENDPOINT=api-gateway-nginx.keptn
TASK_SUBSCRIPTION="sh.keptn.event.deployment.triggered\\,sh.keptn.event.test.triggered"
JES_VERSION="0.3.0"
helm upgrade --install \
--create-namespace -n ${JES_NAMESPACE} \
job-executor-service \
"https://github.com/keptn-contrib/job-executor-service/releases/download/${JES_VERSION}/job-executor-service-${JES_VERSION}.tgz" \
--set remoteControlPlane.autoDetect.enabled="false" \
--set remoteControlPlane.topicSubscription=${TASK_SUBSCRIPTION} \
--set remoteControlPlane.api.token=${KEPTN_API_TOKEN} \
--set remoteControlPlane.api.hostname=${KEPTN_ENDPOINT} \
--set remoteControlPlane.api.protocol=${KEPTN_API_PROTOCOL} \
--wait

kubectl apply \
    -f data/helm-serviceAccount.yaml \
    -f data/helm-clusterRole.yaml \
    -f data/helm-clusterRoleBinding.yaml
