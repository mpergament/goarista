// Copyright (C) 2016  Arista Networks, Inc.
// Use of this source code is governed by the Apache License 2.0
// that can be found in the COPYING file.

package producer

import (
	"strings"
	"sync"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/aristanetworks/goarista/kafka/openconfig"
	pb "github.com/aristanetworks/goarista/openconfig"
	"github.com/aristanetworks/goarista/test"
	"github.com/golang/protobuf/proto"
)

type mockAsyncProducer struct {
	input     chan *sarama.ProducerMessage
	successes chan *sarama.ProducerMessage
	errors    chan *sarama.ProducerError
}

func newMockAsyncProducer() *mockAsyncProducer {
	return &mockAsyncProducer{
		input:     make(chan *sarama.ProducerMessage),
		successes: make(chan *sarama.ProducerMessage),
		errors:    make(chan *sarama.ProducerError)}
}

func (p *mockAsyncProducer) AsyncClose() {
	panic("Not implemented")
}

func (p *mockAsyncProducer) Close() error {
	close(p.successes)
	close(p.errors)
	return nil
}

func (p *mockAsyncProducer) Input() chan<- *sarama.ProducerMessage {
	return p.input
}

func (p *mockAsyncProducer) Successes() <-chan *sarama.ProducerMessage {
	return p.successes
}

func (p *mockAsyncProducer) Errors() <-chan *sarama.ProducerError {
	return p.errors
}

func newPath(path string) *pb.Path {
	if path == "" {
		return nil
	}
	return &pb.Path{Element: strings.Split(path, "/")}
}

func TestKafkaProducer(t *testing.T) {
	mock := newMockAsyncProducer()
	toDB := make(chan proto.Message)
	topic := "occlient"
	systemID := "Foobar"
	toDBProducer := &producer{
		notifsChan:    toDB,
		kafkaProducer: mock,
		topic:         topic,
		key:           sarama.StringEncoder(systemID),
		encoder:       openconfig.MessageEncoder,
		done:          make(chan struct{}),
		wg:            sync.WaitGroup{},
	}

	go toDBProducer.Run()

	response := &pb.SubscribeResponse{
		Response: &pb.SubscribeResponse_Update{
			Update: &pb.Notification{
				Timestamp: 0,
				Prefix:    newPath("/foo/bar"),
				Update:    []*pb.Update{},
			},
		},
	}

	toDB <- response

	kafkaMessage := <-mock.input
	if kafkaMessage.Topic != topic {
		t.Errorf("Unexpected Topic: %s, expecting %s", kafkaMessage.Topic, topic)
	}
	key, err := kafkaMessage.Key.Encode()
	if err != nil {
		t.Fatalf("Error encoding key: %s", err)
	}
	if string(key) != systemID {
		t.Errorf("Kafka message didn't have expected key: %s, expecting %s", string(key), systemID)
	}

	valueBytes, err := kafkaMessage.Value.Encode()
	if err != nil {
		t.Fatalf("Error encoding value: %s", err)
	}
	result := pb.SubscribeResponse{}
	err = proto.Unmarshal(valueBytes, &result)
	if err != nil {
		t.Fatalf("Error decoding into protobuf: %s", err)
	}

	if !test.DeepEqual(response, &result) {
		t.Errorf("Protobuf sent from Kafka Producer does not match original.\nOriginal: %v\nNew:%v",
			response, &result)
	}

	toDBProducer.Stop()
}