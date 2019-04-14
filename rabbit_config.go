package ocrworker

import (
	"flag"
)

type RabbitConfig struct {
	AmqpURI      string
	Exchange     string
	ExchangeType string
	RoutingKey   string
	Reliable     bool
	AmqpAPIURI   string
	APIPort      string
	APIPathQueue string
	APIQueueName string
	APIPathStats string
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

	flag.Parse()
	if len(AmqpURI) > 0 {
		rabbitConfig.AmqpURI = AmqpURI
	}
	if len(AmqpAPIURI) > 0 {
		rabbitConfig.AmqpAPIURI = AmqpAPIURI
	}

	return rabbitConfig

}
