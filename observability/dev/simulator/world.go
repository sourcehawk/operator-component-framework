package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
)

// owner is a simulated custom resource. It satisfies ocm.ObjectLike.
type owner struct {
	namespace, name string
}

func (o owner) GetName() string      { return o.name }
func (o owner) GetNamespace() string { return o.namespace }

// resource is one managed resource of a component, as the framework labels it.
type resource struct {
	component, identifier, kind string
}

// controllerSim drives one controller's worth of series: a framework recorder
// for conditions and applies, and the controller-runtime lookalikes.
type controllerSim struct {
	name      string
	ownerKind string
	rec       *metrics.Recorder
	rt        *runtimeMetrics
}

func (c *controllerSim) labels(r resource) component.ResourceMetricLabels {
	return component.ResourceMetricLabels{
		OwnerKind: c.ownerKind, Component: r.component, Identifier: r.identifier, Kind: r.kind,
	}
}

// reconcile records one reconcile: the workqueue hand-off, the reconcile
// result and duration, and a few REST calls. apply and condition recording is
// left to the caller, which knows the scenario.
func (c *controllerSim) reconcile(result string, queueWait, duration time.Duration) {
	c.rt.queueAdds.WithLabelValues(c.name, c.name).Inc()
	c.rt.queueDuration.WithLabelValues(c.name, c.name).Observe(queueWait.Seconds())
	c.rt.workDuration.WithLabelValues(c.name, c.name).Observe(duration.Seconds())
	c.rt.reconcileTime.WithLabelValues(c.name).Observe(duration.Seconds())
	c.rt.reconcileTotal.WithLabelValues(c.name, result).Inc()
	if result == resultError {
		c.rt.reconcileErrors.WithLabelValues(c.name).Inc()
		c.rt.queueRetries.WithLabelValues(c.name, c.name).Inc()
	}
	c.rt.restRequests.WithLabelValues("200", "GET", "10.96.0.1:443").Add(float64(2 + rand.IntN(3)))
	c.rt.restRequests.WithLabelValues("200", "PATCH", "10.96.0.1:443").Add(float64(1 + rand.IntN(4)))
	if rand.IntN(20) == 0 {
		c.rt.restRequests.WithLabelValues("409", "PATCH", "10.96.0.1:443").Inc()
	}
}

func (c *controllerSim) apply(r resource, op concepts.ConvergingOperation) {
	c.rec.RecordResourceApply(c.labels(r), op)
}

func (c *controllerSim) applyError(r resource) {
	c.rec.RecordResourceApplyError(c.labels(r))
}

func (c *controllerSim) condition(o owner, conditionType, status, reason string, since time.Time) {
	c.rec.RecordConditionFor(c.ownerKind, o, conditionType, status, reason, since)
}

// world holds every scenario and runs them until ctx is done.
type world struct {
	webapp, database *controllerSim
	rt               *runtimeMetrics
	leader           bool
	start            time.Time
}

func newWorld(conditions *ocm.OperatorConditionsGauge, collectors *metrics.Collectors, rt *runtimeMetrics, leader bool) *world {
	w := &world{rt: rt, leader: leader, start: time.Now()}
	w.webapp = &controllerSim{
		name: "webapp", ownerKind: "WebApp", rt: rt,
		rec: metrics.NewRecorder("webapp", conditions, collectors),
	}
	w.database = &controllerSim{
		name: "database", ownerKind: "Database", rt: rt,
		rec: metrics.NewRecorder("database", conditions, collectors),
	}
	rt.initController("webapp", 4)
	rt.initController("database", 2)
	return w
}

// Component and namespace names reused across the scripted world.
const (
	componentServer = "server"
	namespaceShop   = "shop"
)

// Condition reasons the simulated owners report.
const (
	reasonHealthy  = "Healthy"
	reasonFailing  = "Failing"
	reasonCreating = "Creating"
	reasonUnknown  = "Unknown"
)

