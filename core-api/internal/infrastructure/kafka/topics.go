package kafka

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	kafkago "github.com/segmentio/kafka-go"
)

// EnsureTopics crea los tópicos si no existen. Se llama al arrancar
// core-api, antes de levantar el consumer: evita la carrera donde el
// consumer arranca contra un tópico que nadie publicó todavía (auto-
// creación solo dispara con el primer mensaje).
func EnsureTopics(ctx context.Context, brokers []string, topics ...string) error {
	conn, err := kafkago.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("conectando a kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("buscando controller de kafka: %w", err)
	}

	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))
	controllerConn, err := kafkago.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return fmt.Errorf("conectando al controller de kafka: %w", err)
	}
	defer controllerConn.Close()

	configs := make([]kafkago.TopicConfig, 0, len(topics))
	for _, topic := range topics {
		configs = append(configs, kafkago.TopicConfig{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
		})
	}

	if err := controllerConn.CreateTopics(configs...); err != nil && !isTopicExistsErr(err) {
		return fmt.Errorf("creando tópicos: %w", err)
	}
	return nil
}

func isTopicExistsErr(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}
