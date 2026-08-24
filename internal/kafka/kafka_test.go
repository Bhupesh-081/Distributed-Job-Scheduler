package kafka

import (
	"os"
	"reflect"
	"testing"
)

func TestBrokers(t *testing.T) {
	os.Unsetenv("KAFKA_BROKERS")
	if got := Brokers(); !reflect.DeepEqual(got, []string{"localhost:9092"}) {
		t.Fatalf("default: got %v", got)
	}

	os.Setenv("KAFKA_BROKERS", "a:9092,b:9092")
	defer os.Unsetenv("KAFKA_BROKERS")
	if got := Brokers(); !reflect.DeepEqual(got, []string{"a:9092", "b:9092"}) {
		t.Fatalf("parsed: got %v", got)
	}
}
