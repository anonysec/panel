package alerts

import "log"

// AlertHandler is a callback invoked when alerts are emitted.
// Implementations may log, send Telegram notifications, write to DB, etc.
type AlertHandler func(alert Alert)

// LogHandler is the default AlertHandler that logs alerts to stdout.
func LogHandler(alert Alert) {
	log.Printf("[alerts] %s: node=%d value=%.1f threshold=%.1f msg=%q",
		alert.Type, alert.NodeID, alert.Value, alert.Threshold, alert.Message)
}

// Alerter coordinates threshold checking and alert dispatch.
// It holds the configured thresholds and a list of handlers to invoke
// when alerts are emitted.
type Alerter struct {
	Thresholds Thresholds
	handlers   []AlertHandler
}

// NewAlerter creates an Alerter with the given thresholds and the default log handler.
func NewAlerter(thresholds Thresholds) *Alerter {
	return &Alerter{
		Thresholds: thresholds,
		handlers:   []AlertHandler{LogHandler},
	}
}

// OnAlert registers an additional alert handler.
func (a *Alerter) OnAlert(fn AlertHandler) {
	a.handlers = append(a.handlers, fn)
}

// EvaluateMetrics checks metrics against thresholds and dispatches any alerts.
// This is the entry point called by the metrics stream consumer (ProcessEvent).
func (a *Alerter) EvaluateMetrics(nodeID int64, cpu, ram, disk float64) {
	alerts := CheckMetrics(nodeID, cpu, ram, disk, a.Thresholds)
	for _, alert := range alerts {
		a.dispatch(alert)
	}
}

// EvaluateStatusTransition checks a node status change and dispatches an alert if warranted.
// This is the entry point called by the connection pool's OnStatusChange callback.
func (a *Alerter) EvaluateStatusTransition(nodeID int64, old, new string) {
	alert := CheckStatusTransition(nodeID, old, new)
	if alert != nil {
		a.dispatch(*alert)
	}
}

// dispatch sends an alert to all registered handlers.
func (a *Alerter) dispatch(alert Alert) {
	for _, h := range a.handlers {
		h(alert)
	}
}
