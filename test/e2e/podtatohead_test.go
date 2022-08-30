package e2e

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/keptn/go-utils/pkg/api/models"
	"github.com/stretchr/testify/require"
)

func TestPodtatoheadEvaluation(t *testing.T) {
	if !isE2ETestingAllowed() {
		t.Skip("Skipping TestHelloWorldDeployment, not allowed by environment")
	}

	// Setup the E2E test environment
	testEnv, err := newTestEnvironment(
		"../events/podtatohead.deploy-v0.1.1.triggered.json",
		"../shipyard/podtatohead.deployment.yaml",
		"../data/podtatohead.jes-config.yaml",
	)

	require.NoError(t, err)

	additionalResources := []struct {
		FilePath     string
		ResourceName string
	}{
		{FilePath: "../data/podtatoserver-0.1.0.tgz", ResourceName: "charts/podtatoserver.tgz"},
		// {FilePath: "../data/locust.basic.py", ResourceName: "locust/basic.py"},
		// {FilePath: "../data/locust.conf", ResourceName: "locust/locust.conf"},
		{FilePath: "../data/podtatohead.sli.yaml", ResourceName: "datadog/sli.yaml"},
		{FilePath: "../data/podtatohead.slo.yaml", ResourceName: "slo.yaml"},
	}

	err = testEnv.SetupTestEnvironment()
	require.NoError(t, err)

	// Make sure project is delete after the tests are completed
	defer testEnv.Cleanup()

	// Upload additional resources to the keptn project
	for _, resource := range additionalResources {
		content, err := ioutil.ReadFile(resource.FilePath)
		require.NoError(t, err, "Unable to read file %s", resource.FilePath)

		err = testEnv.API.AddServiceResource(testEnv.EventData.Project, testEnv.EventData.Stage,
			testEnv.EventData.Service, resource.ResourceName, string(content))

		require.NoErrorf(t, err, "unable to create file %s", resource.ResourceName)
	}

	// Test if the configuration of prometheus was without errors
	t.Run("Configure Datadog", func(t *testing.T) {
		// Configure monitoring
		configureMonitoring, err := readKeptnContextExtendedCE("../events/podtatohead.configure-monitoring.json")
		require.NoError(t, err)

		configureMonitoringContext, err := testEnv.API.SendEvent(configureMonitoring)
		require.NoError(t, err)

		// wait until datadog is configured correctly ...
		requireWaitForEvent(t,
			testEnv.API,
			5*time.Second,
			1*time.Second,
			configureMonitoringContext,
			"sh.keptn.event.configure-monitoring.finished",
			func(event *models.KeptnContextExtendedCE) bool {
				responseEventData, err := parseKeptnEventData(event)
				require.NoError(t, err)

				return responseEventData.Result == "pass" && responseEventData.Status == "succeeded"
			},
			"datadog-service",
		)
	})

	// Test deployment of podtatohead v0.1.1 where all SLI values must be according to SLO
	t.Run("Deploy podtatohead v0.1.1", func(t *testing.T) {
		// Send the event to keptn to deploy, test and evaluate the service
		keptnContext, err := testEnv.API.SendEvent(testEnv.Event)
		require.NoError(t, err)

		// Checking a .started event is received from the evaluation process
		requireWaitForEvent(t,
			testEnv.API,
			15*time.Minute,
			1*time.Second,
			keptnContext,
			"sh.keptn.event.get-sli.started",
			func(_ *models.KeptnContextExtendedCE) bool {
				return true
			},
			"datadog-service",
		)

		requireWaitForEvent(t,
			testEnv.API,
			15*time.Minute,
			1*time.Second,
			keptnContext,
			"sh.keptn.event.get-sli.finished",
			func(event *models.KeptnContextExtendedCE) bool {
				responseEventData, err := parseKeptnEventData(event)
				require.NoError(t, err)

				return responseEventData.Result == "pass" && responseEventData.Status == "succeeded"
			},
			"datadog-service",
		)

		requireWaitForEvent(t,
			testEnv.API,
			15*time.Minute,
			1*time.Second,
			keptnContext,
			"sh.keptn.event.evaluation.finished",
			func(event *models.KeptnContextExtendedCE) bool {
				responseEventData, err := parseKeptnEventData(event)
				require.NoError(t, err)

				return responseEventData.Result == "pass" && responseEventData.Status == "succeeded"
			},
			"lighthouse-service",
		)
	})

	// Test deployment of podtatohead v0.1.2 where the lighthouse-service will fail the evaluation
	t.Run("Deploy podtatohead v0.1.2", func(t *testing.T) {
		event, err := readKeptnContextExtendedCE("../events/podtatohead.deploy-v0.1.2.triggered.json")
		require.NoError(t, err)

		keptnContext, err := testEnv.API.SendEvent(event)
		require.NoError(t, err)

		// Checking a .started event is received from the evaluation process
		requireWaitForEvent(t,
			testEnv.API,
			15*time.Minute,
			1*time.Second,
			keptnContext,
			"sh.keptn.event.get-sli.started",
			func(_ *models.KeptnContextExtendedCE) bool {
				return true
			},
			"datadog-service",
		)

		requireWaitForEvent(t,
			testEnv.API,
			15*time.Minute,
			1*time.Second,
			keptnContext,
			"sh.keptn.event.get-sli.finished",
			func(event *models.KeptnContextExtendedCE) bool {
				responseEventData, err := parseKeptnEventData(event)
				require.NoError(t, err)

				return responseEventData.Result == "pass" && responseEventData.Status == "succeeded"
			},
			"datadog-service",
		)

		requireWaitForEvent(t,
			testEnv.API,
			15*time.Minute,
			1*time.Second,
			keptnContext,
			"sh.keptn.event.evaluation.finished",
			func(event *models.KeptnContextExtendedCE) bool {
				responseEventData, err := parseKeptnEventData(event)
				require.NoError(t, err)

				return responseEventData.Result == "fail" && responseEventData.Status == "succeeded"
			},
			"lighthouse-service",
		)
	})

	// Note: Remediation skipped in this test because it is configured to trigger after 10m
	// TODO: Maybe make a REST call to the alertmanager and ask which alerts are pending?
}