var (
	webappDeployment = resource{componentServer, "deployment", "Deployment"}
	webappService    = resource{componentServer, "service", "Service"}
	webappConfigMap  = resource{componentServer, "configmap", "ConfigMap"}
	webappIngress    = resource{"ingress", "ingress", "Ingress"}

	dbStatefulSet = resource{"storage", "statefulset", "StatefulSet"}
	dbPVC         = resource{"storage", "pvc", "PersistentVolumeClaim"}
	dbCronJob     = resource{"backup", "cronjob", "CronJob"}
)

// run starts every scenario goroutine and blocks until ctx is done.
func (w *world) run(ctx context.Context) {
	w.rt.leader.WithLabelValues("demo-operator").Set(boolToFloat(w.leader))

	go w.healthyWebApps(ctx)
	go w.hotLoop(ctx)
	go w.databases(ctx)
	go w.workqueueBacklog(ctx)
	go w.panics(ctx)
	<-ctx.Done()
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// healthyWebApps: fifty owners; one random owner reconciles every six to
// eleven seconds, so each of the fifty is reconciled roughly every five to ten
// minutes on average. All resources converged, Ready=True since the simulator
// started. The hot-loop owner (webapp-01) is among them and is also healthy by
// its conditions.
func (w *world) healthyWebApps(ctx context.Context) {
	c := w.webapp
	owners := make([]owner, 50)
	for i := range owners {
		owners[i] = owner{namespace: fmt.Sprintf("team-%c", 'a'+rune(i%5)), name: fmt.Sprintf("webapp-%02d", i+1)}
	}
	tick := func(o owner) {
		c.reconcile("success", time.Duration(rand.IntN(50))*time.Millisecond, time.Duration(50+rand.IntN(250))*time.Millisecond)
		for _, r := range []resource{webappDeployment, webappService, webappConfigMap, webappIngress} {
			c.apply(r, concepts.ConvergingOperationNone)
		}
		c.condition(o, "ServerReady", "True", reasonHealthy, w.start)
		c.condition(o, "IngressReady", "True", reasonHealthy, w.start)
		c.condition(o, "Ready", "True", reasonHealthy, w.start)
	}
	for _, o := range owners {
		tick(o)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(6+rand.IntN(6)) * time.Second):
			tick(owners[rand.IntN(len(owners))])
		}
	}
}

// hotLoop: webapp-01's configmap is rewritten on every reconcile, four times a
// second, the way a non-idempotent mutation behaves once the watch event
// requeues the owner. The other resources of that owner converge.
func (w *world) hotLoop(ctx context.Context) {
	c := w.webapp
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.reconcile("success", 5*time.Millisecond, time.Duration(30+rand.IntN(40))*time.Millisecond)
			c.apply(webappConfigMap, concepts.ConvergingOperationUpdated)
			c.apply(webappDeployment, concepts.ConvergingOperationNone)
			c.apply(webappService, concepts.ConvergingOperationNone)
		}
	}
}

