package ocrworker

import (
	"encoding/json"
	"flag"
	"github.com/rs/zerolog/log"
)

type RabbitConfig struct {
	AmqpURI      string
	Exchange     string
	ExchangeType string
	RoutingKey   string
	Reliable     bool
	AmqpAPIURI   string
	APIPathQueue string
	APIQueueName string
	APIPathStats string
	QueuePrio    map[string]uint8
	QueuePrioArg string
}

func DefaultTestConfig() RabbitConfig {

	// Reliable: false due to major issues that would completely
	// wedge the rpc worker.  Setting the buffered channels length
	// higher would delay the problem, but then it would still happen later.

	rabbitConfig := RabbitConfig{
		AmqpURI:      "amqp://guest:guest@localhost:5672/",
		Exchange:     "open-ocr-exchange",
		ExchangeType: "direct",
		RoutingKey:   "decode-ocr",
		Reliable:     false, // setting to false because of observed issues
		AmqpAPIURI:   "http://guest:guest@localhost:15672",
		APIPathQueue: "/api/queues/%2f/",
		APIQueueName: "decode-ocr",
		APIPathStats: "/api/nodes",
		QueuePrio:    map[string]uint8{"standard": 1},
	}
	return rabbitConfig

}

type FlagFunction func()

func NoOpFlagFunction() FlagFunction {
	return func() {}
}

func DefaultConfigFlagsOverride(flagFunction FlagFunction) RabbitConfig {
	rabbitConfig := DefaultTestConfig()

	flagFunction()
	var AmqpAPIURI string
	var AmqpURI string
	var QueuePrioArg string
	flag.StringVar(
		&AmqpURI,
		"amqp_uri",
		"",
		"The Amqp URI, eg: amqp://guest:guest@localhost:5672/",
	)
	flag.StringVar(
		&AmqpAPIURI,
		"amqpapi_uri",
		"",
		"The Amqp API URI, eg: http://guest:guest@localhost:15672/",
	)
	flag.StringVar(
		&QueuePrioArg,
		"queue_prio",
		"",
		"JSON formated list wich doc_type and corresponding prio ",
	)

	flag.Parse()
	if len(AmqpURI) > 0 {
		rabbitConfig.AmqpURI = AmqpURI
	}
	if len(AmqpAPIURI) > 0 {
		rabbitConfig.AmqpAPIURI = AmqpAPIURI
	}
	if len(QueuePrioArg) > 0 {
		err := json.Unmarshal([]byte(QueuePrioArg), &rabbitConfig.QueuePrio)
		if err != nil {
			log.Fatal().Err(err).Msg("Message priority argument list is not in a proper JSON format eg. {\"egvp\":9}")
		}
	}

	return rabbitConfig

}
