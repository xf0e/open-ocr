package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/couchbaselabs/logg"
	"github.com/streadway/amqp"
	"sync"
	"time"
)

const (
	RPCResponseTimeout   = time.Minute * 1
	ResponseCacheTimeout = time.Minute * 1
)

type OcrRpcClient struct {
	rabbitConfig RabbitConfig
	connection   *amqp.Connection
	channel      *amqp.Channel
}

type OcrResult struct {
	Text   string `json:"text"`
	Status string `json:"status"`
	Id     string `json:"id"`
}

func NewOcrResult(id string) OcrResult {
	ocrResult := &OcrResult{}
	ocrResult.Status = "processing"
	ocrResult.Id = id
	return *ocrResult
}

var (
	requestsAndTimersMu sync.Mutex
	requests            = make(map[string]chan OcrResult)
	timers              = make(map[string]*time.Timer)
)

func NewOcrRpcClient(rc RabbitConfig) (*OcrRpcClient, error) {
	ocrRpcClient := &OcrRpcClient{
		rabbitConfig: rc,
	}
	return ocrRpcClient, nil
}

func (c *OcrRpcClient) DecodeImage(ocrRequest OcrRequest, requestID string) (OcrResult, error) {
	var err error

	if ocrRequest.ReplyTo != "" {
		logg.LogTo("OCR_CLIENT", "Automated response requested")
		validURL, err := checkUrlForReplyTo(ocrRequest.ReplyTo)
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
		logg.LogTo("OCR_CLIENT", "Message with higher priority requested: %s", ocrRequest.DocType)
		// set highest priority for defined message id
		// TODO do not hardcode DocType priority
		if ocrRequest.DocType == "egvp" {
			messagePriority = 9
		}

	}

	correlationUuid := requestID
	logg.LogTo("OCR_CLIENT", "dialing %q", c.rabbitConfig.AmqpURI)
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
			DeliveryMode:    amqp.Transient,  // 1=non-persistent, 2=persistent
			Priority:        messagePriority, // 0-9
			ReplyTo:         callbackQueue.Name,
			CorrelationId:   correlationUuid,
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
		logg.LogTo("OCR_CLIENT", "Asynchronous request accepted")
		timer := time.NewTimer(ResponseCacheTimeout)
		requestsAndTimersMu.Lock()
		requests[requestID] = rpcResponseChan
		timers[requestID] = timer
		requestsAndTimersMu.Unlock()
		// deferred == true but no automatic reply to the requester
		// client should poll to get the ocr
		if ocrRequest.ReplyTo == "" {
			go func() {
				<-timer.C
				CheckOcrStatusByID(requestID)
			}()
			return OcrResult{
				Id:     requestID,
				Status: "processing",
			}, nil
		} else { // automatic delivery oder POST to the requester
			timerWithPostAction := time.NewTimer(time.Second * 30)
			// check interval for order to be ready to deliver
			tickerWithPostAction := time.NewTicker(time.Second * 2)
			done := make(chan bool, 1)
			go func() {
				<-timerWithPostAction.C
				done <- true
			}()
			go func() {
			T:
				for {
					select {
					case <-done:
						tickerWithPostAction.Stop()
						fmt.Println("Request processing took too long, aborting")
						// TODO DELETE request by timeout, check for cleanup, check for faster reply. Set done status
						ocrPostClient := NewOcrPostClient()
						err := ocrPostClient.postOcrRequest(requestID, ocrRequest.ReplyTo)
						if err != nil {
							logg.LogError(err)
						}
						break T
					case t := <-tickerWithPostAction.C:
						logg.LogTo("OCR_CLIENT", "checking for request %s to be done %s", requestID, t)
						ocrRes, err := CheckOcrStatusByID(requestID)
						if err != nil {
							logg.LogError(err)
						} // only if status is done end the goroutine. otherwise continue polling
						if ocrRes.Status == "done" {
							logg.LogTo("OCR_CLIENT", "request %s is ready", requestID)
							ocrPostClient := NewOcrPostClient()
							err := ocrPostClient.postOcrRequest(requestID, ocrRequest.ReplyTo)
							if err != nil {
								logg.LogError(err)
							}
							tickerWithPostAction.Stop()
							break T
						}
					}
				}
			}()
		} // initial response to the caller to inform it with request id
		return OcrResult{
			Id:     requestID,
			Status: "processing",
		}, nil
	} else {
		return CheckReply(rpcResponseChan, RPCResponseTimeout)
	}
}

