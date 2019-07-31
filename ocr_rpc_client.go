package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/url"

	//"github.com/sasha-s/go-deadlock"
	"github.com/streadway/amqp"
	"os"
	"sync"
	"time"
)

// rpcResponseTimeout sets timeout for getting the result from channel
var rpcResponseTimeout = time.Second * 20

type OcrRpcClient struct {
	rabbitConfig RabbitConfig
	connection   *amqp.Connection
	channel      *amqp.Channel
}

type OcrResult struct {
	Text   string `json:"text"`
	Status string `json:"status"`
	ID     string `json:"id"`
}

func newOcrResult(id string) OcrResult {
	ocrResult := &OcrResult{}
	ocrResult.Status = "processing"
	ocrResult.ID = id
	return *ocrResult
}

var (
	requestsAndTimersMu sync.RWMutex
	// Requests is for holding and monitoring queued requests
	Requests = make(map[string]chan OcrResult)
	timers   = make(map[string]*time.Timer)
)
var (
	numRetries uint8 = 3
)

func NewOcrRpcClient(rc RabbitConfig) (*OcrRpcClient, error) {
	ocrRpcClient := &OcrRpcClient{
		rabbitConfig: rc,
	}
	return ocrRpcClient, nil
}

// DecodeImage is the main function to do a ocr on incoming request.
// It's handling the parameter and the whole workflow
func (c *OcrRpcClient) DecodeImage(ocrRequest OcrRequest, requestID string) (OcrResult, error) {
	var err error

	logger := zerolog.New(os.Stdout).With().
		Str("component", "OCR_CLIENT").
		Uint("Timeout", ocrRequest.TimeOut).
		Str("RequestID", requestID).Timestamp().Logger()

	logger.Info().Bool("Deferred", ocrRequest.Deferred).
		Str("DocType", ocrRequest.DocType).
		Interface("EngineArgs", ocrRequest.EngineArgs).
		Bool("InplaceDecode", ocrRequest.InplaceDecode).
		Uint16("PageNumber", ocrRequest.PageNumber).
		Str("ReplyTo", ocrRequest.ReplyTo).
		Str("UserAgent", ocrRequest.UserAgent).
		Str("EngineType", string(ocrRequest.EngineType)).
		Str("ReferenceID", ocrRequest.ReferenceID).
		Msg("incoming request")

	if ocrRequest.ReplyTo != "" {
		logger.Info().Msg("Automated response requested")
		validURL, err := checkURLForReplyTo(ocrRequest.ReplyTo)
		if err != nil {
			return OcrResult{}, err
		}
		ocrRequest.ReplyTo = validURL
		// force set the deferred flag to drop the connection and deliver
		// ocr automatically to the URL in ReplyTo tag
		ocrRequest.Deferred = true
	}

	var messagePriority uint8 = 1
	if ocrRequest.DocType != "" {
		logger.Info().Str("DocType", ocrRequest.DocType).
			Msg("message type is specified, check for higher priority request")
		// set highest priority for defined message id
		logger.Debug().Interface("doc_types_available", c.rabbitConfig.QueuePrio)
		if val, ok := c.rabbitConfig.QueuePrio[ocrRequest.DocType]; ok {
			messagePriority = val
		} else {
			messagePriority = c.rabbitConfig.QueuePrio["standard"]
		}
	}
	// setting the timeout for worker if not set or to high
	if ocrRequest.TimeOut >= c.rabbitConfig.MaximalResponseCacheTimeout || ocrRequest.TimeOut == 0 {
		ocrRequest.TimeOut = c.rabbitConfig.ResponseCacheTimeout
	}

	// setting rabbitMQ correlation ID. There is no reason to be different from requestID
	correlationUUID := requestID
	urlToLog, _ := url.Parse(c.rabbitConfig.AmqpURI)
	logger.Info().Str("DocType", ocrRequest.DocType).
		Str("AmqpURI", urlToLog.Scheme+"://"+urlToLog.Host+urlToLog.Path).
		Msg("dialing RabbitMQ")

	c.connection, err = amqp.Dial(c.rabbitConfig.AmqpURI)
	if err != nil {
		return OcrResult{Text: "Internal Server Error: message broker is not reachable", Status: "error"}, err
	}
	// if we close the connection here, the deferred status wont get the ocr result
	// and will be always returning "processing"
	// defer c.connection.Close()

	c.channel, err = c.connection.Channel()
	if err != nil {
		return OcrResult{}, err
	}

	if err := c.channel.ExchangeDeclare(
		c.rabbitConfig.Exchange,     // name
		c.rabbitConfig.ExchangeType, // type
		true,                        // durable
		false,                       // auto-deleted
		false,                       // internal
		false,                       // noWait
		nil,                         // arguments
	); err != nil {
		return OcrResult{}, err
	}

	rpcResponseChan := make(chan OcrResult)

	callbackQueue, err := c.subscribeCallbackQueue(correlationUUID, rpcResponseChan)
	if err != nil {
		return OcrResult{}, err
	}

	// Reliable publisher confirms require confirm.select support from the
	// connection.
	if c.rabbitConfig.Reliable {
		if err := c.channel.Confirm(false); err != nil {
			return OcrResult{}, err
		}

		ack, nack := c.channel.NotifyConfirm(make(chan uint64, 1), make(chan uint64, 1))

		defer confirmDelivery(ack, nack)
	}

	// TODO: we only need to download image urlToLog if there are
	// any preprocessors.  if rabbitmq isn't in same data center
	// as open-ocr, it will be expensive in terms of bandwidth
	// to have image binary in messages
	if ocrRequest.ImgBytes == nil {

		// if we do not have bytes use base 64 file by converting it to bytes
		if ocrRequest.hasBase64() {
			logger.Info().Msg("OCR request has base 64 convert it to bytes")

			err = ocrRequest.decodeBase64()
			if err != nil {
				logger.Warn().Err(err).Msg("Error decoding base64")
				return OcrResult{}, err
			}
		} else {
			// if we do not have base 64 or bytes download the file
			err = ocrRequest.downloadImgUrl()
			if err != nil {
				logger.Warn().Err(err).Msg("Error downloading img urlToLog")
				return OcrResult{}, err
			}
		}
	}

	routingKey := ocrRequest.nextPreprocessor(c.rabbitConfig.RoutingKey)
	logger.Info().Str("routingKey", routingKey).Msg("publishing with routing key")

	ocrRequestJson, err := json.Marshal(ocrRequest)
	if err != nil {
		return OcrResult{}, err
	}
	if err = c.channel.Publish(
		c.rabbitConfig.Exchange, // publish to an exchange
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "application/json",
			ContentEncoding: "",
			Body:            []byte(ocrRequestJson),
			DeliveryMode:    amqp.Transient,  // 1=non-persistent, 2=persistent
			Priority:        messagePriority, // 0-9
			ReplyTo:         callbackQueue.Name,
			CorrelationId:   correlationUUID,
			// a bunch of application/implementation-specific fields
		},
	); err != nil {
		return OcrResult{}, nil
	}
	// TODO rewrite postClient to not check the status, just give it an ocrRequest of file
	// TODO rewrite it also check if there are memory leak after global timeout
	// TODO on deffered request if you get request by polling before it was
	// TODO automaticaly delivered then atomatic deliver will POST empty request back after timeout
	if ocrRequest.Deferred {
		logger.Info().Msg("Asynchronous request accepted")
		timer := time.NewTimer(time.Duration(c.rabbitConfig.ResponseCacheTimeout) * time.Second)
		logger.Debug().Msg("locking vrequestsAndTimersMu")
		requestsAndTimersMu.RLock()
		Requests[requestID] = rpcResponseChan
		timers[requestID] = timer
		logger.Debug().Msg("unlocking vrequestsAndTimersMu")
		requestsAndTimersMu.RUnlock()
		// deferred == true but no automatic reply to the requester
		// client should poll to get the ocr
		if ocrRequest.ReplyTo == "" {
			// thi go routine will cancel the request after global timeout if client stopped polling
			go func() {
				<-timer.C
				_, _ = CheckOcrStatusByID(requestID, false)
			}()
			return OcrResult{
				ID:     requestID,
				Status: "processing",
			}, nil
		}
		// automatic delivery oder POST to the requester
		// check interval for order to be ready to deliver
		tickerWithPostAction := time.NewTicker(c.rabbitConfig.tickerWithPostActionInterval)

		go func() {
		T:
			for {
				select {
				case t := <-tickerWithPostAction.C:
					logger.Debug().Str("time", t.String()).Msg("checking for request to be done")
					ocrRes, err := CheckOcrStatusByID(requestID, false)
					if err != nil {
						logger.Error().Err(err)
					} // only if status is done end the goroutine. otherwise continue polling
					if ocrRes.Status == "done" || ocrRes.Status == "error" {
						logger.Info().Msg("request is ready")

						var tryCounter uint8 = 1
						ocrPostClient := newOcrPostClient()
						for ok := true; ok; ok = tryCounter <= numRetries {
							err = ocrPostClient.postOcrRequest(&ocrRes, ocrRequest.ReplyTo, tryCounter)
							if err != nil {
								tryCounter++
								logger.Error().Err(err)
								time.Sleep(2 * time.Second)
							} else {
								break
							}
						}
						tickerWithPostAction.Stop()
						break T
					}
				}
			}
		}()
		// initial response to the caller to inform it with request id
		return OcrResult{
			ID:     requestID,
			Status: "processing",
		}, nil
	} else {
		// in case it works, error checking is needed
		return CheckOcrStatusByID(requestID, false)
	}
}

