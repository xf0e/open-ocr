package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/couchbaselabs/logg"
	"github.com/nu7hatch/gouuid"
	"github.com/streadway/amqp"
	"time"
)

const (
	RpcResponseTimeout   = time.Minute * 60
	ResponseCacheTimeout = time.Minute * 60
)

type OcrRpcClient struct {
	rabbitConfig RabbitConfig
	connection   *amqp.Connection
	channel      *amqp.Channel
}

type OcrResult struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

var requests = make(map[string]chan OcrResult)
var timers = make(map[string]*time.Timer)

func NewOcrRpcClient(rc RabbitConfig) (*OcrRpcClient, error) {
	ocrRpcClient := &OcrRpcClient{
		rabbitConfig: rc,
	}
	return ocrRpcClient, nil
}

func (c *OcrRpcClient) DecodeImage(ocrRequest OcrRequest) (OcrResult, error) {
	var err error

	correlationUuidRaw, err := uuid.NewV4()
	if err != nil {
		return OcrResult{}, err
	}
	correlationUuid := correlationUuidRaw.String()

	logg.LogTo("OCR_CLIENT", "dialing %q", c.rabbitConfig.AmqpURI)
	c.connection, err = amqp.Dial(c.rabbitConfig.AmqpURI)
	if err != nil {
		return OcrResult{}, err
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

	rpcResponseChan := make(chan OcrResult, 1)

	callbackQueue, err := c.subscribeCallbackQueue(correlationUuid, rpcResponseChan)
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

	// TODO: we only need to download image url if there are
	// any preprocessors.  if rabbitmq isn't in same data center
	// as open-ocr, it will be expensive in terms of bandwidth
	// to have image binary in messages
	if ocrRequest.ImgBytes == nil {

		// if we do not have bytes use base 64 file by converting it to bytes
		if ocrRequest.hasBase64() {

			logg.LogTo("OCR_CLIENT", "OCR request has base 64 convert it to bytes")

			err = ocrRequest.decodeBase64()
			if err != nil {
				logg.LogTo("OCR_CLIENT", "Error decoding base64: %v", err)
				return OcrResult{}, err
			}
		} else {
			// if we do not have base 64 or bytes download the file
			err = ocrRequest.downloadImgUrl()
			if err != nil {
				logg.LogTo("OCR_CLIENT", "Error downloading img url: %v", err)
				return OcrResult{}, err
			}
		}
	}

	logg.LogTo("OCR_CLIENT", "ocrRequest before: %v", ocrRequest)
	routingKey := ocrRequest.nextPreprocessor(c.rabbitConfig.RoutingKey)
	logg.LogTo("OCR_CLIENT", "publishing with routing key %q", routingKey)
	logg.LogTo("OCR_CLIENT", "ocrRequest after: %v", ocrRequest)

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
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			ReplyTo:         callbackQueue.Name,
			CorrelationId:   correlationUuid,
			// a bunch of application/implementation-specific fields
		},
	); err != nil {
		return OcrResult{}, nil
	}

	if ocrRequest.Deferred {
		logg.LogTo("OCR_CLIENT", "Distributed request")
		requestId, _ := uuid.NewV4()
		timer := time.NewTimer(ResponseCacheTimeout)
		requests[requestId.String()] = rpcResponseChan
		timers[requestId.String()] = timer
		go func() {
			<-timer.C
			CheckOcrStatusById(requestId.String())
		}()
		return OcrResult{
			Text: requestId.String(),
		}, nil
	} else {
		return CheckReply(rpcResponseChan, RpcResponseTimeout)
	}
}

func (c OcrRpcClient) subscribeCallbackQueue(correlationUuid string, rpcResponseChan chan OcrResult) (amqp.Queue, error) {

	// declare a callback queue where we will receive rpc responses
	callbackQueue, err := c.channel.QueueDeclare(
		"",    // name -- let rabbit generate a random one
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // noWait
		nil,   // arguments
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
		nil,                     // arguments
	); err != nil {
		return amqp.Queue{}, err
	}

	logg.LogTo("OCR_CLIENT", "callbackQueue name: %v", callbackQueue.Name)

	deliveries, err := c.channel.Consume(
		callbackQueue.Name, // name
		tag,                // consumerTag,
		true,               // noAck
		true,               // exclusive
		false,              // noLocal
		false,              // noWait
		nil,                // arguments
	)
	if err != nil {
		return amqp.Queue{}, err
	}

	go c.handleRpcResponse(deliveries, correlationUuid, rpcResponseChan)

	return callbackQueue, nil

}

func (c OcrRpcClient) handleRpcResponse(deliveries <-chan amqp.Delivery, correlationUuid string, rpcResponseChan chan OcrResult) {
	logg.LogTo("OCR_CLIENT", "looping over deliveries..")
	// TODO this defer is probably a memory leak
	// defer c.connection.Close()
	for d := range deliveries {
		if d.CorrelationId == correlationUuid {
			defer c.connection.Close()
			logg.LogTo(
				"OCR_CLIENT",
				"got %dB delivery(first 64 Bytes): [%v] %q.  Reply to: %v",
				len(d.Body),
				d.DeliveryTag,
				d.Body[0:64],
				d.ReplyTo,
			)

			ocrResult := OcrResult{
				Text: string(d.Body),
			}

			logg.LogTo("OCR_CLIENT", "send result to rpcResponseChan")
			rpcResponseChan <- ocrResult
			logg.LogTo("OCR_CLIENT", "sent result to rpcResponseChan")

			return

		} else {
			logg.LogTo("OCR_CLIENT", "ignoring delivery w/ correlation id: %v", d.CorrelationId)
		}

	}
}

func CheckOcrStatusById(requestId string) (OcrResult, error) {
	if _, ok := requests[requestId]; !ok {
		return OcrResult{}, fmt.Errorf("no such request %s", requestId)
	}
	ocrResult, err := CheckReply(requests[requestId], time.Second*2)
	if ocrResult.Status != "processing" {
		close(requests[requestId])
		delete(requests, requestId)
		timers[requestId].Stop()
		delete(timers, requestId)
	}
	return ocrResult, err
}

func CheckReply(rpcResponseChan chan OcrResult, timeout time.Duration) (OcrResult, error) {
	logg.LogTo("OCR_CLIENT", "Checking for response")
	select {
	case ocrResult := <-rpcResponseChan:
		return ocrResult, nil
	case <-time.After(timeout):
		return OcrResult{Text: "Timeout waiting for RPC response", Status: "processing"}, nil
	}
}

func confirmDelivery(ack, nack chan uint64) {
	select {
	case tag := <-ack:
		logg.LogTo("OCR_CLIENT", "confirmed delivery, tag: %v", tag)
	case tag := <-nack:
		logg.LogTo("OCR_CLIENT", "failed to confirm delivery: %v", tag)
	}
}
