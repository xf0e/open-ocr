package ocrworker

import (
	"sync"
	"sync/atomic"
	"time"
)

var (
	// requestsAndTimersMu sync.RWMutex
	// requestsAndTimersMu sync.RWMutex
	// Requests is for holding and monitoring queued requests
	// Requests           = make(map[string]chan OcrResult)
	ocrWasSentBackChan = make(chan string)
	RequestsTrack      = sync.Map{}
	RequestTrackLength = uint32(0)
)

// CheckOcrStatusByID checks status of an ocr request based on origin of request
func CheckOcrStatusByID(requestID string) (OcrResult, bool) {

	if _, ok := RequestsTrack.Load(requestID); !ok {
		// log.Info().Str("component", "OCR_CLIENT").Str("RequestID", requestID).Msg("no such request found in the queue")
		return OcrResult{}, false
	}

	ocrResult := OcrResult{}

	tempChannel := make(chan OcrResult)
	v, ok := RequestsTrack.Load(requestID)
	if ok {
		tempChannel = v.(chan OcrResult)
	} else {
		return OcrResult{}, false
	}

	select {
	case ocrResult, _ = <-tempChannel:
		// log.Debug().Str("component", "OCR_CLIENT").Msg("got ocrResult := <-Requests[requestID]")
		defer deleteRequestFromQueue(requestID)
	default:
		return OcrResult{Status: "processing", ID: requestID}, true
	}

	return ocrResult, true
}

func getQueueLen() uint {

	return uint(atomic.LoadUint32(&RequestTrackLength))
}

func deleteRequestFromQueue(requestID string) {

	inFlightGauge.Dec()
	atomic.AddUint32(&RequestTrackLength, ^uint32(0))
	RequestsTrack.Delete(requestID)
}

func addNewOcrResultToQueue(storageTime int, requestID string, rpcResponseChan chan OcrResult) {

	inFlightGauge.Inc()
	atomic.AddUint32(&RequestTrackLength, 1)
	RequestsTrack.Store(requestID, rpcResponseChan)

	// this go routine will cancel the request after global timeout or if request was sent back
	// if the requestID arrives on ocrWasSentBackChan - ocrResult was send back to requester an request deletion is triggered
	go func() {
		select {
		case <-ocrWasSentBackChan:
			if _, ok := RequestsTrack.Load(requestID); ok {
				deleteRequestFromQueue(requestID)
			}
		case <-time.After(time.Second * time.Duration(storageTime+10)):
			if _, ok := RequestsTrack.Load(requestID); ok {
				deleteRequestFromQueue(requestID)
			}
		}
	}()
}
