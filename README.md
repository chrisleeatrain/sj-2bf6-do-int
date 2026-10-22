# txn-api sandbox

A small fake transactions service, and what's needed to run it on a local Kubernetes cluster.

| Path | What it is |
| --- | --- |
| `app/go`, `app/python` | The same service written twice. Pick whichever you're more comfortable with. |
| `deploy/helm/txn-api` | Helm chart for the service |
| `terraform/` | Namespace, shared config, and credentials |
| `scripts/` | `doctor.sh` (prereq check), `loadgen.sh` (sends traffic) |
| `data/` | Sample production logs |
| `kind-config.yaml`, `kind-seed.yaml` | Local cluster definition, and objects that exist in prod but aren't in Terraform |

## The session

45 minutes, screen shared, in your own terminal or a Codespace. Treat this like a repo a teammate handed you. Nobody is expected to finish everything; we care more about how you work than how far you get.

- Think out loud. If you're stuck, say so.
- Use whatever you normally would: your editor, shell, docs, search.
- AI tools are fine. Keep them on the shared screen and tell us what you asked.
- kind is just the local cluster, not part of the exercise. Every kind command here works as written.

## Prerequisites

Docker, [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), kubectl, [Helm](https://helm.sh/docs/intro/install/) 3+, Terraform 1.11+. Python 3 and `jq` are optional.

```sh
make doctor
```

If you'd rather not install anything, open the repo in a Codespace (**Code → Codespaces → Create codespace**, 4-core or larger). Everything is preinstalled.

## Bring the service up

```sh
export APP_LANG=go   # or python
```

1. Create the cluster. This also applies `kind-seed.yaml`.

   ```sh
   make cluster
   ```

2. Apply the Terraform.

   ```sh
   cd terraform
   terraform init
   terraform plan
   terraform apply
   cd ..
   ```

3. Build the image.

   ```sh
   docker build -t txn-api-$APP_LANG:dev app/$APP_LANG
   ```

4. Load the image into the cluster (repeat after every rebuild) and deploy.

   ```sh
   kind load docker-image txn-api-$APP_LANG:dev --name txn-sandbox
   helm upgrade --install txn-api deploy/helm/txn-api \
     -n txn-platform \
     --set image.repository=txn-api-$APP_LANG
   ```

5. Test it.

   ```sh
   kubectl -n txn-platform port-forward svc/txn-api 8080:80
   ```

   In another terminal:

   ```sh
   curl localhost:8080/healthz
   curl -X POST localhost:8080/api/transactions -d '{"amount_cents": 1250, "currency": "USD"}'
   make loadgen
   ```

`make help` lists shortcuts for these steps. `make clean` tears everything down.

## If there's time

- **Adopt the `txn-reports` namespace.** It's in the cluster but not in Terraform: someone created it by hand during the Q2 reporting incident, and the reporting team still uses it. Bring it under the Terraform in `terraform/`. It's a production namespace, so it has to come through untouched: not deleted, not recreated, not changed.
- **Review the credentials.** `terraform/secrets.tf` came from a teammate's PR and was applied in step 2. Read it as the reviewer on that PR and tell us what you think.

## If there's more time

Pick one, or just talk us through it:

- **Answer a question with a script** (bash or Python), either:
  - *Live metrics.* With `make loadgen` running, use `/metrics` to report the current 5xx error rate for each route.
  - *Logs.* `data/txn-api.log.jsonl` is an hour of production logs during a rough patch. Which route had the worst p95 latency, and what are the three most common error messages? Roughly when did things go wrong?
- Add resource requests/limits and a PodDisruptionBudget to the chart. How would you pick the numbers?
- What would you alert on for this service, and at what thresholds?

## Reference

### kind

| What | Command |
| --- | --- |
| List clusters | `kind get clusters` |
| Copy a local image into the cluster | `kind load docker-image <image>:<tag> --name txn-sandbox` |
| See which images the node has | `docker exec txn-sandbox-control-plane crictl images` |
| Shell into the node | `docker exec -it txn-sandbox-control-plane bash` |
| Delete the cluster | `kind delete cluster --name txn-sandbox` |

Everything else is ordinary kubectl against the `kind-txn-sandbox` context.

### The service

| Endpoint | Description |
| --- | --- |
| `GET /healthz` | Liveness. Always 200 once the process is up. |
| `GET /readyz` | Readiness. 200 when the service is ready for traffic. |
| `POST /api/transactions` | Create a transaction. Body: `{"amount_cents": 1250, "currency": "USD"}`. Currencies: USD, EUR, GBP, USDC. |
| `GET /api/transactions/{id}` | Fetch a transaction. Storage is in memory, per pod. |
| `GET /metrics` | Prometheus metrics |

Configuration (environment variables):

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `TXN_REGION` | yes | | Region label stamped on transactions and logs. The service exits at startup without it. |
| `TXN_FAIL_RATE` | no | `0` | Fraction of requests (0–1) that fail with a simulated ledger error |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |
| `PORT` | no | `8000` | Listen port |

Metrics:

| Metric | Type | Labels |
| --- | --- | --- |
| `http_requests_total` | counter | `method`, `route`, `status` |
| `http_request_duration_seconds` | histogram | `method`, `route` |
| `txn_processed_total` | counter | `currency`, `result` (`ok`, `rejected`, `error`) |

Logs are JSON, one object per line, on stdout.
