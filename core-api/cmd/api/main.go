package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	authinfra "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
	kafkainfra "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/kafka"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/memory"
	redisinfra "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/redis"
	transporthttp "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/transport/http"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustGetenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s es obligatoria", key)
	}
	return v
}

func main() {
	port := getenv("PORT", "8080")
	brokers := strings.Split(getenv("KAFKA_BROKERS", "localhost:9092"), ",")

	jwtExpirationMinutes, err := strconv.Atoi(getenv("JWT_EXPIRATION_MINUTES", "60"))
	if err != nil {
		log.Fatalf("JWT_EXPIRATION_MINUTES debe ser un entero: %v", err)
	}
	tokens := authinfra.NewTokenService(mustGetenv("JWT_SECRET"), jwtExpirationMinutes)
	authHandler := transporthttp.NewAuthHandler(tokens, mustGetenv("AUTH_USERNAME"), mustGetenv("AUTH_PASSWORD"))

	ctx := context.Background()

	if err := kafkainfra.EnsureTopics(ctx, brokers, kafkainfra.TopicRiskEvaluationRequested, kafkainfra.TopicRiskEvaluationCompleted); err != nil {
		log.Fatalf("no se pudieron preparar los tópicos de Kafka: %v", err)
	}

	// TODO(Eduard): reemplazar por implementaciones reales de Postgres y
	// Redis cuando existan (ver README, sección de pendientes).
	payments := memory.NewPaymentIntentRepository()
	history := memory.NewPaymentIntentStatusHistoryRepository()
	locker := redisinfra.NoopIdempotencyLocker{}
	uow := memory.UnitOfWork{}
	riskPublisher := kafkainfra.NewRiskRequestPublisher(brokers)
	defer riskPublisher.Close()

	service := application.NewPaymentIntentService(payments, history, locker, uow, riskPublisher)

	consumer := kafkainfra.NewRiskResultConsumer(brokers, "core-api-risk-results", service)
	defer consumer.Close()

	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	go func() {
		if err := consumer.Run(consumerCtx); err != nil && consumerCtx.Err() == nil {
			log.Printf("consumer de riesgo terminó con error: %v", err)
		}
	}()

	paymentHandler := transporthttp.NewPaymentIntentHandler(service)
	readinessHandler := transporthttp.NewReadinessHandler(brokers[0])
	router := transporthttp.NewRouter(paymentHandler, readinessHandler, authHandler, tokens)

	go func() {
		if err := router.Listen(":" + port); err != nil {
			log.Printf("servidor HTTP terminó con error: %v", err)
		}
	}()
	log.Printf("core-api escuchando en :%s", port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("apagando core-api...")
	stopConsumer()
	_ = router.Shutdown()
}
