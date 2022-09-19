package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	cloudevents "github.com/cloudevents/sdk-go/v2" // make sure to use v2 cloudevents here
	"github.com/kelseyhightower/envconfig"

	"github.com/keptn-sandbox/datadog-service/pkg/utils"
	keptnv1 "github.com/keptn/go-utils/pkg/lib"
	"github.com/keptn/go-utils/pkg/lib/keptn"
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	logger "github.com/sirupsen/logrus"
)

var keptnOptions = keptn.KeptnOpts{}

const (
	envVarLogLevel = "LOG_LEVEL"
)

type envConfig struct {
	// Port on which to listen for cloudevents
	Port int `envconfig:"RCV_PORT" default:"8080"`
	// Path to which cloudevents are sent
	Path string `envconfig:"RCV_PATH" default:"/"`
	// Whether we are running locally (e.g., for testing) or on production
	Env string `envconfig:"ENV" default:"local"`
	// URL of the Keptn configuration service (this is where we can fetch files from the config repo)
	ConfigurationServiceUrl string `envconfig:"CONFIGURATION_SERVICE" default:""`
}

// ServiceName specifies the current services name (e.g., used as source when sending CloudEvents)
const ServiceName = "datadog-service"

// based on https://github.com/sirupsen/logrus/pull/653#issuecomment-454467900

/**
 * Parses a Keptn Cloud Event payload (data attribute)
 */
func parseKeptnCloudEventPayload(event cloudevents.Event, data interface{}) error {
	err := event.DataAs(data)
	if err != nil {
		logger.Errorf("Got Data Error: %s", err.Error())
		return err
	}
	return nil
}

/**
 * This method gets called when a new event is received from the Keptn Event Distributor
 * Depending on the Event Type will call the specific event handler functions, e.g: handleDeploymentFinishedEvent
 * See https://github.com/keptn/spec/blob/0.2.0-alpha/cloudevents.md for details on the payload
 */
func processKeptnCloudEvent(ctx context.Context, event cloudevents.Event) error {
	// create keptn handler
	logger.Info("Initializing Keptn Handler")

	// Convert configure.monitoring event to configure-monitoring event
	// This is because keptn CLI sends the former and waits for the latter in the code
	// Issue around this: https://github.com/keptn/keptn/issues/6805
	if event.Type() == keptnv1.ConfigureMonitoringEventType {
		event.SetType(keptnv2.ConfigureMonitoringTaskName)
	}

	ddKeptn, err := keptnv2.NewKeptn(&event, keptnOptions)
	if err != nil {
		return errors.New("Could not create Keptn Handler: " + err.Error())
	}

	logger.Infof("gotEvent(%s): %s - %s", event.Type(), ddKeptn.KeptnContext, event.Context.GetID())

	if err != nil {
		logger.Errorf("failed to parse incoming cloudevent: %v", err)
		return err
	}

	/**
	* CloudEvents types in Keptn 0.8.0 follow the following pattern:
	* - sh.keptn.event.${EVENTNAME}.triggered
	* - sh.keptn.event.${EVENTNAME}.started
	* - sh.keptn.event.${EVENTNAME}.status.changed
	* - sh.keptn.event.${EVENTNAME}.finished
	*
	* For convenience, types can be generated using the following methods:
	* - triggered:      keptnv2.GetTriggeredEventType(${EVENTNAME}) (e.g,. keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName))
	* - started:        keptnv2.GetStartedEventType(${EVENTNAME}) (e.g,. keptnv2.GetStartedEventType(keptnv2.DeploymentTaskName))
	* - status.changed: keptnv2.GetStatusChangedEventType(${EVENTNAME}) (e.g,. keptnv2.GetStatusChangedEventType(keptnv2.DeploymentTaskName))
	* - finished:       keptnv2.GetFinishedEventType(${EVENTNAME}) (e.g,. keptnv2.GetFinishedEventType(keptnv2.DeploymentTaskName))
	*
	* Keptn reserves some Cloud Event types, please read up on that here: https://keptn.sh/docs/0.8.x/manage/shipyard/
	*
	* For those Cloud Events the keptn/go-utils library conveniently provides several data structures
	* and strings in github.com/keptn/go-utils/pkg/lib/v0_2_0, e.g.:
	* - deployment: DeploymentTaskName, DeploymentTriggeredEventData, DeploymentStartedEventData, DeploymentFinishedEventData
	* - test: TestTaskName, TestTriggeredEventData, TestStartedEventData, TestFinishedEventData
	* - ... (they all follow the same pattern)
	*
	*
	* In most cases you will be interested in processing .triggered events (e.g., sh.keptn.event.deployment.triggered),
	* which you an achieve as follows:
	* if event.type() == keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName) { ... }
	*
	* Processing the event payload can be achieved as follows:
	*
	* eventData := &keptnv2.DeploymentTriggeredEventData{}
	* parseKeptnCloudEventPayload(event, eventData)
	*
	* See https://github.com/keptn/spec/blob/0.2.0-alpha/cloudevents.md for more details of Keptn Cloud Events and their payload
	* Also, see https://github.com/keptn-sandbox/echo-service/blob/a90207bc119c0aca18368985c7bb80dea47309e9/pkg/events.go as an example how to create your own CloudEvents
	**/

	/**
	* The following code presents a very generic implementation of processing almost all possible
	* Cloud Events that are retrieved by this service.
	* Please follow the documentation provided above for more guidance on the different types.
	* Feel free to delete parts that you don't need.
	**/
	switch event.Type() {

	// -------------------------------------------------------
	// sh.keptn.event.configure-monitoring (sent by keptnCLI to configure monitoring)
	case keptnv2.ConfigureMonitoringTaskName: // sh.keptn.event.configure-monitoring.triggered
		logger.Infof("Processing configure-monitoring.Triggered Event")

		eventData := &keptnv2.ConfigureMonitoringTriggeredEventData{}
		parseKeptnCloudEventPayload(event, eventData)
		event.SetType(keptnv2.GetTriggeredEventType(keptnv2.ConfigureMonitoringTaskName))

		return HandleConfigureMonitoringTriggeredEvent(ddKeptn, event, eventData)

	// -------------------------------------------------------
	// sh.keptn.event.get-sli (sent by lighthouse-service to fetch SLIs from the sli provider)
	case keptnv2.GetTriggeredEventType(keptnv2.GetSLITaskName): // sh.keptn.event.get-sli.triggered
		logger.Infof("Processing get-sli.triggered Event")

		eventData := &keptnv2.GetSLITriggeredEventData{}
		parseKeptnCloudEventPayload(event, eventData)

		return HandleGetSliTriggeredEvent(ddKeptn, event, eventData)

	}
	// Unknown Event -> Throw Error!
	errorMsg := fmt.Sprintf("Unhandled Keptn Cloud Event: %s", event.Type())

	logger.Error(errorMsg)
	return errors.New(errorMsg)
}

