package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	cloudevents "github.com/cloudevents/sdk-go/v2" // make sure to use v2 cloudevents here
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
)

// HandleGetSliTriggeredEvent handles get-sli.triggered events if SLIProvider == datadog
func HandleGetSliTriggeredEvent(myKeptn *keptnv2.Keptn, incomingEvent cloudevents.Event, data *keptnv2.GetSLITriggeredEventData) error {
	log.Printf("Handling get-sli.triggered Event: %s", incomingEvent.Context.GetID())

	// Step 1 - Do we need to do something?
	// Lets make sure we are only processing an event that really belongs to our SLI Provider
	if data.GetSLI.SLIProvider != "datadog" {
		log.Printf("Not handling get-sli event as it is meant for %s", data.GetSLI.SLIProvider)
		return nil
	}

	// Step 2 - Send out a get-sli.started CloudEvent
	// The get-sli.started cloud-event is new since Keptn 0.8.0 and is required to be send when the task is started
	_, err := myKeptn.SendTaskStartedEvent(data, ServiceName)
	if err != nil {
		log.Printf("err when starting the event: %v", err)
	}

	start, err := parseUnixTimestamp(data.GetSLI.Start)
	if err != nil {
		return err
	}
	end, err := parseUnixTimestamp(data.GetSLI.End)
	if err != nil {
		return err
	}

	if err != nil {
		errMsg := fmt.Sprintf("Failed to send task started CloudEvent (%s), aborting...", err.Error())
		log.Println(errMsg)
		return err
	}

	// Step 4 - prep-work
	// Get any additional input / configuration data
	// - Labels: get the incoming labels for potential config data and use it to pass more labels on result, e.g: links
	// - SLI.yaml: if your service uses SLI.yaml to store query definitions for SLIs get that file from Keptn
	// labels := data.Labels
	// if labels == nil {
	// 	labels = make(map[string]string)
	// }
	// testRunID := labels["testRunId"]

	// Step 5 - get SLI Config File
	// Get SLI File from datadog subdirectory of the config repo - to add the file use:
	//   keptn add-resource --project=PROJECT --stage=STAGE --service=SERVICE --resource=my-sli-config.yaml  --resourceUri=datadog/sli.yaml
	sliFile := "datadog/sli.yaml"
	// sliConfigFileContent, err := myKeptn.GetKeptnResource(sliFile)
	queries, err := myKeptn.GetSLIConfiguration(data.Project, data.Stage, data.Service, sliFile)

	// FYI you do not need to "fail" if sli.yaml is missing, you can also assume smart defaults like we do
	// in keptn-contrib/dynatrace-service and keptn-contrib/prometheus-service
	if err != nil {
		// failed to fetch sli config file
		errMsg := fmt.Sprintf("Failed to fetch SLI file %s from config repo: %s", sliFile, err.Error())
		log.Println(errMsg)
		// send a get-sli.finished event with status=error and result=failed back to Keptn

		_, err = myKeptn.SendTaskFinishedEvent(&keptnv2.EventData{
			Status: keptnv2.StatusErrored,
			Result: keptnv2.ResultFailed,
		}, ServiceName)

		return err
	}

	fmt.Println(queries)

	// Step 6 - do your work - iterate through the list of requested indicators and return their values
	// Indicators: this is the list of indicators as requested in the SLO.yaml
	// SLIResult: this is the array that will receive the results
	indicators := data.GetSLI.Indicators
	sliResults := []*keptnv2.SLIResult{}
	ctx := datadog.NewDefaultContext(context.Background())
	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)

	for _, indicatorName := range indicators {
		// for i := 0; i < 2; i++ {
		time.Sleep(time.Second * 30)
		fmt.Println("indicators", indicators)

		fmt.Println("indicatorName", indicatorName)
		query := replaceQueryParameters(data, queries[indicatorName], start, end)

		fmt.Println("QUERY", query)
		resp, r, err := apiClient.MetricsApi.QueryMetrics(ctx, start.Unix(), end.Unix(), query)

		if err != nil {
			log.Printf("'%s': error getting value for the query: %v : %v\n", query, resp, err)
			log.Printf("'%s': full HTTP response: %v\n", query, r)
			continue
		}

		log.Printf("resp: %v", resp)
		responseContent, _ := json.MarshalIndent(resp, "", "  ")
		log.Printf("Response from `MetricsApi.QueryMetrics`:\n%s\n", responseContent)
		log.Println("(*resp.Series)", (*resp.Series))
		// TODO: Use logger here?

		if len((*resp.Series)) != 0 {
			points := *((*resp.Series)[0].Pointlist)
			sliResult := &keptnv2.SLIResult{
				Metric:  indicatorName,
				Value:   *points[len(points)-1][1],
				Success: true,
			}
			log.Printf("sliResult: %v", sliResult)
			sliResults = append(sliResults, sliResult)
		}
		// }

	}

	// Step 7 - add additional context via labels (e.g., a backlink to the monitoring or CI tool)
	// labels["Link to Data Source"] = "https://mydatasource/myquery?testRun=" + testRunID

	// Step 8 - Build get-sli.finished event data
	getSliFinishedEventData := &keptnv2.GetSLIFinishedEventData{
		EventData: keptnv2.EventData{
			Status: keptnv2.StatusSucceeded,
			Result: keptnv2.ResultPass,
		},
		GetSLI: keptnv2.GetSLIFinished{
			IndicatorValues: sliResults,
			Start:           data.GetSLI.Start,
			End:             data.GetSLI.End,
		},
	}

	_, err = myKeptn.SendTaskFinishedEvent(getSliFinishedEventData, ServiceName)

	if err != nil {
		errMsg := fmt.Sprintf("Failed to send task finished CloudEvent (%s), aborting...", err.Error())
		log.Println(errMsg)
		return err
	}

	return nil
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
