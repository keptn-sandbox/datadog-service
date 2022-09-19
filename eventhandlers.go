package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	cloudevents "github.com/cloudevents/sdk-go/v2" // make sure to use v2 cloudevents here
	"github.com/keptn-sandbox/datadog-service/pkg/utils"
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	logger "github.com/sirupsen/logrus"
)

const (
	sliFile                        = "datadog/sli.yaml"
	defaultSleepBeforeAPIInSeconds = 60
)

// We have to put a min of 60s of sleep for the datadog API to reflect the data correctly
// More info: https://github.com/keptn-sandbox/datadog-service/issues/8
var sleepBeforeAPIInSeconds int

func init() {
	var err error
	sleepBeforeAPIInSeconds, err = strconv.Atoi(strings.TrimSpace(os.Getenv("SLEEP_BEFORE_API_IN_SECONDS")))
	if err != nil || sleepBeforeAPIInSeconds < defaultSleepBeforeAPIInSeconds {
		logger.Infof("defaulting SLEEP_BEFORE_API_IN_SECONDS to 60s because it was set to '%v' which is less than the min allowed value of 60s", sleepBeforeAPIInSeconds)
		sleepBeforeAPIInSeconds = defaultSleepBeforeAPIInSeconds
	}
}

// HandleGetSliTriggeredEvent handles get-sli.triggered events if SLIProvider == datadog
func HandleGetSliTriggeredEvent(ddKeptn *keptnv2.Keptn, incomingEvent cloudevents.Event, data *keptnv2.GetSLITriggeredEventData) error {
	var shkeptncontext string
	_ = incomingEvent.Context.ExtensionAs("shkeptncontext", &shkeptncontext)
	configureLogger(incomingEvent.Context.GetID(), shkeptncontext)

	logger.Infof("Handling get-sli.triggered Event: %s", incomingEvent.Context.GetID())

	// Step 1 - Do we need to do something?
	// Lets make sure we are only processing an event that really belongs to our SLI Provider
	if data.GetSLI.SLIProvider != "datadog" {
		logger.Infof("Not handling get-sli event as it is meant for %s", data.GetSLI.SLIProvider)
		return nil
	}

	// Step 2 - Send out a get-sli.started CloudEvent
	// The get-sli.started cloud-event is new since Keptn 0.8.0 and is required to be send when the task is started
	_, err := ddKeptn.SendTaskStartedEvent(data, ServiceName)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to send task started CloudEvent (%s), aborting...", err.Error())
		logger.Error(errMsg)
		return err
	}

	start, err := parseUnixTimestamp(data.GetSLI.Start)
	if err != nil {
		logger.Errorf("unable to parse sli start timestamp: %v", err)
		return err
	}
	end, err := parseUnixTimestamp(data.GetSLI.End)
	if err != nil {
		logger.Errorf("unable to parse sli end timestamp: %v", err)
		return err
	}

	// Step 4 - prep-work
	// Get any additional input / configuration data
	// - Labels: get the incoming labels for potential config data and use it to pass more labels on result, e.g: links
	// - SLI.yaml: if your service uses SLI.yaml to store query definitions for SLIs get that file from Keptn
	labels := data.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	// Step 5 - get SLI Config File
	// Get SLI File from datadog subdirectory of the config repo - to add the file use:
	//   keptn add-resource --project=PROJECT --stage=STAGE --service=SERVICE --resource=my-sli-config.yaml  --resourceUri=datadog/sli.yaml
	sliConfig, err := ddKeptn.GetSLIConfiguration(data.Project, data.Stage, data.Service, sliFile)
	logger.Debugf("SLI config: %v", sliConfig)

	// FYI you do not need to "fail" if sli.yaml is missing, you can also assume smart defaults like we do
	// in keptn-contrib/dynatrace-service and keptn-contrib/prometheus-service
	if err != nil {
		// failed to fetch sli config file
		errMsg := fmt.Sprintf("Failed to fetch SLI file %s from config repo: %s", sliFile, err.Error())
		logger.Error(errMsg)
		// send a get-sli.finished event with status=error and result=failed back to Keptn

		_, err = ddKeptn.SendTaskFinishedEvent(&keptnv2.EventData{
			Status: keptnv2.StatusErrored,
			Result: keptnv2.ResultFailed,
			Labels: labels,
		}, ServiceName)

		return err
	}

	// Step 6 - do your work - iterate through the list of requested indicators and return their values
	// Indicators: this is the list of indicators as requested in the SLO.yaml
	// SLIResult: this is the array that will receive the results
	indicators := data.GetSLI.Indicators
	sliResults := []*keptnv2.SLIResult{}
	ctx := datadog.NewDefaultContext(context.Background())
	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)

	logger.Debug("indicators:", indicators)
	errored := false

	for _, indicatorName := range indicators {
		// Pulling the data from Datadog api immediately gives incorrect data in api response
		// we have to wait for some time for the correct data to be reflected in the api response
		// TODO: Find a better way around the sleep time for datadog api
		logger.Debugf("waiting for %vs so that the metrics data is reflected correctly in the api", sleepBeforeAPIInSeconds)
		time.Sleep(time.Second * time.Duration(sleepBeforeAPIInSeconds))

		query := replaceQueryParameters(data, sliConfig[indicatorName], start, end)
		logger.Debugf("actual query sent to datadog: %v, from: %v, to: %v", query, start.Unix(), end.Unix())
		resp, r, err := apiClient.MetricsApi.QueryMetrics(ctx, start.Unix(), end.Unix(), query)
		if err != nil {
			logger.Errorf("'%s': error getting value for the query: %v : %v\n", query, resp, err)
			logger.Errorf("'%s': full HTTP response: %v\n", query, r)
			errored = true
			continue
		}

		logger.Debugf("response from the metrics api: %v", resp)

		if len((*resp.Series)) != 0 {
			points := *((*resp.Series)[0].Pointlist)
			sliResult := &keptnv2.SLIResult{
				Metric:  indicatorName,
				Value:   *points[len(points)-1][1],
				Success: true,
			}
			logger.WithFields(logger.Fields{"indicatorName": indicatorName}).Debugf("SLI result from the metrics api: %v", sliResult)
			sliResults = append(sliResults, sliResult)
		} else {
			logger.WithFields(logger.Fields{"indicatorName": indicatorName}).Debugf("got 0 in the SLI result (indicates empty response from the API)")
		}

	}

	// Step 7 - Build get-sli.finished event data
	getSliFinishedEventData := &keptnv2.GetSLIFinishedEventData{
		EventData: keptnv2.EventData{
			Status: keptnv2.StatusSucceeded,
			Result: keptnv2.ResultPass,
			Labels: labels,
		},
		GetSLI: keptnv2.GetSLIFinished{
			IndicatorValues: sliResults,
			Start:           data.GetSLI.Start,
			End:             data.GetSLI.End,
		},
	}

	if errored {
		getSliFinishedEventData.EventData.Status = keptnv2.StatusErrored
		getSliFinishedEventData.EventData.Result = keptnv2.ResultFailed
	}

	logger.Debugf("SLI finished event: %v", *getSliFinishedEventData)

	_, err = ddKeptn.SendTaskFinishedEvent(getSliFinishedEventData, ServiceName)

	if err != nil {
		errMsg := fmt.Sprintf("Failed to send task finished CloudEvent (%s), aborting...", err.Error())
		logger.Error(errMsg)
		return err
	}

	return nil
}

