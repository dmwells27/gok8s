package main

import (
	"context"
	"fmt"
	"github.com/IBM/sarama"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Consumer represents a Sarama consumer group consumer
type Consumer struct {
	ready chan bool
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (consumer *Consumer) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready to start consuming messages
	close(consumer.ready)
	return nil
}

// Cleanup is run at the end of a session, after all ConsumeClaim goroutines have exited
func (consumer *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (consumer *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		fmt.Printf("Message claimed: Topic=%s, Partition=%d, Offset=%d, Value=%s\n",
			message.Topic, message.Partition, message.Offset, string(message.Value))
		session.MarkMessage(message, "") // Mark the message as consumed
	}
	return nil
}

func Consume(writer http.ResponseWriter, request *http.Request) {
	brokers := []string{"my-cluster-kafka-brokers.kafka.svc.cluster.local:9092"} // Replace with your Kafka broker addresses
	topic := "access_log"                                                        // Replace with your Kafka topic name
	groupID := "my_consumer_group"                                               // Replace with your consumer group ID

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0 // Specify the appropriate Kafka version
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second

	/**
	 * Setup a new Sarama consumer group
	 */
	consumer := Consumer{
		ready: make(chan bool),
	}

	ctx, cancel := context.WithCancel(context.Background())
	client, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		log.Fatalf("Error creating consumer group client: %v", err)
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// `Consume` should be called inside a loop, when
			// a consumer session is ended, the consumer group library
			// will automatically rebalance and start a new session.
			if err := client.Consume(ctx, strings.Split(topic, ","), &consumer); err != nil {
				log.Printf("Error from consumer: %v", err)
				if err == sarama.ErrClosedConsumerGroup {
					return
				}
			}
			// check if context was cancelled, signaling that the consumer should stop
			if ctx.Err() != nil {
				return
			}
			consumer.ready = make(chan bool) // Reset the ready channel for a new session
		}
	}()

	<-consumer.ready // Await till the consumer has been set up
	log.Println("Sarama consumer up and running! Waiting for messages...")

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		log.Println("terminating: context cancelled")
	case <-sigchan:
		log.Println("terminating: via signal")
	}

	cancel()
	wg.Wait()
	if err = client.Close(); err != nil {
		log.Printf("Error closing client: %v", err)
	}
}
