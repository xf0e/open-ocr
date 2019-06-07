package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/nu7hatch/gouuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sasha-s/go-deadlock"
	"net/http"
	"os"
)

// OcrHTTPStatusHandler is for initial handling of ocr request
type OcrHTTPStatusHandler struct {
	RabbitConfig RabbitConfig
}

func NewOcrHttpHandler(r RabbitConfig) *OcrHTTPStatusHandler {
	return &OcrHTTPStatusHandler{
		RabbitConfig: r,
	}
}

var (
	// AppStop and ServiceCanAccept are global. Used to set the flag for logging and stopping the application
	AppStop            bool
	ServiceCanAccept   bool
	ServiceCanAcceptMu deadlock.Mutex
)

func (s *OcrHTTPStatusHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	//_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	log.Info().Str("component", "OCR_HTTP").Msg("serveHttp called")
	defer req.Body.Close()

	ServiceCanAcceptMu.Lock()
	serviceCanAcceptLocal := ServiceCanAccept
	appStopLocal := AppStop
	ServiceCanAcceptMu.Unlock()
	if !serviceCanAcceptLocal && !appStopLocal {
		err := "no resources available to process the request"
		log.Error().Err(fmt.Errorf(err)).Str("component", "OCR_HTTP")
		http.Error(w, err, 503)
		return
	}

	if !serviceCanAcceptLocal && appStopLocal {
		err := "service is going down"
		log.Error().Err(fmt.Errorf(err)).Str("component", "OCR_HTTP")
		http.Error(w, err, 503)
		return
	}

	ocrRequest := OcrRequest{}
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&ocrRequest)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_HTTP")
		http.Error(w, "Unable to unmarshal json", 400)
		return
	}

	ocrResult, err := HandleOcrRequest(ocrRequest, s.RabbitConfig)

	if err != nil {
		msg := "Unable to perform OCR decode.  Error: %v"
		errMsg := fmt.Sprintf(msg, err)
		log.Error().Err(err).Str("component", "OCR_HTTP").Msg("Unable to perform OCR decode")
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	js, err := json.Marshal(ocrResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(js)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_HTTP").Msg("http write() failed")
	}
}

func HandleOcrRequest(ocrRequest OcrRequest, rabbitConfig RabbitConfig) (OcrResult, error) {

	var requestIDRaw, _ = uuid.NewV4()
	requestID := requestIDRaw.String()
	ocrResult := newOcrResult(requestID)
	ocrRequest.RequestID = requestID
	// set the context for zerolog, RequestID will be printed on each logging event
	logger := zerolog.New(os.Stdout).With().
		Str("RequestID", requestID).Timestamp().Logger()
	switch ocrRequest.InplaceDecode {
	case true:
		// inplace decode: short circuit rabbitmq, and just call
		// ocr engine directly
		ocrEngine := NewOcrEngine(ocrRequest.EngineType)

		ocrResult, err := ocrEngine.ProcessRequest(ocrRequest)

		if err != nil {
			logger.Error().Err(err).Str("component", "OCR_HTTP").Msg("Error processing ocr request")
			return OcrResult{}, err
		}

		return ocrResult, nil
	default:
		// add a new job to rabbitMQ and wait for worker to respond w/ result
		ocrClient, err := NewOcrRpcClient(rabbitConfig)
		if err != nil {
			logger.Error().Err(err).Str("component", "OCR_HTTP")
			return OcrResult{}, err
		}

		ocrResult, err = ocrClient.DecodeImage(ocrRequest, requestID)
		if err != nil {
			logger.Error().Err(err).Str("component", "OCR_HTTP")
			return OcrResult{}, err
		}

		return ocrResult, nil
	}

}
