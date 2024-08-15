
# prometheus-expvar-proxy

prometheus-expvar-proxy is a HTTP proxy that rewrites [Go expvars](https://pkg.go.dev/expvar) from scraping targets into Prometheus metrics.

It's adapted from [prometheus-expvar-exporter](blitiri.com.ar/go/prometheus-expvar-exporter).

It supports only simple counters and gauges without any labels, and only the HTTP protocol.

## Install

```
go install github.com/relex/prometheus-expvar-proxy@latest
```

## Run

```
~/go/bin/prometheus-expvar-proxy --addr=0.0.0.0:8000
```

## Details

For example Go expvars from [datadog-agent](https://docs.datadoghq.com/integrations/agent_metrics/):

```json
{
    "logs-agent": {
        "BytesSent": 18481,
        "DestinationErrors": 0,
        "EncodedBytesSent": 1390,
        "HttpDestinationStats": {
            "container-images_9_reliable_0": {
                "idleMs": 0,
```

are translated into:

```
logs_agent_BytesSent 18481
logs_agent_DestinationErrors 0
logs_agent_EncodedBytesSent 1390
logs_agent_HttpDestinationStats_container_images_9_reliable_0_idleMs 0
```
