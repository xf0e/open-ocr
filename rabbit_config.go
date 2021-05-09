package ocrworker

import (
	"encoding/json"
	"flag"
	"fmt"

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
	/* ResponseCacheTimeout sets default(!!!) global timeout in seconds for request
	   engine will be killed after reaching the time limit, user will get timeout error */
	ResponseCacheTimeout uint
	// MaximalResponseCacheTimeout client won't be able set the ResponseCacheTimeout higher of it's value
	MaximalResponseCacheTimeout uint
	FactorForMessageAccept      uint
}

func DefaultTestConfig() RabbitConfig {

	// Reliable: false due to major issues that would completely
	// wedge the rpc worker.  Setting the buffered channels length
	// higher would delay the problem, but then it would still happen later.

	rabbitConfig := RabbitConfig{
		AmqpURI:                     "amqp://guest:guest@localhost:5672/",
		Exchange:                    "open-ocr-exchange",
		ExchangeType:                "direct",
		RoutingKey:                  "decode-ocr",
		Reliable:                    false, // setting to false because of observed issues
		AmqpAPIURI:                  "http://guest:guest@localhost:15672",
		APIPathQueue:                "/api/queues/%2f/",
		APIQueueName:                "decode-ocr",
		APIPathStats:                "/api/nodes",
		QueuePrio:                   map[string]uint8{"standard": 1},
		ResponseCacheTimeout:        28800,
		MaximalResponseCacheTimeout: 28800,
		// tickerWithPostActionInterval: time.Second * 2,
		FactorForMessageAccept: 2,
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
	var (
		AmqpAPIURI                  string
		AmqpURI                     string
		QueuePrioArg                string
		ResponseCacheTimeout        uint
		MaximalResponseCacheTimeout uint
		FactorForMessageAccept      uint
	)
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
		"JSON formatted list which doc_type and corresponding priority of maximal value of 9 e.g. -queue_prio {\"egvp\":9}",
	)
	flag.UintVar(
		&ResponseCacheTimeout,
		"default_timeout",
		28800,
		"Default(!) timeout in seconds for request; ocr job will be killed after reaching this limit "+
			"and generated ocr response will contain an timeout error",
	)
	flag.UintVar(
		&MaximalResponseCacheTimeout,
		"maximal_timeout",
		28800,
		"Clients won't be able to set timeout for ocr requests higher than this value in seconds",
	)
	flag.UintVar(
		&FactorForMessageAccept,
		"worker_factor",
		2,
		"Limits number of accepted request by formula worker_factor * number of running workers.",
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
	} else {
		rabbitConfig.QueuePrioArg = QueuePrioArg
	}

	if MaximalResponseCacheTimeout < ResponseCacheTimeout {
		err := fmt.Errorf("maximal_timeout is lower than default_timeout")
		log.Fatal().Err(err).Msg("setting maximal_timeout lower than default_timeout is not allowed, if in doubt set both to same value")
	} else {
		rabbitConfig.MaximalResponseCacheTimeout = MaximalResponseCacheTimeout
		rabbitConfig.ResponseCacheTimeout = ResponseCacheTimeout
	}

	if FactorForMessageAccept > 0 {
		rabbitConfig.FactorForMessageAccept = FactorForMessageAccept
	}

	return rabbitConfig
}