func HandleConfigureMonitoringTriggeredEvent(ddKeptn *keptnv2.Keptn, incomingEvent cloudevents.Event, data *keptnv2.ConfigureMonitoringTriggeredEventData) error {
	var shkeptncontext string
	_ = incomingEvent.Context.ExtensionAs("shkeptncontext", &shkeptncontext)
	configureLogger(incomingEvent.Context.GetID(), shkeptncontext)

	logger.Infof("Handling configure-monitoring.triggered Event: %s", incomingEvent.Context.GetID())

	_, err := ddKeptn.SendTaskStartedEvent(data, ServiceName)
	if err != nil {
		logger.Errorf("err when sending task started the event: %v", err)
		return err
	}

	configureMonitoringFinishedEventData := &keptnv2.ConfigureMonitoringFinishedEventData{
		EventData: keptnv2.EventData{
			Status:  keptnv2.StatusSucceeded,
			Result:  keptnv2.ResultPass,
			Project: data.Project,
			Stage:   data.Service,
			Service: data.Service,
			Message: "Finished configuring monitoring",
		},
	}

	logger.Debugf("Configure Monitoring finished event: %v", *configureMonitoringFinishedEventData)

	_, err = ddKeptn.SendTaskFinishedEvent(configureMonitoringFinishedEventData, ServiceName)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to send task finished CloudEvent (%s), aborting...", err.Error())
		logger.Error(errMsg)
		return err
	}

	return nil
}

func configureLogger(eventID, keptnContext string) {
	logger.SetFormatter(&utils.Formatter{
		Fields: logger.Fields{
			"service":      "datadog-service",
			"eventId":      eventID,
			"keptnContext": keptnContext,
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
}

func replaceQueryParameters(data *keptnv2.GetSLITriggeredEventData, query string, start, end time.Time) string {
	query = strings.Replace(query, "$PROJECT", data.Project, -1)
	query = strings.Replace(query, "$STAGE", data.Stage, -1)
	query = strings.Replace(query, "$SERVICE", data.Service, -1)
	query = strings.Replace(query, "$project", data.Project, -1)
	query = strings.Replace(query, "$stage", data.Stage, -1)
	query = strings.Replace(query, "$service", data.Service, -1)
	durationString := strconv.FormatInt(getDurationInSeconds(start, end), 10)
	query = strings.Replace(query, "$DURATION", durationString, -1)
	return query
}

func getDurationInSeconds(start, end time.Time) int64 {

	seconds := end.Sub(start).Seconds()
	return int64(math.Ceil(seconds))
}

func parseUnixTimestamp(timestamp string) (time.Time, error) {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err == nil {
		return parsedTime, nil
	}

	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return time.Now(), err
	}
	unix := time.Unix(timestampInt, 0)
	return unix, nil
}
