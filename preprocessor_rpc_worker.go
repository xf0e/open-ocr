package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/streadway/amqp"
)

type PreprocessorRpcWorker struct {
	rabbitConfig    RabbitConfig
	conn            *amqp.Connection
	channel         *amqp.Channel
	tag             string
	Done            chan error
	bindingKey      string
	preprocessorMap map[string]Preprocessor
}

var preprocessorTag = ksuid.New().String()

func NewPreprocessorRpcWorker(rc RabbitConfig, preprocessor string) (*PreprocessorRpcWorker, error) {

	preprocessorMap := make(map[string]Preprocessor)
	preprocessorMap[PreprocessorStrokeWidthTransform] = StrokeWidthTransformer{}
	preprocessorMap[PreprocessorIdentity] = IdentityPreprocessor{}
	preprocessorMap[PreprocessorConvertPdf] = ConvertPdf{}

	_, ok := preprocessorMap[preprocessor]
	if !ok {
		return nil, fmt.Errorf("no preprocessor found for: %q", preprocessor)
	}

	preprocessorRpcWorker := &PreprocessorRpcWorker{
		rabbitConfig:    rc,
		conn:            nil,
		channel:         nil,
		tag:             preprocessorTag,
		Done:            make(chan error),
		bindingKey:      preprocessor,
		preprocessorMap: preprocessorMap,
	}
	return preprocessorRpcWorker, nil
}

func (w PreprocessorRpcWorker) Run() error {

	var err error
	log.Info().Str("component", "PREPROCESSOR_WORKER").Msg("Run() called...")
	log.Info().Str("component", "PREPROCESSOR_WORKER").
		Str("AmqpURI", w.rabbitConfig.AmqpURI).
		Msg("dialing amqpURI...")

	w.conn, err = amqp.Dial(w.rabbitConfig.AmqpURI)
	if err != nil {
		return err
	}

	go func() {
		fmt.Printf("closing: %s", <-w.conn.NotifyClose(make(chan *amqp.Error)))
	}()

	log.Info().Str("component", "PREPROCESSOR_WORKER").Msg("got Connection, getting Channel")
	w.channel, err = w.conn.Channel()
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

	// just call the queue the same name as the binding key, since
	// there is no reason to have a different name.
	queueName := w.bindingKey

	queue, err := w.channel.QueueDeclare(
		queueName, // name of the queue
		true,      // durable
		false,     // delete when usused
		false,     // exclusive
		false,     // noWait
		nil,       // arguments
	)
	if err != nil {
		return err
	}

	if err = w.channel.QueueBind(
		queue.Name,              // name of the queue
		w.bindingKey,            // bindingKey
		w.rabbitConfig.Exchange, // sourceExchange
		false,                   // noWait
		nil,                     // arguments
	); err != nil {
		return err
	}

	log.Info().Str("component", "PREPROCESSOR_WORKER").
		Str("preprocessorTag", preprocessorTag).
		Str("bindingKey", w.bindingKey).
		Msg("Queue bound to Exchange, starting Consume")
	deliveries, err := w.channel.Consume(
		queue.Name,      // name
		preprocessorTag, // consumerTag,
		true,            // noAck
		false,           // exclusive
		false,           // noLocal
		false,           // noWait
		nil,             // arguments
	)
	if err != nil {
		return err
	}

	go w.handle(deliveries, w.Done)

	return nil
}

func (w *PreprocessorRpcWorker) Shutdown() error {
	// will close() the deliveries channel
	if err := w.channel.Cancel(w.tag, true); err != nil {
		return fmt.Errorf("worker cancel failed: %s", err)
	}

	if err := w.conn.Close(); err != nil {
		return fmt.Errorf("AMQP connection close error: %s", err)
	}

	defer log.Info().Str("component", "PREPROCESSOR_WORKER").Msg("Shutdown OK")

	// wait for handle() to exit
	return <-w.Done
}

func (w *PreprocessorRpcWorker) handle(deliveries <-chan amqp.Delivery, done chan error) {
	for d := range deliveries {
		log.Info().Str("component", "PREPROCESSOR_WORKER").
			Int("size", len(d.Body)).
			Uint64("DeliveryTag", d.DeliveryTag).
			Str("ReplyTo", d.ReplyTo).
			Msg("got delivery")

		err := w.handleDelivery(d)
		if err != nil {
			log.Error().Err(err).Str("component", "PREPROCESSOR_WORKER").Msg("Error handling delivery in preprocessor.")
		}

	}
	log.Info().Str("component", "PREPROCESSOR_WORKER").Msg("handle: deliveries channel closed")
	done <- fmt.Errorf("handle: deliveries channel closed")
}

