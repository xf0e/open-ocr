package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"
	"github.com/streadway/amqp"
	"time"
)

type OcrRpcWorker struct {
	rabbitConfig RabbitConfig
	conn         *amqp.Connection
	channel      *amqp.Channel
	tag          string
	Done         chan error
}

var (
	// tag is based on ksuid K-Sortable Globally Unique IDs
	tag = ksuid.New().String()
)

func NewOcrRpcWorker(rc RabbitConfig) (*OcrRpcWorker, error) {
	ocrRpcWorker := &OcrRpcWorker{
		rabbitConfig: rc,
		conn:         nil,
		channel:      nil,
		tag:          tag,
		Done:         make(chan error),
	}
	return ocrRpcWorker, nil
}

func (w OcrRpcWorker) Run() error {

	var err error
	queueArgs := make(amqp.Table)
	queueArgs["x-max-priority"] = uint8(9)

	log.Info().
		Str("component", "OCR_WORKER").
		Str("tag", tag).
		Msg("Run() called...")

	log.Info().
		Str("component", "OCR_WORKER").
		Str("tag", tag).
		Str("host", w.rabbitConfig.AmqpURI).
		Msg("dialing rabbitMq")

	w.conn, err = amqp.Dial(w.rabbitConfig.AmqpURI)
	if err != nil {
		log.Warn().
			Str("component", "OCR_WORKER").
			Err(err).
			Str("tag", tag).
			Msg("error connecting to rabbitMq")
		return err
	}

	go func() {
		fmt.Printf("closing: %s", <-w.conn.NotifyClose(make(chan *amqp.Error)))
	}()

	log.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Msg("got Connection, getting channel")
	w.channel, err = w.conn.Channel()
	// setting the prefetchCount to 1 reduces the Memory Consumption by the worker
	err = w.channel.Qos(1, 0, true)
	if err != nil {
		return err
	}

	if err = w.channel.ExchangeDeclare(
		w.rabbitConfig.Exchange,     // name of the exchange
		w.rabbitConfig.ExchangeType, // type
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
	queueName := w.rabbitConfig.RoutingKey

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

	log.Info().Str("component", "OCR_WORKER").Str("RoutingKey", w.rabbitConfig.RoutingKey).
		Str("tag", tag).
		Msg("binding to routing key")

	if err = w.channel.QueueBind(
		queue.Name,                // name of the queue
		w.rabbitConfig.RoutingKey, // bindingKey
		w.rabbitConfig.Exchange,   // sourceExchange
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
			Str("CorrelationId", d.CorrelationId).
			Str("ReplyTo", d.ReplyTo).
			Str("ConsumerTag", d.ConsumerTag).
			Uint64("DeliveryTag", d.DeliveryTag).
			Str("Exchange", d.Exchange).
			Str("RoutingKey", d.RoutingKey).
			Msg("got delivery")
		// reply from engine here
		// id is not set, Text is set, Status is set
		ocrResult, err := w.resultForDelivery(d)
		if err != nil {
			log.Error().Err(err).Str("component", "OCR_WORKER").
				Str("tag", tag).
				Msg("Error generating ocr result")
		}

		err = w.sendRpcResponse(ocrResult, d.ReplyTo, d.CorrelationId)
		if err != nil {
			log.Error().Err(err).Str("component", "OCR_WORKER").
				Str("id", ocrResult.ID).
				Str("tag", tag).
				Msg("Error generating ocr result")

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

func (w *OcrRpcWorker) resultForDelivery(d amqp.Delivery) (OcrResult, error) {

	ocrRequest := OcrRequest{}
	ocrResult := OcrResult{}
	err := json.Unmarshal(d.Body, &ocrRequest)
	if err != nil {
		msg := "Error unmarshalling json: %v.  Error: %v"
		errMsg := fmt.Sprintf(msg, string(d.CorrelationId), err)
		log.Error().Err(err).Caller().
			Str("Id", d.CorrelationId).
			Str("tag", tag).
			Msg("error unmarshalling json delivery")
		ocrResult.Text = errMsg
		return ocrResult, err
	}

	ocrEngine := NewOcrEngine(ocrRequest.EngineType)
	ocrResult, err = ocrEngine.ProcessRequest(ocrRequest)
	if err != nil {
		msg := "Error processing image url: %v.  Error: %v"
		errMsg := fmt.Sprintf(msg, ocrRequest.RequestID, err)
		log.Error().Err(err).
			Str("Id", ocrResult.ID).
			Str("tag", tag).
			Str("ImgUrl", ocrRequest.ImgUrl).
			Msg("Error processing image")

		ocrResult.Text = errMsg
		return ocrResult, err
	}

	return ocrResult, nil

}

func (w *OcrRpcWorker) sendRpcResponse(r OcrResult, replyTo string, correlationId string) error {

	if w.rabbitConfig.Reliable {
		// Do not use w.rabbitConfig.Reliable=true due to major issues
		// that will completely  wedge the rpc worker.  Setting the
		// buffered channels length higher would delay the problem,
		// but then it would still happen later.
		if err := w.channel.Confirm(false); err != nil {
			return err
		}

		ack, nack := w.channel.NotifyConfirm(make(chan uint64, 100), make(chan uint64, 100))

		defer confirmDeliveryWorker(ack, nack)
	}

	log.Info().Str("component", "OCR_WORKER").
		Str("tag", tag).
		Str("Id", correlationId).
		Str("replyTo", replyTo).Msg("sendRpcResponse to")
	// ocr worker is publishing back the decoded text
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}

	if err := w.channel.Publish(
		w.rabbitConfig.Exchange, // publish to an exchange
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
	log.Info().Str("component", "OCR_WORKER").Str("Id", correlationId).
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
	case <-time.After(RPCResponseTimeout):
		// this is bad, the worker will probably be dysfunctional
		// at this point, so panic
		log.Panic().Str("component", "OCR_WORKER").Msg("timeout trying to confirm delivery. Worker panic")
	}
}
