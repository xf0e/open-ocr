package ocrworker

import (
	"sync"
	"sync/atomic"
)

var (
	RequestsTrack      = sync.Map{}
	RequestTrackLength = uint32(0)
)

// CheckOcrStatusByID checks status of an ocr request based on origin of request
func CheckOcrStatusByID(requestID string) (OcrResult, bool) {
	v, ok := RequestsTrack.Load(requestID)
	if ok {
		tempChannel := v.(chan OcrResult)
		ocrResult := OcrResult{}
		select {
		case ocrResult = <-tempChannel:
			defer deleteRequestFromQueue(requestID)
			// log.Debug().Str("component", "OCR_CLIENT").Msg("got ocrResult := <-Requests[requestID]")
			return ocrResult, true
		default:
			return OcrResult{Status: "processing", ID: requestID}, true
		}
	} else {
		// log.Debug().Str("component", "OCR_CLIENT").Str("RequestID", requestID).Msg("no such request found in the queue")
		return OcrResult{}, false
	}
}

func getQueueLen() uint {
	return uint(atomic.LoadUint32(&RequestTrackLength))
}

func deleteRequestFromQueue(requestID string) {
	if _, ok := RequestsTrack.Load(requestID); ok {
		atomic.AddUint32(&RequestTrackLength, ^uint32(0))
		inFlightGauge.Dec()
		RequestsTrack.Delete(requestID)
	}
}

func addNewOcrResultToQueue(requestID string, rpcResponseChan chan OcrResult) {
	atomic.AddUint32(&RequestTrackLength, 1)
	inFlightGauge.Inc()
	RequestsTrack.Store(requestID, rpcResponseChan)
}
