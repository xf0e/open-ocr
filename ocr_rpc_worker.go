package ocrworker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"
)

type OcrRpcWorker struct {
	workerConfig WorkerConfig
	conn         *amqp.Connection
	channel      *amqp.Channel
	tag          string
	Done         chan error
}

// tag is based on K-Sortable Globally Unique IDs
var tag = ksuid.New().String()

// NewOcrRpcWorker is needed to establish a connection to a message broker
func NewOcrRpcWorker(wc *WorkerConfig) (*OcrRpcWorker, error) {
	ocrRpcWorker := &OcrRpcWorker{
		workerConfig: *wc,
		conn:         nil,
		channel:      nil,
		tag:          tag,
		Done:         make(chan error),
	}
	return ocrRpcWorker, nil
}

func (w *OcrRpcWorker) Run() error {
	var err error
	queueArgs := make(amqp.Table)
	queueArgs["x-max-priority"] = uint8(9)

	log.Debug().
		Str("component", "OCR_WORKER").
		Str("tag", tag).
		Msg("Run() called...")

	urlToLog, _ := url.Parse(w.workerConfig.AmqpURI)
	log.Info().
		Str("component", "OCR_WORKER").
		Str("tag", tag).
		Str("amqp", urlToLog.Scheme+"://"+urlToLog.Host+urlToLog.Path).
		Msg("dialing rabbitMQ")

	w.conn, err = amqp.Dial(w.workerConfig.AmqpURI)
	if err != nil {
		log.Warn().
			Str("component", "OCR_WORKER").
			Err(err).
			Str("tag", tag).
			Msg("error connecting to rabbitMQ")
		return err
	}

	go func() {
		fmt.Printf("closing: %s", <-w.conn.NotifyClose(make(chan *amqp.Error)))
	}()

	log.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Msg("got Connection, getting channel")
	w.channel, err = w.conn.Channel()
	if err != nil {
		return err
	}
	// setting the prefetchCount to 1 reduces the Memory Consumption by the worker
	err = w.channel.Qos(int(w.workerConfig.NumParallelJobs), 0, true)
	if err != nil {
		return err
	}

	if err := w.channel.ExchangeDeclare(
		w.workerConfig.Exchange,     // name of the exchange
		w.workerConfig.ExchangeType, // type
		true,                        // durable
		false,                       // delete when complete
		false,                       // internal
		false,                       // noWait
		nil,                         // arguments
	); err != nil {
		return err
	}

	// just use the routing key as the queue name, since there's no reason
	// to have a different name
	queueName := w.workerConfig.RoutingKey

	queue, err := w.channel.QueueDeclare(
		queueName, // name of the queue
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // noWait
		queueArgs, // arguments
	)
	if err != nil {
		return err
	}

	log.Info().Str("component", "OCR_WORKER").Str("RoutingKey", w.workerConfig.RoutingKey).
		Str("tag", tag).
		Msg("binding to routing key")

	if err := w.channel.QueueBind(
		queue.Name,                // name of the queue
		w.workerConfig.RoutingKey, // bindingKey
		w.workerConfig.Exchange,   // sourceExchange
		false,                     // noWait
		queueArgs,                 // arguments
	); err != nil {
		return err
	}

	log.Info().Str("component", "OCR_WORKER").Str("tag", tag).
		Msg("Queue bound to Exchange, starting Consume tag")
	deliveries, err := w.channel.Consume(
		queue.Name, // name
		tag,        // consumerTag,
		false,      // noAck
		false,      // exclusive
		false,      // noLocal
		false,      // noWait
		queueArgs,  // arguments
	)
	if err != nil {
		return err
	}

	go w.handle(deliveries, w.Done)

	return nil
}

func (w *OcrRpcWorker) Shutdown() error {
	// will close() the deliveries channel
	if err := w.channel.Cancel(w.tag, true); err != nil {
		return fmt.Errorf("worker with tag %s cancel failed: %s", tag, err)
	}

	if err := w.conn.Close(); err != nil {
		return fmt.Errorf("AMQP connection with worker %s close error: %s", tag, err)
	}

	defer log.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Msg("Shutdown OK")

	// wait for handle() to exit
	return <-w.Done
}

