package ocrworker

import (
	"encoding/json"
	"fmt"
	// "github.com/sasha-s/go-deadlock"
	"net/http"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"
)

// OcrHTTPStatusHandler is for initial handling of ocr request
type OcrHTTPStatusHandler struct {
	RabbitConfig RabbitConfig
}

func NewOcrHttpHandler(r *RabbitConfig) *OcrHTTPStatusHandler {
	return &OcrHTTPStatusHandler{
		RabbitConfig: *r,
	}
}

var (
	// AppStop and ServiceCanAccept are global. Used to set the flag for logging and stopping the application
	AppStop            bool
	ServiceCanAccept   bool
	ServiceCanAcceptMu sync.Mutex
)

func (s *OcrHTTPStatusHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// _ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	log.Info().Str("component", "OCR_HTTP").Msg("serveHttp called")
	defer req.Body.Close()
	var httpStatus = 200

	ServiceCanAcceptMu.Lock()
	serviceCanAcceptLocal := ServiceCanAccept
	appStopLocal := AppStop
	ServiceCanAcceptMu.Unlock()
	if !serviceCanAcceptLocal && !appStopLocal {
		err := "no resources available to process the request"
		log.Warn().Str("component", "OCR_HTTP").Err(fmt.Errorf(err)).
			Msg("conditions for accepting new requests are not met")
		httpStatus = 503
		http.Error(w, err, httpStatus)
		return
	}

	if !serviceCanAcceptLocal && appStopLocal {
		err := "service is going down"
		log.Warn().Str("component", "OCR_HTTP").Err(fmt.Errorf(err)).
			Msg("conditions for accepting new requests are not met")
		httpStatus = 503
		http.Error(w, err, httpStatus)
		return
	}

	ocrRequest := OcrRequest{}
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&ocrRequest)
	if err != nil {
		log.Warn().Str("component", "OCR_HTTP").Err(err).
			Msg("did the client send a valid json?")
		httpStatus = 400
		http.Error(w, "Unable to unmarshal json, malformed request", httpStatus)
		return
	}

	ocrResult, httpStatus, err := HandleOcrRequest(&ocrRequest, &s.RabbitConfig)

	if err != nil {
		msg := "Unable to perform OCR decode. Error: %v"
		errMsg := fmt.Sprintf(msg, err)
		log.Error().Err(err).Str("component", "OCR_HTTP").Msg("Unable to perform OCR decode")
		http.Error(w, errMsg, httpStatus)
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

// HandleOcrRequest will process incoming OCR request by routing it through the whole process chain
func HandleOcrRequest(ocrRequest *OcrRequest, workerConfig *RabbitConfig) (OcrResult, int, error) {
	var httpStatus = 200
	var requestIDRaw = ksuid.New()
	requestID := requestIDRaw.String()
	ocrResult := newOcrResult(requestID)
	ocrRequest.RequestID = requestID
	// set the context for zerolog, RequestID will be printed on each logging event
	logger := zerolog.New(os.Stdout).With().
		Str("RequestID", requestID).Timestamp().Logger()
	switch ocrRequest.InplaceDecode {
	case true:
		// inplace decode: short circuit rabbitmq, and just call ocr engine directly
		ocrEngine := NewOcrEngine(ocrRequest.EngineType)

		workingConfig := WorkerConfig{}
		ocrResult, err := ocrEngine.ProcessRequest(ocrRequest, &workingConfig)

		if err != nil {
			logger.Error().Err(err).Str("component", "OCR_HTTP").Msg("Error processing ocr request")
			httpStatus = 500
			return OcrResult{}, httpStatus, err
		}

		return ocrResult, httpStatus, nil
	default:
		// add a new job to rabbitMQ and wait for worker to respond w/ result
		ocrClient, err := NewOcrRpcClient(workerConfig)
		if err != nil {
			logger.Error().Err(err).Str("component", "OCR_HTTP")
			httpStatus = 500
			return OcrResult{}, httpStatus, err
		}

		ocrResult, httpStatus, err = ocrClient.DecodeImage(ocrRequest, requestID)
		if err != nil {
			logger.Error().Err(err).Str("component", "OCR_HTTP")
			return OcrResult{}, httpStatus, err
		}

		return ocrResult, httpStatus, nil
	}

}
