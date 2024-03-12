# Overview

Holds the `NetworkTraffic` plugin. A simple plugin that considers prometheus network
metrics while scoring the nodes. The metric currently consider is the node's bandwitdh.

The plugin has the following arguments that need to be provided:

- prometheusAddress: endpoint of the prometheus service in the cluster
- networkInterface: network interface for which the metrics should be considered.
- timeRangeInMinutes: the range for which the metrics will be considered.

## Prometheus Query

The query used in the plugin is reflected bellow and it considers that the prometheus
service is being port forwarded to the localhost:

```shell
curl -g 'http://localhost:9090/api/v1/query?query=sum_over_time(node_network_receive_bytes_total{kubernetes_node="node53",device="ens192"}[5m])' | jq
```

```shell
# Query to retrieve current node network receive in bytes
sum_over_time(node_network_receive_bytes_total{kubernetes_node="node53",device="ens192"}[5m])
# Query to retrieve highest metric which will be used as the 100% value
topk(1,sum_over_time(node_network_receive_bytes_total{device="ens192"}[5m]))
```