/**
 * Usage: ./main
 * no args: starts listening for cloudnative events on localhost:port/path
 *
 * Environment Variables
 * env=runlocal   -> will fetch resources from local drive instead of configuration service
 */
func main() {
	logger.SetFormatter(&utils.Formatter{
		Fields: logger.Fields{
			"service":      "datadog-service",
			"eventId":      "",
			"keptnContext": "",
		},
		BuiltinFormatter: &logger.TextFormatter{},
	})

	if os.Getenv(envVarLogLevel) != "" {
		logLevel, err := logger.ParseLevel(os.Getenv(envVarLogLevel))
		if err != nil {
			logger.WithError(err).Error("could not parse log level provided by 'LOG_LEVEL' env var")
		} else {
			logger.SetLevel(logLevel)
		}
	}

	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		logger.Fatalf("Failed to process env var: %s", err)
	}

	os.Exit(_main(os.Args[1:], env))
}

/**
 * Opens up a listener on localhost:port/path and passes incoming requets to gotEvent
 */
func _main(args []string, env envConfig) int {
	// configure keptn options
	if env.Env == "local" {
		logger.Info("env=local: Running with local filesystem to fetch resources")
		keptnOptions.UseLocalFileSystem = true
	}

	keptnOptions.ConfigurationServiceURL = env.ConfigurationServiceUrl

	logger.Info("Starting datadog-service...")
	logger.Infof("    on Port = %d; Path=%s", env.Port, env.Path)

	ctx := context.Background()
	ctx = cloudevents.WithEncodingStructured(ctx)

	logger.Infof("Creating new http handler")

	// configure http server to receive cloudevents
	p, err := cloudevents.NewHTTP(cloudevents.WithPath(env.Path), cloudevents.WithPort(env.Port))

	if err != nil {
		logger.Fatalf("failed to create client, %v", err)
	}
	c, err := cloudevents.NewClient(p)
	if err != nil {
		logger.Fatalf("failed to create client, %v", err)
	}

	logger.Infof("Starting receiver")
	logger.Fatal(c.StartReceiver(ctx, processKeptnCloudEvent).Error())
	return 0
}
