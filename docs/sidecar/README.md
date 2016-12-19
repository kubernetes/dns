# sidecar

`sidecar` is a daemon that exports metrics and performs healthcheck on DNS
systems.

## Running

`sidecar` is configured through command line flags, defaults of which
can be found by executing it with `--help`. Important flags to configure:

| Flag | Description |
| ---- | ---- |
| `--dnsmasq-{addr,port}` | endpoint of dnsmasq DNS service |
| `--prometheus-{addr,port}` | endpoint used to export metrics |
| `--probe` label,server,domain name,interval | probe DNS server with domain name every `interval` seconds, reporting its health under `healthcheck/`label. |
