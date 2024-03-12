package networktraffic

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"sigs.k8s.io/scheduler-plugins/pkg/networktraffic/testutils"
)

func TestPrometheus(t *testing.T) {
	singleMetricResult := fmt.Sprintf(testutils.MetricResult, testutils.DefaultResultValue)
	multipleMetricsResult := fmt.Sprintf("%s,%s", singleMetricResult, singleMetricResult)
	testCases := []struct {
		name            string
		queryResult     string
		isErrorExpected bool
		requestsToError int
		statusCode      int
	}{
		{
			name:        "success scenario",
			queryResult: fmt.Sprintf(fmt.Sprintf(testutils.SuccessResponseTemplate, testutils.MetricResult), testutils.DefaultResultValue),
		},
		{
			name:            "success response with multiple metrics",
			queryResult:     fmt.Sprintf(testutils.SuccessResponseTemplate, multipleMetricsResult),
			isErrorExpected: true,
		},
		{
			name:            "query failure",
			queryResult:     testutils.ErrorResponse,
			isErrorExpected: true,
			requestsToError: 1,
			statusCode:      404,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockPrometheus := testutils.NewMockPrometheus(t)
			if tc.isErrorExpected {
				mockPrometheus = testutils.NewMockPrometheusWithError(t, tc.requestsToError, tc.statusCode)
			}
			mockPrometheus.HandleFunc("/api/v1/query", mockPrometheus.MockQueryRequestHandler)
			mockPrometheus.SetQuery(tc.queryResult)
			mockServer := httptest.NewServer(mockPrometheus)
			defer mockServer.Close()

			prom := NewPrometheus(mockServer.URL, testutils.DefaultInterface, time.Duration(5)*time.Minute)

			nodeMeasure, err := prom.GetNodeBandwidthMeasure("node05")
			if tc.isErrorExpected {
				if err == nil {
					t.Error("error is expected executing prometheus query")
				}
			} else if int64(nodeMeasure.Value) != testutils.DefaultResultValue {
				t.Errorf("expected response value is %d, got %s", testutils.DefaultResultValue, nodeMeasure.Value)
			}
		})
	}

}
