// Package kafka is a thin wrapper around segmentio/kafka-go for the job
// pipeline. Messages carry only a job ID; Kafka is a transport/trigger,
// never the source of truth for job state (see docs/design-decisions.md
// "Kafka as transport, not authority"). Every service reads the actual job
// row from Postgres before acting on a message.
package kafka

import (
	"context"
	"os"
	"strings"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
)

const (
	TopicRun   = "run"
	TopicRetry = "retry"
	TopicDead  = "dead"
)

// Brokers reads KAFKA_BROKERS (comma-separated), defaulting to localhost:9092.
func Brokers() []string {
	v := os.Getenv("KAFKA_BROKERS")
	if v == "" {
		return []string{"localhost:9092"}
	}
	return strings.Split(v, ",")
}

// Producer publishes job IDs to a Kafka topic, keyed by job ID so that
// repeated messages for the same job (e.g. retries) land on the same
// partition and stay in order.
type Producer struct {
	w *kafkago.Writer
}

func NewProducer(brokers []string) *Producer {
	return &Producer{w: &kafkago.Writer{
		Addr:     kafkago.TCP(brokers...),
		Balancer: &kafkago.Hash{},
	}}
}

func (p *Producer) PublishJob(ctx context.Context, topic string, jobID uuid.UUID) error {
	return p.w.WriteMessages(ctx, kafkago.Message{
		Topic: topic,
		Key:   []byte(jobID.String()),
		Value: []byte(jobID.String()),
	})
}

func (p *Producer) Close() error { return p.w.Close() }

// Consumer reads job IDs off a topic within a consumer group.
type Consumer struct {
	r *kafkago.Reader
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	return &Consumer{r: kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		// A consumer that joins its group before the topic exists (e.g.
		// this service starting before the first job is ever published)
		// gets assigned zero partitions and, without this, stays stuck
		// there forever: kafka-go doesn't rejoin on its own when the
		// topic shows up later. This makes it notice and rejoin.
		WatchPartitionChanges: true,
	})}
}

// ReadJob blocks for the next message and parses its value as a job ID.
// The raw message is returned too so the caller can commit it.
func (c *Consumer) ReadJob(ctx context.Context) (uuid.UUID, kafkago.Message, error) {
	msg, err := c.r.FetchMessage(ctx)
	if err != nil {
		return uuid.UUID{}, msg, err
	}
	id, err := uuid.Parse(string(msg.Value))
	return id, msg, err
}

func (c *Consumer) Commit(ctx context.Context, msg kafkago.Message) error {
	return c.r.CommitMessages(ctx, msg)
}

func (c *Consumer) Close() error { return c.r.Close() }
