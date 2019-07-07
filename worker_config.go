package ocrworker

import (
	"flag"
)

type WorkerConfig struct {
	AmqpURI      string
	Exchange     string
	ExchangeType string
	RoutingKey   string
	Reliable     bool
	AmqpAPIURI   string
	APIPathQueue string
	APIQueueName string
	APIPathStats string
	SaveFiles    bool
}

func DefaultWorkerConfig() WorkerConfig {

	// Reliable: false due to major issues that would completely
	// wedge the rpc worker.  Setting the buffered channels length
	// higher would delay the problem, but then it would still happen later.

	workerConfig := WorkerConfig{
		AmqpURI:      "amqp://guest:guest@localhost:5672/",
		Exchange:     "open-ocr-exchange",
		ExchangeType: "direct",
		RoutingKey:   "decode-ocr",
		Reliable:     false, // setting to false because of observed issues
		AmqpAPIURI:   "http://guest:guest@localhost:15672",
		APIPathQueue: "/api/queues/%2f/",
		APIQueueName: "decode-ocr",
		APIPathStats: "/api/nodes",
		SaveFiles:    false,
	}
	return workerConfig

}

type FlagFunctionWorker func()

func NoOpFlagFunctionWorker() FlagFunctionWorker {
	return func() {}
}

func DefaultConfigFlagsWorkerOverride(flagFunction FlagFunctionWorker) WorkerConfig {
	workerConfig := DefaultWorkerConfig()

	flagFunction()
	var (
		AmqpURI string
	//	SaveFiles bool
	)
	flag.StringVar(
		&AmqpURI,
		"amqp_uri",
		"",
		"The Amqp URI, eg: amqp://guest:guest@localhost:5672/",
	)
	/*	flag.BoolVar(
		&SaveFiles,
		"save_files",
		false,
		"if set there will be no clean up of temporary files",
	)*/

	flag.Parse()
	if len(AmqpURI) > 0 {
		workerConfig.AmqpURI = AmqpURI
	}

	return workerConfig
}