func (c OcrRpcClient) subscribeCallbackQueue(correlationUuid string, rpcResponseChan chan OcrResult) (amqp.Queue, error) {

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

	logg.LogTo("OCR_CLIENT", "callbackQueue name: %v", callbackQueue.Name)

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

	go c.handleRpcResponse(deliveries, correlationUuid, rpcResponseChan)

	return callbackQueue, nil

}

func (c OcrRpcClient) handleRpcResponse(deliveries <-chan amqp.Delivery, correlationUuid string, rpcResponseChan chan OcrResult) {
	logg.LogTo("OCR_CLIENT", "looping over deliveries..")
	// TODO this defer is probably a memory leak
	// defer c.connection.Close()
	for d := range deliveries {
		if d.CorrelationId == correlationUuid {
			bodyLenToLog := len(d.Body)
			defer c.connection.Close()
			if bodyLenToLog > 32 {
				bodyLenToLog = 32
			}
			logg.LogTo(
				"OCR_CLIENT",
				"got %dB delivery(first 32 bytes): [%v] %q.  Reply to: %v",
				len(d.Body),
				d.DeliveryTag,
				d.Body[0:bodyLenToLog],
				d.ReplyTo,
			)
			// ocrResult := OcrResult{
			// 	Text: string(d.Body),
			// }+
			// TODO check if additional transcoding ocrResult > JSON > ocrResult is needed
			ocrResult := OcrResult{}
			err := json.Unmarshal(d.Body, &ocrResult)
			if err != nil {
				msg := "Error unmarshaling json: %v.  Error: %v"
				errMsg := fmt.Sprintf(msg, string(d.Body[0:bodyLenToLog]), err)
				logg.LogError(fmt.Errorf(errMsg))
			}
			ocrResult.Id = correlationUuid

			logg.LogTo("OCR_CLIENT", "send result to rpcResponseChan")
			// TODO: on request which is beyond of timeout chanel is closed and the cli_http panics
			rpcResponseChan <- ocrResult
			logg.LogTo("OCR_CLIENT", "sent result to rpcResponseChan")

			return

		} else {
			logg.LogTo("OCR_CLIENT", "ignoring delivery w/ correlation id: %v", d.CorrelationId)
		}

	}
}

func CheckOcrStatusByID(requestID string) (OcrResult, error) {
	requestsAndTimersMu.Lock()
	if _, ok := requests[requestID]; !ok {
		requestsAndTimersMu.Unlock()
		return OcrResult{}, fmt.Errorf("no such request %s", requestID)
	}
	ocrResult, err := CheckReply(requests[requestID], time.Second*2)
	if ocrResult.Status != "processing" {
		// TODO race condition on requests, here we need to lock the requests map
		close(requests[requestID])
		delete(requests, requestID)
		timers[requestID].Stop()
		delete(timers, requestID)
	}
	requestsAndTimersMu.Unlock()
	return ocrResult, err
}

// CheckReply checks the status of deferred request and reply to the requester with
// status or, if done, with orc text
func CheckReply(rpcResponseChan chan OcrResult, timeout time.Duration) (OcrResult, error) {
	logg.LogTo("OCR_CLIENT", "Checking for response")
	select {
	case ocrResult := <-rpcResponseChan:
		return ocrResult, nil
	case <-time.After(timeout):
		return OcrResult{Text: "Timeout waiting for RPC response", Status: "error"}, nil
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