func (w *OcrRpcWorker) handle(deliveries <-chan amqp.Delivery, done chan error) {
	for d := range deliveries {
		log.Info().Str("component", "OCR_WORKER").
			Str("tag", tag).
			Int("msg_size", len(d.Body)).
			Uint8("DeliveryMode", d.DeliveryMode).
			Uint8("Priority", d.Priority).
			Str("RequestID", d.CorrelationId).
			Str("ReplyToQueue", d.ReplyTo).
			Str("ConsumerTag", d.ConsumerTag).
			Uint64("DeliveryTag", d.DeliveryTag).
			Str("Exchange", d.Exchange).
			Str("RoutingKey", d.RoutingKey).
			Msg("worker got delivery, starting processing")
		// reply from engine here
		// id is not set, Text is set, Status is set
		ocrResult, err := w.resultForDelivery(&d)
		if err != nil {
			log.Error().Err(err).Str("component", "OCR_WORKER").
				Str("RequestID", d.CorrelationId).
				Str("tag", tag).
				Msg("Error generating ocr result")
		}

		err = w.sendRpcResponse(ocrResult, d.ReplyTo, d.CorrelationId)
		if err != nil {
			log.Error().Err(err).Str("component", "OCR_WORKER").
				Str("RequestID", ocrResult.ID).
				Str("tag", tag).
				Msg("Error generating ocr result, sendRpcResponse failed")

			// if we can't send our response, let's just abort
			done <- err
			break
		}
		err = d.Ack(false)
		if err != nil {
			log.Warn().Str("component", "OCR_WORKER").Err(err).
				Str("tag", tag).
				Msg("Ack() was not successful")
		}

	}
	log.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Msg("handle: deliveries channel closed")
	done <- fmt.Errorf("handle: deliveries channel closed")
}

func (w *OcrRpcWorker) resultForDelivery(d *amqp.Delivery) (OcrResult, error) {
	ocrRequest := OcrRequest{}
	ocrResult := OcrResult{ID: d.CorrelationId}
	err := json.Unmarshal(d.Body, &ocrRequest)
	if err != nil {
		msg := "Error unmarshalling json: %v.  Error: %v"
		errMsg := fmt.Sprintf(msg, d.CorrelationId, err)
		log.Error().Err(err).Caller().
			Str("RequestID", ocrResult.ID).
			Str("tag", tag).
			Msg("error unmarshalling json delivery")
		ocrResult.Text = errMsg
		ocrResult.Status = "error"
		return ocrResult, err
	}

	ocrEngine := NewOcrEngine(ocrRequest.EngineType)
	ocrResult, err = ocrEngine.ProcessRequest(&ocrRequest, &w.workerConfig)
	if err != nil {
		msg := "Error processing image url: %v.  Error: %v"
		errMsg := fmt.Sprintf(msg, ocrRequest.RequestID, err)
		log.Error().Err(err).
			Str("RequestID", ocrRequest.RequestID).
			Str("tag", tag).
			Str("ImgUrl", ocrRequest.ImgUrl).
			Msg("Error processing image")

		ocrResult.Text = errMsg
		ocrResult.Status = "error"
		return ocrResult, err
	}

	return ocrResult, nil
}

func (w *OcrRpcWorker) sendRpcResponse(r OcrResult, replyTo, correlationId string) error {
	// RequestID is the same as correlationId
	logger := zerolog.New(os.Stdout).With().
		Str("RequestID", correlationId).Timestamp().Logger()

	if w.workerConfig.Reliable {
		// Do not use w.workerConfig.Reliable=true due to major issues
		// that will completely  wedge the rpc worker.  Setting the
		// buffered channels length higher would delay the problem,
		// but then it would still happen later.
		if err := w.channel.Confirm(false); err != nil {
			return err
		}

		ack, nack := w.channel.NotifyConfirm(make(chan uint64, 100), make(chan uint64, 100))

		defer confirmDeliveryWorker(ack, nack)
	}

	logger.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Str("replyTo", replyTo).Msg("sendRpcResponse to")
	// ocr worker is publishing back the decoded text
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := w.channel.PublishWithContext(
		ctx,
		w.workerConfig.Exchange, // publish to an exchange
		replyTo,                 // routing to 0 or more queues
		false,                   // mandatory
		false,                   // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			Body:            body,
			// Body:            []byte(r.Text),
			DeliveryMode:  amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:      0,              // 0-9
			CorrelationId: correlationId,
			// a bunch of application/implementation-specific fields
		},
	); err != nil {
		return err
	}
	logger.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Str("replyTo", replyTo).
		Msg("sendRpcResponse succeeded")
	return nil
}

func confirmDeliveryWorker(ack, nack chan uint64) {
	log.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Msg("awaiting delivery confirmation...")
	select {
	case tag := <-ack:
		log.Info().Str("component", "OCR_WORKER").Uint64("tag", tag).
			Msg("confirmed delivery")
	case tag := <-nack:
		log.Info().Str("component", "OCR_WORKER").Uint64("tag", tag).
			Msg("failed to confirm delivery")
	case <-time.After(rpcResponseTimeout):
		// this is bad, the worker will probably be dysfunctional
		// at this point, so panic
		log.Panic().Str("component", "OCR_WORKER").Msg("timeout trying to confirm delivery. Worker panic")
	}
}
