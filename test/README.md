### Creating everything manually for debugging
keptn create project e2e-project --shipyard=shipyard/podtatohead.deployment.yaml

keptn create service podtatoserver --project=e2e-project

keptn add-resource --project="e2e-project" --stage="staging" --service="podtatoserver" --resource=data/podtatohead.sli.yaml --resourceUri=datadog/sli.yaml

keptn add-resource --project=e2e-project --service=podtatoserver --all-stages --resource=data/podtatoserver-0.1.0.tgz --resourceUri=helm/podtatoserver.tgz

keptn add-resource --project="e2e-project" --stage="staging" --service="podtatoserver" --resource=data/podtatohead.slo.yaml --resourceUri=slo.yaml

keptn send event -f events/podtatohead.deploy-v0.1.1.triggered.json 