func (c OcrRpcClient) subscribeCallbackQueue(correlationUUID string, rpcResponseChan chan OcrResult) (amqp.Queue, error) {

	queueArgs := make(amqp.Table)
	queueArgs["x-max-priority"] = uint8(10)

	// declare a callback queue where we will receive rpc responses
	callbackQueue, err := c.channel.QueueDeclare(
		"",        // name -- let rabbit generate a random one
		false,     // durable
		true,      // delete when unused
		true,      // exclusive
		false,     // noWait
		queueArgs, // arguments
	)
	if err != nil {
		return amqp.Queue{}, err
	}

	// bind the callback queue to an exchange + routing key
	if err = c.channel.QueueBind(
		callbackQueue.Name,      // name of the queue
		callbackQueue.Name,      // bindingKey
		c.rabbitConfig.Exchange, // sourceExchange
		false,                   // noWait
		queueArgs,               // arguments
	); err != nil {
		return amqp.Queue{}, err
	}

	log.Info().Str("component", "OCR_CLIENT").Str("callbackQueue", callbackQueue.Name)

	deliveries, err := c.channel.Consume(
		callbackQueue.Name, // name
		tag,                // consumerTag,
		true,               // noAck
		true,               // exclusive
		false,              // noLocal
		false,              // noWait
		queueArgs,          // arguments
	)
	if err != nil {
		return amqp.Queue{}, err
	}

	go c.handleRpcResponse(deliveries, correlationUUID, rpcResponseChan)

	return callbackQueue, nil

}

