package integration

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/scheduler"
	schedapi "k8s.io/kubernetes/pkg/scheduler/apis/config"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	testutils "k8s.io/kubernetes/test/integration/util"
	imageutils "k8s.io/kubernetes/test/utils/image"
	"sigs.k8s.io/scheduler-plugins/pkg/apis/config"
	"sigs.k8s.io/scheduler-plugins/pkg/networktraffic"
	networktrafficutils "sigs.k8s.io/scheduler-plugins/pkg/networktraffic/testutils"
	"sigs.k8s.io/scheduler-plugins/test/util"
)

var (
	node01winner = map[string]int{
		"node01": 1000,
		"node02": 15,
		"node03": 23,
		"node06": 13,
	}

	node02winner = map[string]int{
		"node01": 1,
		"node02": 15,
		"node03": 10,
		"node06": 13,
	}

	node03winner = map[string]int{
		"node01": 1,
		"node02": 15,
		"node03": 23,
		"node06": 13,
	}

	draw = map[string]int{
		"node01": 15,
		"node02": 23,
		"node03": 23,
		"node06": 13,
	}
)

func TestNetworkTrafficPlugin(t *testing.T) {
	mockPrometheus := networktrafficutils.NewMockPrometheus(t)
	mockPrometheus.HandleFunc("/api/v1/query", mockPrometheus.MockQueryMapRequestHandler)
	mockServer := httptest.NewServer(mockPrometheus)
	defer mockServer.Close()

	expectedResult := map[string][]string{
		"pod01": {"node01"},
		"pod02": {"node02"},
		"pod03": {"node03"},
		"pod04": {"node02", "node03"},
	}

	mockMap := map[string]map[string]int{
		"pod01": node01winner,
		"pod02": node02winner,
		"pod03": node03winner,
		"pod04": draw,
	}

	registry := frameworkruntime.Registry{networktraffic.Name: networktraffic.New}
	profile := schedapi.KubeSchedulerProfile{
		SchedulerName: v1.DefaultSchedulerName,
		Plugins: &schedapi.Plugins{
			Score: &schedapi.PluginSet{
				Enabled: []schedapi.Plugin{
					{Name: networktraffic.Name,
						Weight: 50000},
				},
				Disabled: []schedapi.Plugin{
					{Name: "*"},
				},
			},
		},
		PluginConfig: []schedapi.PluginConfig{
			{
				Name: networktraffic.Name,
				Args: &config.NetworkTrafficArgs{
					Address:            "",
					NetworkInterface:   "",
					TimeRangeInMinutes: 5,
				},
			},
		},
	}

	testCtx := util.InitTestSchedulerWithOptions(
		t,
		testutils.InitTestMaster(t, "sched-allocatable", nil),
		true,
		scheduler.WithProfiles(profile),
		scheduler.WithFrameworkOutOfTreeRegistry(registry),
	)

	defer testutils.CleanupTest(t, testCtx)

	cs, ns := testCtx.ClientSet, testCtx.NS.Name

	nodeNames := []string{"node01", "node02", "node03", "node06"}

	for _, nodeName := range nodeNames {
		node := st.MakeNode().Name(nodeName).Label("node", nodeName).Obj()

		_, err := cs.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create Node %q: %v", nodeName, err)
		}
	}

	// Create Pods.
	var pods []*v1.Pod
	podNames := []string{"pod01", "pod02", "pod03", "pod04"}
	pause := imageutils.GetPauseImageName()
	for i := 0; i < len(podNames); i++ {
		pod := st.MakePod().Namespace(ns).Name(podNames[i]).Container(pause).Obj()
		pods = append(pods, pod)
	}

	t.Logf("Start to create 5 Pods.")
	for i := range pods {
		mockPrometheus.SetQueryMap(mockMap[pods[i].Name])

		t.Logf("Creating Pod %q", pods[i].Name)
		_, err := cs.CoreV1().Pods(ns).Create(context.TODO(), pods[i], metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create Pod %q: %v", pods[i].Name, err)
		}

		// Wait for the pod to be scheduled.
		err = wait.Poll(1*time.Second, 120*time.Second, func() (bool, error) {
			return podScheduled(cs, pods[i].Namespace, pods[i].Name), nil
		})
		if err != nil {
			t.Fatalf("Waiting for pod %q to be scheduled, error: %v", pods[i].Name, err.Error())
		}

		pod, err := cs.CoreV1().Pods(ns).Get(context.TODO(), pods[i].Name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		// The other pods should be scheduled on the small nodes.
		if pod.Spec.NodeName == nodeNames[0] ||
			pod.Spec.NodeName == nodeNames[1] {
			t.Logf("Pod %q is on a small node as expected.", pod.Name)
			continue
		} else {
			t.Errorf("Pod %q is on node %q when it was expected on a small node",
				pod.Name, pod.Spec.NodeName)
		}

		fail := true
		for _, v := range expectedResult[pod.Name] {
			if v == pod.Spec.NodeName {
				fail = false
			}
		}

		if fail {
			t.Errorf("expect pod '%s' to be on node: %s", pod.Name, expectedResult[pod.Name])
		}
	}
}
