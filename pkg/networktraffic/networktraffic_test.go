package networktraffic

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	fakeframework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1/fake"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	pluginconfig "sigs.k8s.io/scheduler-plugins/pkg/apis/config"
	"sigs.k8s.io/scheduler-plugins/pkg/networktraffic/testutils"
)

func TestNew(t *testing.T) {
	networkTrafficArgs := &pluginconfig.NetworkTrafficArgs{
		Address:            "localhost",
		NetworkInterface:   "ens192",
		TimeRangeInMinutes: 5,
	}

	obj := runtime.Object(networkTrafficArgs)

	_, ok := obj.(*pluginconfig.NetworkTrafficArgs)
	if !ok {
		t.Error("expect NetworkTrafficArgs to implement runtime.Object interface")
	}
}

func TestNetworkTraffic(t *testing.T) {
	nodesMetrics := map[string]int{
		"node01": 1,
		"node02": 15,
		"node03": 10,
		"node06": 13,
	}

	expectedResult := map[string]int{
		"node01": 94,
		"node02": 0,
		"node03": 34,
		"node06": 14,
	}

	nodesRepeatedMetrics := map[string]int{
		"node01": 1,
		"node02": 15,
		"node03": 15,
		"node06": 13,
	}

	expectedResultRepeated := map[string]int{
		"node01": 94,
		"node02": 0,
		"node03": 0,
		"node06": 14,
	}

	nodeInfos := makeNodeInfo("node01", "node02", "node03", "node06")

	testCases := []struct {
		name           string
		nodeInfos      []*framework.NodeInfo
		nodesValues    map[string]int
		expectedResult map[string]int
	}{
		{"happy path", nodeInfos, nodesMetrics, expectedResult},
		{"same metrics", nodeInfos, nodesRepeatedMetrics, expectedResultRepeated},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cs := clientsetfake.NewSimpleClientset()
			informerFactory := informers.NewSharedInformerFactory(cs, 0)
			registeredPlugins := []st.RegisterPluginFunc{
				st.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
				st.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
				st.RegisterScorePlugin(Name, New, 1),
			}
			fakeSharedLister := &fakeSharedLister{nodes: tc.nodeInfos}

			mockPrometheus := testutils.NewMockPrometheus(t)
			mockPrometheus.HandleFunc("/api/v1/query", mockPrometheus.MockQueryRequestHandler)
			mockServer := httptest.NewServer(mockPrometheus)
			defer mockServer.Close()
			args := pluginconfig.NetworkTrafficArgs{NetworkInterface: "ens192", TimeRangeInMinutes: 5, Address: mockServer.URL}

			pluginsConfig := []schedulerapi.PluginConfig{
				{Name: Name, Args: &args},
			}

			fh, err := NewFramework(
				registeredPlugins,
				pluginsConfig,
				frameworkruntime.WithClientSet(cs),
				frameworkruntime.WithInformerFactory(informerFactory),
				frameworkruntime.WithSnapshotSharedLister(fakeSharedLister),
			)
			if err != nil {
				t.Fatalf("fail to create framework: %s", err)
			}

			networkTraffic, err := New(&args, fh)
			if err != nil {
				t.Fatalf("failed to initialize plugin NetworkTraffic, got error: %s", err)
			}

			pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: tc.name}}

			var gotList framework.NodeScoreList
			plugin := networkTraffic.(framework.ScorePlugin)
			for i := range tc.nodeInfos {
				nodeName := tc.nodeInfos[i].Node().Name
				prometheusResult := fmt.Sprintf(fmt.Sprintf(testutils.SuccessResponseTemplate, testutils.MetricResult), tc.nodesValues[nodeName])
				mockPrometheus.SetQuery(prometheusResult)
				score, err := plugin.Score(context.Background(), nil, pod, nodeName)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				gotList = append(gotList, framework.NodeScore{Name: tc.nodeInfos[i].Node().Name, Score: score})
			}

			status := plugin.ScoreExtensions().NormalizeScore(context.Background(), nil, pod, gotList)
			if !status.IsSuccess() {
				t.Errorf("unexpected error: %v", status)
			}

			for _, s := range gotList {
				expectedScore := tc.expectedResult[s.Name]

				if expectedScore != int(s.Score) {
					t.Errorf("expected score is %d, got %d", expectedScore, s.Score)
				}
			}
		})
	}
}

// NewFramework creates a Framework from the register functions and options.
func NewFramework(fns []st.RegisterPluginFunc, pluginConfigs []schedulerapi.PluginConfig, opts ...frameworkruntime.Option) (framework.Framework, error) {
	registry := frameworkruntime.Registry{}
	plugins := &schedulerapi.Plugins{}
	for _, f := range fns {
		f(&registry, plugins, pluginConfigs)
	}
	return frameworkruntime.NewFramework(registry, plugins, pluginConfigs, opts...)
}

func makeNodeInfo(nodesName ...string) []*framework.NodeInfo {
	nodeInfoList := []*framework.NodeInfo{}

	for _, node := range nodesName {
		ni := framework.NewNodeInfo()
		ni.SetNode(&v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: node},
		})
		nodeInfoList = append(nodeInfoList, ni)
	}

	return nodeInfoList
}

var _ framework.SharedLister = &fakeSharedLister{}

type fakeSharedLister struct {
	nodes []*framework.NodeInfo
}

func (f *fakeSharedLister) NodeInfos() framework.NodeInfoLister {
	return fakeframework.NodeInfoLister(f.nodes)
}
