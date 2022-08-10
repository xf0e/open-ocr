package ocrworker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	amqp "github.com/rabbitmq/amqp091-go"
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

var numRetries uint = 3

func NewOcrRpcClient(rc *RabbitConfig) (*OcrRpcClient, error) {
	ocrRpcClient := &OcrRpcClient{
		rabbitConfig: *rc,
	}
	return ocrRpcClient, nil
}

// DecodeImage is the main function to do an ocr on incoming request.
// It's handling the parameter and the whole workflow
func (c *OcrRpcClient) DecodeImage(ocrRequest *OcrRequest) (OcrResult, int, error) {
	var err error

	logger := zerolog.New(os.Stdout).With().
		Str("component", "OCR_CLIENT").
		Uint("Timeout", ocrRequest.TimeOut).
		Str("RequestID", ocrRequest.RequestID).Timestamp().Logger()

	logger.Info().Bool("Deferred", ocrRequest.Deferred).
		Str("DocType", ocrRequest.DocType).
		Interface("EngineArgs", ocrRequest.EngineArgs).
		Bool("InplaceDecode", ocrRequest.InplaceDecode).
		Uint16("PageNumber", ocrRequest.PageNumber).
		Str("ReplyTo", ocrRequest.ReplyTo).
		Str("UserAgent", ocrRequest.UserAgent).
		Str("EngineType", ocrRequest.EngineType.String()).
		Str("ReferenceID", ocrRequest.ReferenceID).
		Msg("incoming request")

	if ocrRequest.ReplyTo != "" {
		logger.Info().Msg("Automated response requested")
		validURL, err := checkURLForReplyTo(ocrRequest.ReplyTo)
		if err != nil {
			return OcrResult{}, 400, err
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
	correlationID := ocrRequest.RequestID
	urlToLog, _ := url.Parse(c.rabbitConfig.AmqpURI)
	logger.Info().Str("DocType", ocrRequest.DocType).
		Str("AmqpURI", urlToLog.Scheme+"://"+urlToLog.Host+urlToLog.Path).
		Msg("dialing RabbitMQ")

	c.connection, err = amqp.Dial(c.rabbitConfig.AmqpURI)
	if err != nil {
		return OcrResult{Text: "Internal Server Error: message broker is not reachable", Status: "error"}, 500, err
	}
	// if we close the connection here, the deferred status won't get the ocr result
	// and will be always returning "processing"
	// defer c.connection.Close()

	c.channel, err = c.connection.Channel()
	if err != nil {
		return OcrResult{}, 500, err
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
		return OcrResult{}, 500, err
	}

	rpcResponseChan := make(chan OcrResult, c.rabbitConfig.FactorForMessageAccept)

	callbackQueue, err := c.subscribeCallbackQueue(correlationID, rpcResponseChan)
	if err != nil {
		return OcrResult{}, 500, err
	}

	// Reliable publisher confirms require confirm.select support from the
	// connection.
	if c.rabbitConfig.Reliable {
		if err := c.channel.Confirm(false); err != nil {
			return OcrResult{}, 500, err
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
				return OcrResult{}, 500, err
			}
		} else {
			// if we do not have base 64 or bytes download the file
			err = ocrRequest.downloadImgUrl()
			if err != nil {
				logger.Warn().Err(err).Msg("Error downloading img urlToLog")
				return OcrResult{}, 500, err
			}
		}
	}

	routingKey := ocrRequest.nextPreprocessor(c.rabbitConfig.RoutingKey)
	logger.Info().Str("routingKey", routingKey).Msg("publishing with routing key")

	ocrRequestJson, err := json.Marshal(ocrRequest)
	if err != nil {
		return OcrResult{}, 500, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err = c.channel.PublishWithContext(
		ctx,
		c.rabbitConfig.Exchange, // publish to an exchange
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "application/json",
			ContentEncoding: "",
			Body:            ocrRequestJson,
			DeliveryMode:    amqp.Transient,  // 1=non-persistent, 2=persistent
			Priority:        messagePriority, // 0-9
			ReplyTo:         callbackQueue.Name,
			CorrelationId:   correlationID,
			// a bunch of application/implementation-specific fields
		},
	); err != nil {
		return OcrResult{}, 500, nil
	}

	if ocrRequest.Deferred {
		logger.Info().Msg("Asynchronous request accepted")

		addNewOcrResultToQueue(ocrRequest.RequestID, rpcResponseChan)
		// deferred == true but no automatic reply to the requester
		// client should poll to get the ocr
		if ocrRequest.ReplyTo == "" {
			// this go routine will cancel the request after global timeout or if requester doesn't retrieve the request
			logger.Info().Msg("deferred request without reply-to address set, will decay automatically after " + strconv.FormatUint(uint64(ocrRequest.TimeOut), 10) + " seconds")
			go func() {
				timeout := time.After(time.Second * time.Duration(ocrRequest.TimeOut+10))
			Loop:
				for {
					select {
					case <-timeout:
						if _, ok := RequestsTrack.Load(ocrRequest.RequestID); ok {
							deleteRequestFromQueue(ocrRequest.RequestID)
							logger.Info().Msg("deferred request without reply-to address has decayed, client doesn't claimed request in time")
							break Loop
						}
					default:
						if _, ok := RequestsTrack.Load(ocrRequest.RequestID); !ok {
							break Loop
						}
					}
					time.Sleep(5 * time.Second)
				}
			}()
			return OcrResult{
				Status: "processing",
				ID:     ocrRequest.RequestID,
			}, 200, nil
		}
		// automatic delivery POST to the requester
		// check interval for order to be ready to deliver
		go func(requestID string) {
			// trigger deleting request from internal queue
			defer func() {
				//	if _, ok := RequestsTrack.Load(ocrRequest.RequestID); ok {
				//		deleteRequestFromQueue(ocrRequest.RequestID)
				logger.Info().Msg("request handling finished, deleting it from the queue")
				deleteRequestFromQueue(requestID)
				//	}

			}()
			ocrRes := OcrResult{ID: ocrRequest.RequestID, Status: "error", Text: ""}
			ocrPostClient := newOcrPostClient()
			var tryCounter uint = 1
		T:
			for {
				select {
				case ocrResult := <-rpcResponseChan:
					logger.Info().Msg("request is ready for sending back")
					ocrRes = ocrResult
					for ok := true; ok; ok = tryCounter <= numRetries {
						err = ocrPostClient.postOcrRequest(&ocrRes, ocrRequest.ReplyTo, tryCounter)
						if err != nil {
							logger.Info().Uint("delivery_attempt", tryCounter).Msg("delivery attempt " +
								strconv.FormatUint(uint64(tryCounter), 10) + " was not successful, attempt " + strconv.FormatUint(uint64(tryCounter), 10) +
								"/" + strconv.FormatUint(uint64(numRetries), 10))
							tryCounter++
							logger.Error().Err(err)
							time.Sleep(2 * time.Second)
						} else {
							logger.Debug().Msg("delivery was successful")
							break T
						}
					}
					break T
				case <-time.After(rpcResponseTimeout * time.Second):
					err = ocrPostClient.postOcrRequest(&ocrRes, ocrRequest.ReplyTo, tryCounter)
					if err != nil {
						tryCounter++
						logger.Error().Err(err)
						time.Sleep(rpcResponseTimeout * time.Second)
					} else {
						break T
					}
					break T
				}
			}
		}(ocrRequest.RequestID)
		// initial response to the caller to inform it with request id
		return OcrResult{
			ID:     ocrRequest.RequestID,
			Status: "processing",
		}, 200, nil
	} else { // handle not deferred request
		select {
		case ocrResult := <-rpcResponseChan:
			// logger.Debug().Str("st", ocrResult.Status).Str("text", ocrResult.Text).Str("id", ocrResult.ID)
			return ocrResult, 200, nil
		case <-time.After(time.Duration(c.rabbitConfig.ResponseCacheTimeout) * time.Second):
			return OcrResult{}, 500, fmt.Errorf("timeout waiting for RPC response")
		}
	}
}

func (c *OcrRpcClient) subscribeCallbackQueue(correlationID string, rpcResponseChan chan OcrResult) (amqp.Queue, error) {
	queueArgs := make(amqp.Table)
	queueArgs["x-max-priority"] = uint8(10)

	// declare a callback queue where we will receive rpc responses
	callbackQueue, err := c.channel.QueueDeclare(
		correlationID, // set to correlationID aka requestID; empty name -- let rabbit generate a random one
		false,         // durable
		true,          // delete when unused
		true,          // exclusive
		false,         // noWait
		queueArgs,     // arguments
	)
	if err != nil {
		return amqp.Queue{}, err
	}

	// bind the callback queue to an exchange + routing key
	if err := c.channel.QueueBind(
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

	go c.handleRPCResponse(deliveries, correlationID, rpcResponseChan)

	return callbackQueue, nil
}

func (c *OcrRpcClient) handleRPCResponse(deliveries <-chan amqp.Delivery, correlationID string, rpcResponseChan chan OcrResult) {
	// correlationID is the same as RequestID
	logger := zerolog.New(os.Stdout).With().
		Str("component", "OCR_CLIENT").Str("RequestID", correlationID).Timestamp().Logger()
	logger.Info().Msg("looping over deliveries...:")

	logger.Debug().Int("deliveries", len(deliveries)).Msg("Number of elements in deliveries variable")
	for d := range deliveries {
		logger.Debug().Str("deliveries", d.CorrelationId).Msg("looping over deliveries variable")
		if d.CorrelationId == correlationID {
			bodyLenToLog := len(d.Body)
			logger.Debug().Str("deliveries", d.CorrelationId).Msg("reached if in d.CorrelationId == correlationID")
			defer func(connection *amqp.Connection) {
				err := connection.Close()
				if err != nil {
					logger.Warn().Err(err).Msg("Could not close RabbitMq connection ")
				}
			}(c.connection)
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
			ocrResult.ID = correlationID

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