func (c OcrRpcClient) handleRpcResponse(deliveries <-chan amqp.Delivery, correlationUuid string, rpcResponseChan chan OcrResult) {
	// correlationUuid is the same as RequestID
	logger := zerolog.New(os.Stdout).With().
		Str("component", "OCR_CLIENT").Str("RequestID", correlationUuid).Timestamp().Logger()
	logger.Info().Msg("looping over deliveries...:")

	for d := range deliveries {
		if d.CorrelationId == correlationUuid {
			bodyLenToLog := len(d.Body)
			defer c.connection.Close()
			if bodyLenToLog > 32 {
				bodyLenToLog = 32
			}
			logger.Info().Int("size", len(d.Body)).Uint64("DeliveryTag", d.DeliveryTag).
				Str("payload(32 Bytes)", string(d.Body[0:bodyLenToLog])).
				Str("ReplyTo", d.ReplyTo).
				Msg("got delivery")

			ocrResult := OcrResult{}
			err := json.Unmarshal(d.Body, &ocrResult)
			if err != nil {
				msg := "Error unmarshalling json: %v.  Error: %v"
				errMsg := fmt.Sprintf(msg, string(d.Body[0:bodyLenToLog]), err)
				logger.Error().Err(fmt.Errorf(errMsg))
			}
			ocrResult.ID = correlationUuid

			logger.Info().Msg("send result to rpcResponseChan")
			rpcResponseChan <- ocrResult
			logger.Info().Msg("sent result to rpcResponseChan")
			return

		} else {
			logger.Info().Str("CorrelationId", d.CorrelationId).
				Msg("ignoring delivery w/ correlation id")
		}
	}
}

// CheckOcrStatusByID checks status of an ocr request based on origin of request
func CheckOcrStatusByID(requestID string, httpStatusCheck bool) (OcrResult, error) {
	log.Debug().Str("component", "OCR_CLIENT").Msg("CheckOcrStatusByID called")
	requestsAndTimersMu.RLock()
	if _, ok := Requests[requestID]; !ok {
		requestsAndTimersMu.RUnlock()
		return OcrResult{}, fmt.Errorf("no such request %s", requestID)
	} else if ok && httpStatusCheck {
		return OcrResult{Status: "processing", ID: requestID}, nil
	}

	log.Debug().Str("component", "OCR_CLIENT").Msg("getting ocrResult := <-Requests[requestID]")
	ocrResult := OcrResult{}
	requestsAndTimersMu.RLock()
	select {
	case ocrResult = <-Requests[requestID]:
		log.Debug().Str("component", "OCR_CLIENT").Msg("got ocrResult := <-Requests[requestID]")
	default:
		sampled := log.Sample(&zerolog.BasicSampler{N: 10})
		sampled.Debug().Str("component", "OCR_CLIENT").
			Msg("Number of messages in the queue:" + fmt.Sprintf("%v", len(Requests)))
	}
	requestsAndTimersMu.RUnlock()

	if ocrResult.Status != "processing" && ocrResult.ID != "" {
		log.Debug().Str("component", "OCR_CLIENT").Msg("deleting from Requests and timers")
		requestsAndTimersMu.RLock()
		delete(Requests, requestID)
		timers[requestID].Stop()
		delete(timers, requestID)
		requestsAndTimersMu.RUnlock()
	}
	return ocrResult, nil
}

func confirmDelivery(ack, nack chan uint64) {
	select {
	case tag := <-ack:
		log.Info().Str("component", "OCR_CLIENT").
			Uint64("tag", tag).
			Msg("confirmed delivery with tag")
	case tag := <-nack:
		log.Info().Str("component", "OCR_CLIENT").
			Uint64("tag", tag).
			Msg("failed to confirm delivery")
	}
}
