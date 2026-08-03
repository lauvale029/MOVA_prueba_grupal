// Package metrics expone las métricas Prometheus de core-api. Ninguna
// etiqueta lleva merchant_id ni payment_intent_id (cardinalidad no
// acotada) — mismo criterio que ya siguen los tres servicios Python.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var Registry = prometheus.NewRegistry()

var factory = promauto.With(Registry)

var (
	PaymentIntentsCreatedTotal = factory.NewCounter(prometheus.CounterOpts{
		Name: "core_payment_intents_created_total",
		Help: "Payment Intents creados",
	})

	PaymentIntentsResolvedTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "core_payment_intents_resolved_total",
		Help: "Payment Intents resueltos, por decisión de riesgo y camino",
	}, []string{"decision", "via"})

	RiskPublishTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "core_risk_publish_total",
		Help: "Intentos de publicar risk.evaluation.requested, por resultado",
	}, []string{"outcome"})
)

func init() {
	Registry.MustRegister(prometheus.NewGoCollector())
	Registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
}
