package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	gce "cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/monitoring/v3"
)

var (
	name  = flag.String("name", "", "The metric name.")
	value = flag.Float64("value", 0.0, "The value to export.")
)

func main() {
	flag.Parse()
	export(*name, *value)
}

func export(name string, value float64) {
	sd, err := monitoring.New(oauth2.NewClient(context.Background(), google.ComputeTokenSource("")))
	if err != nil {
		panic(err)
	}

	projectID, _ := gce.ProjectID()
	project := "projects/" + projectID
	metric, request := buildTimeSeriesRequest(name, value)
	if _, err = sd.Projects.TimeSeries.Create(project, request).Do(); err != nil {
		panic(err)
	}
	log.Printf("Exportted custom metric '%v' = %v.", metric, value)
}

func buildTimeSeriesRequest(name string, value float64) (string, *monitoring.CreateTimeSeriesRequest) {
	metricType := "custom.googleapis.com/" + name
	metricLabels := map[string]string{}
	monitoredResourceType := "k8s_cluster"
	monitoredResourceLabels := buildMonitoredResourceLabels()
	now := time.Now().Format(time.RFC3339)
	return metricType, &monitoring.CreateTimeSeriesRequest{
		TimeSeries: []*monitoring.TimeSeries{
			{
				Metric: &monitoring.Metric{
					Type:   metricType,
					Labels: metricLabels,
				},
				Resource: &monitoring.MonitoredResource{
					Type:   monitoredResourceType,
					Labels: monitoredResourceLabels,
				},
				Points: []*monitoring.Point{{
					Interval: &monitoring.TimeInterval{
						EndTime: now,
					},
					Value: &monitoring.TypedValue{
						DoubleValue: &value,
					},
				}},
			},
		},
	}
}

func buildMonitoredResourceLabels() map[string]string {
	projectID, _ := gce.ProjectID()
	location, _ := gce.InstanceAttributeValue("cluster-location")
	location = strings.TrimSpace(location)
	clusterName, _ := gce.InstanceAttributeValue("cluster-name")
	clusterName = strings.TrimSpace(clusterName)
	return map[string]string{
		"project_id":   projectID,
		"location":     location,
		"cluster_name": clusterName,
	}
}
