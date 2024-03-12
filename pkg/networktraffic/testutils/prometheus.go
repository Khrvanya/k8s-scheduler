package testutils

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const (
	DefaultResultValue = 1441178050902
	DefaultInterface   = "ens192"
	MetricResult       = `
	{
        "metric": {
          "app": "prometheus",
          "app_kubernetes_io_managed_by": "Helm",
          "chart": "prometheus-13.6.0",
          "component": "node-exporter",
          "device": "ens192",
          "heritage": "Helm",
          "instance": "10.43.0.254:9100",
          "job": "kubernetes-service-endpoints",
          "kubernetes_name": "prometheus-1616380099-node-exporter",
          "kubernetes_namespace": "monitor",
          "kubernetes_node": "node05",
          "release": "prometheus-1616380099"
        },
        "value": [
          1618495272.297,
          "%d"
        ]
      }
	`
	SuccessResponseTemplate = `
	{
		"status": "success",
		"data": {
		  "resultType": "vector",
		  "result": [%s]
		}
	  }
`
	ErrorResponse = `
{
	"status": "error",
	"errorType": "bad_data",
	"error": "invalid parameter \"query\": 1:90: parse error: bad duration syntax: \"5\""
} 
`
)

// Implements http.Handler
type MockPrometheus struct {
	t *testing.T
	*http.ServeMux
	requestsToError    int
	responseStatusCode int
	query              []byte
	// map with node name and value that should be returned
	queryMap map[string]int
}

func NewMockPrometheusWithError(t *testing.T, requestsToError int, responseStatusCode int) MockPrometheus {
	return MockPrometheus{
		t:                  t,
		ServeMux:           http.NewServeMux(),
		requestsToError:    requestsToError,
		responseStatusCode: responseStatusCode,
	}
}

func NewMockPrometheus(t *testing.T) MockPrometheus {
	return NewMockPrometheusWithError(t, -1, http.StatusOK)
}

func (p *MockPrometheus) MockQueryRequestHandler(w http.ResponseWriter, r *http.Request) {
	if p.shouldReturnError() {
		w.WriteHeader(p.responseStatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(p.query))
	if err != nil {
		p.t.Error("Error writing query:", err)
	}
}

func (p *MockPrometheus) MockQueryMapRequestHandler(w http.ResponseWriter, r *http.Request) {
	if p.shouldReturnError() {
		w.WriteHeader(p.responseStatusCode)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	query, _ := url.ParseQuery(string(body))
	v := query["query"]

	node := between(v[0], "kubernetes_node=\"", ",device")
	resValue := p.queryMap[node]
	res := fmt.Sprintf(fmt.Sprintf(SuccessResponseTemplate, MetricResult), resValue)

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write([]byte(res))
	if err != nil {
		p.t.Error("Error writing query:", err)
	}
}

func (p *MockPrometheus) shouldReturnError() bool {
	if p.requestsToError == 1 {
		return true
	}

	p.requestsToError--
	return false
}

func (p *MockPrometheus) SetQuery(value string) {
	p.query = []byte(value)
}

func (p *MockPrometheus) SetQueryMap(value map[string]int) {
	p.queryMap = value
}

func between(value string, a string, b string) string {
	posFirst := strings.Index(value, a)
	if posFirst == -1 {
		return ""
	}
	posLast := strings.Index(value, b)
	if posLast == -1 {
		return ""
	}
	posFirstAdjusted := posFirst + len(a)
	if posFirstAdjusted >= posLast {
		return ""
	}
	return value[posFirstAdjusted:posLast]
}