// databases: six owners. orders-db has been Ready=False (Failing) for eight
// hours; users-db is Ready=Unknown; reports-db stays Ready=False while its
// reason flips between Failing and Creating; the rest are healthy, among them
// the cluster-scoped shared-gateway, whose empty namespace makes the condition
// gauge export series without a namespace label. PVC applies fail three times
// out of four, thirty percent of reconciles error, and reconcile durations are
// slow enough for the p99 to cross a minute.
func (w *world) databases(ctx context.Context) {
	c := w.database
	orders := owner{namespaceShop, "orders-db"}
	users := owner{namespaceShop, "users-db"}
	reports := owner{"analytics", "reports-db"}
	healthy := []owner{{namespaceShop, "carts-db"}, {"analytics", "events-db"}}
	clusterOwner := owner{namespace: "", name: "shared-gateway"}
	stuckSince := w.start.Add(-8 * time.Hour)
	unknownSince := w.start.Add(-1 * time.Hour)
	flipSince := w.start
	flipReason := reasonFailing

	i := 0
	tick := func() {
		i++
		result := "success"
		if rand.IntN(10) < 3 {
			result = resultError
		}
		duration := time.Duration(5+rand.IntN(60)) * time.Second
		if rand.IntN(10) < 2 {
			duration = time.Duration(70+rand.IntN(50)) * time.Second
		}
		c.reconcile(result, time.Duration(rand.IntN(400))*time.Millisecond, duration)
		c.apply(dbStatefulSet, concepts.ConvergingOperationNone)
		c.apply(dbCronJob, concepts.ConvergingOperationNone)
		if i%4 == 0 {
			c.apply(dbPVC, concepts.ConvergingOperationNone)
		} else {
			c.applyError(dbPVC)
		}

		if i%60 == 0 { // every three minutes
			// A reason-only change keeps lastTransitionTime, as
			// meta.SetStatusCondition does while the status stays False.
			if flipReason == reasonFailing {
				flipReason = reasonCreating
			} else {
				flipReason = reasonFailing
			}
		}
		c.condition(orders, "StorageReady", "False", reasonFailing, stuckSince)
		c.condition(orders, "BackupReady", "True", reasonHealthy, stuckSince)
		c.condition(orders, "Ready", "False", reasonFailing, stuckSince)
		c.condition(users, "StorageReady", "Unknown", reasonUnknown, unknownSince)
		c.condition(users, "BackupReady", "True", reasonHealthy, unknownSince)
		c.condition(users, "Ready", "Unknown", reasonUnknown, unknownSince)
		c.condition(reports, "StorageReady", "False", flipReason, flipSince)
		c.condition(reports, "BackupReady", "True", reasonHealthy, w.start)
		c.condition(reports, "Ready", "False", flipReason, flipSince)
		for _, o := range healthy {
			c.condition(o, "StorageReady", "True", reasonHealthy, w.start)
			c.condition(o, "BackupReady", "True", reasonHealthy, w.start)
			c.condition(o, "Ready", "True", reasonHealthy, w.start)
		}
		c.condition(clusterOwner, "StorageReady", "True", reasonHealthy, w.start)
		c.condition(clusterOwner, "Ready", "True", reasonHealthy, w.start)
	}
	tick()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

// workqueueBacklog: the database queue is deep, items wait minutes, and both
// of the database controller's workers stay busy chewing through it, while
// webapp's workers idle between zero and two. A reconcile-scoped flip of
// active_workers would almost never be caught by a 5s scrape, so the gauges
// are modelled as persistent levels instead.
func (w *world) workqueueBacklog(ctx context.Context) {
	c := w.database
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			depth := 30 + rand.IntN(20)
			c.rt.queueDepth.WithLabelValues(c.name, c.name, "0").Set(float64(depth))
			c.rt.queueDuration.WithLabelValues(c.name, c.name).Observe(float64(200 + rand.IntN(400)))
			c.rt.queueUnfinished.WithLabelValues(c.name, c.name).Set(float64(60 + rand.IntN(120)))
			c.rt.queueLongestRunning.WithLabelValues(c.name, c.name).Set(float64(30 + rand.IntN(90)))
			c.rt.queueDepth.WithLabelValues(w.webapp.name, w.webapp.name, "0").Set(float64(rand.IntN(3)))
			c.rt.activeWorkers.WithLabelValues(c.name).Set(2)
			c.rt.activeWorkers.WithLabelValues(w.webapp.name).Set(float64(rand.IntN(3)))
		}
	}
}

// panics: the database controller panics once every 20 minutes, starting
// one minute in, so ControllerReconcilePanics is visible without waiting long.
func (w *world) panics(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Minute):
	}
	ticker := time.NewTicker(20 * time.Minute)
	defer ticker.Stop()
	for {
		w.database.rt.reconcilePanics.WithLabelValues("database").Inc()
		w.database.rt.reconcileTotal.WithLabelValues("database", "error").Inc()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
