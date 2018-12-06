# DNS stress tester for k8s on GCE / GKE

This tool stress-tests DNS by repeatedly resolving DNS names, by default 100
resolves per second per pod.  It reports metrics to StackDriver.

The metrics reported to StackDriver are filtered, we send 100% of resolves that
take more than 50ms, but only 1% of those that take less than 50ms.  This helps
limit the volume of data being reported to StackDriver.

## Preliminaries: build image, configure StackDriver etc

You'll need a GCP project to collect the metrics from StackDriver, we'll also
use it for gcr.io

Make sure you're in the correct project with `gcloud config get-value project`, if not set it `gcloud config set project $project`.

Enable StackDriver:

```
gcloud services enable cloudtrace.googleapis.com
gcloud services enable logging.googleapis.com
gcloud services enable monitoring.googleapis.com
```

Configure project integration with gcr.io:
```
gcloud auth configure-docker
```

Create a service account and get the key for it (we'll upload this as a k8s secret later):
```
./dns/k8s/secret/get-service-account.sh
```

---

## Run test on k8s

You can run tests on any k8s cluster, we'll report results back to
GCP.

Make sure you're connected to the right k8s cluster (`kubectl get nodes`).
If you're using GKE, you'll want to do `gcloud container clusters get-credentials $name`

Switch to the dns-stress-test namespace:

```
kubectl create ns dns-stress-test
kubectl config set-context $(kubectl config current-context) --namespace=dns-stress-test`
```

Create a list of DNS names to ping; we'll create a configmap with a
single name `kubernetes.default.svc.cluster.local.` which should not
rely on the recursive nameservers, and doesn't rely on search paths.

```
./dns/k8s/simpleconfig/apply.sh
```


We'll also create a secret with the service account we created previously; this
lets us report results to StackDriver:

```
./dns/k8s/secret/apply.sh
```

Finally create the workload (using the k8s/bazel integration, which
automatically builds the image, uploads to GCR, and templates the manifest):

```
bazel run //dns:k8s.apply
```


If you now do `kubectl get pods` you should see the pods,
and `kubectl logs -f $pod` should print `dns stress test` and not much else
(in particular it should not be giving permission errors on StackDriver!)

## Analyzing results in StackDriver

If you go to StackDriver traces in the cloud console, you should see (a lot) of
dots coming in - they each represent a single DNS resolve, and we are doing
several hundred per second - though we are only collecting 1% of those that take
less than 50ms to resolve.  Note that these tests therefore do cost money to
run!

If you click on a dot you can see the details, including attributes of which pod
ran the test, which name it tried to look up (which we locked to
`kubernetes.default.svc.cluster.local.`), and a few other pieces of metadata
about the pod.  You can see how long the request took.  You can see a single
event, corresponding to the log message in the code.

The most interesting one is "variant": this is done by dns-stress-test, and lets
us easily identify the cloud or environment.  The makefile sets it to the name
of the current kubectl context, which with kops will be the cluster DNS name,
and with GKE will include the cluster name and zone.

You can filter this in StackDriver by doing something like `variant:test.k8s.local`

You can also filter in StackDriver by duration using `latency:100` for > 100 ms

Note that StackDriver UI graph only shows the values in the list, and the list
is limited to 1000.  So the range on the y-axis will increase with filters.  I
typically apply filters until I have < 1000 values (if I care about the details,
which we do here!)

## Stress testing

To stress the system, simply scale up the number of repliacs of the
dns-stress-test deployment.  Each pod is ~ 100 requests per second.

`kubectl scale deployment dns-stress-test --replicas=20`

## Removal

Simply delete the namespace:

`kubectl delete ns dns-stress-test`