func (w *PreprocessorRpcWorker) preprocessImage(ocrRequest *OcrRequest) error {

	descriptor := w.bindingKey // eg, "stroke-width-transform"
	preprocessor := w.preprocessorMap[descriptor]
	log.Info().Str("component", "PREPROCESSOR_WORKER").
		Str("ocrRequest", ocrRequest.RequestID).Str("descriptor", descriptor).
		Msg("Preprocess request via descriptor")

	err := preprocessor.preprocess(ocrRequest)
	if err != nil {
		msg := "Error doing %s on: %v."
		errMsg := fmt.Sprintf(msg, descriptor, ocrRequest)
		log.Error().Err(err).Str("component", "PREPROCESSOR_WORKER").Msg(errMsg)
		return err
	}
	return nil

}

func (w *PreprocessorRpcWorker) strokeWidthTransform(ocrRequest *OcrRequest) error {

	// write bytes to a temp file

	tmpFileNameInput, err := createTempFileName()
	if err != nil {
		return err
	}
	defer os.Remove(tmpFileNameInput)

	tmpFileNameOutput, err := createTempFileName()
	if err != nil {
		return err
	}
	defer os.Remove(tmpFileNameOutput)

	err = saveBytesToFileName(ocrRequest.ImgBytes, tmpFileNameInput)
	if err != nil {
		return err
	}

	// run DecodeText binary on it (if not in path, print warning and do nothing)
	darkOnLightSetting := "1" // todo: this should be passed as a param.
	out, err := exec.Command(
		"DetectText",
		tmpFileNameInput,
		tmpFileNameOutput,
		darkOnLightSetting,
	).CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("component", "PREPROCESSOR_WORKER").Msg(string(out))
	}
	log.Info().Str("component", "PREPROCESSOR_WORKER").Msg("finish DetectText")

	// read bytes from output file into ocrRequest.ImgBytes
	resultBytes, err := ioutil.ReadFile(tmpFileNameOutput)
	if err != nil {
		return err
	}

	ocrRequest.ImgBytes = resultBytes

	return nil

}

func (w *PreprocessorRpcWorker) handleDelivery(d amqp.Delivery) error {

	ocrRequest := OcrRequest{}
	err := json.Unmarshal(d.Body, &ocrRequest)
	if err != nil {
		msg := "Error unmarshaling json: %v."
		errMsg := fmt.Sprintf(msg, string(d.Body))
		log.Error().Err(err).Str("component", "PREPROCESSOR_WORKER").Msg(errMsg)
		return err
	}

	routingKey := ocrRequest.nextPreprocessor(w.rabbitConfig.RoutingKey)
	log.Info().Str("component", "PREPROCESSOR_WORKER").Str("routingKey", routingKey).
		Msg("publishing with routing key")

	err = w.preprocessImage(&ocrRequest)
	if err != nil {
		msg := "Error preprocessing image: %v."
		errMsg := fmt.Sprintf(msg, ocrRequest)
		log.Error().Err(err).Str("component", "PREPROCESSOR_WORKER").Msg(errMsg)
		return err
	}

	ocrRequestJson, err := json.Marshal(ocrRequest)
	if err != nil {
		return err
	}

	log.Info().Str("component", "PREPROCESSOR_WORKER").Str("routingKey", routingKey).
		Msg("sendRpcResponse via routingKey")

	if err := w.channel.Publish(
		w.rabbitConfig.Exchange, // publish to an exchange
		routingKey,              // routing to 0 or more queues
		false,                   // mandatory
		false,                   // immediate
		amqp.Publishing{
			Headers:         amqp.Table{},
			ContentType:     "text/plain",
			ContentEncoding: "",
			Body:            ocrRequestJson,
			DeliveryMode:    amqp.Transient, // 1=non-persistent, 2=persistent
			Priority:        0,              // 0-9
			ReplyTo:         d.ReplyTo,
			CorrelationId:   d.CorrelationId,
			// a bunch of application/implementation-specific fields
		},
	); err != nil {
		return err
	}
	log.Info().Str("component", "PREPROCESSOR_WORKER").Msg("handleDelivery succeeded")

	return nil
}
