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
	Debug        bool
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
		Debug:        false,
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
		amqpURI   string
		saveFiles bool
		debug     bool
	)
	flag.StringVar(
		&amqpURI,
		"amqp_uri",
		"",
		"The Amqp URI, eg: amqp://guest:guest@localhost:5672/",
	)
	flag.BoolVar(
		&saveFiles,
		"save_files",
		false,
		"if set there will be no clean up of temporary files",
	)
	flag.BoolVar(
		&debug,
		"debug",
		false,
		"sets debug flag, program will print more messages",
	)

	flag.Parse()
	if len(amqpURI) > 0 {
		workerConfig.AmqpURI = amqpURI
	}
	workerConfig.SaveFiles = saveFiles
	workerConfig.Debug = debug
	return workerConfig
}
