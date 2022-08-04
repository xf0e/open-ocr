package ocrworker

import (
	"sync"
	"sync/atomic"
)

var (
	RequestsTrack      = sync.Map{}
	RequestTrackLength = uint32(0)
	// ocrWasSentBackChan = make(chan string)
)

// CheckOcrStatusByID checks status of an ocr request based on origin of request
func CheckOcrStatusByID(requestID string) (OcrResult, bool) {
	if _, ok := RequestsTrack.Load(requestID); !ok {
		// log.Info().Str("component", "OCR_CLIENT").Str("RequestID", requestID).Msg("no such request found in the queue")
		return OcrResult{}, false
	}

	v, ok := RequestsTrack.Load(requestID)
	if ok {
		defer deleteRequestFromQueue(requestID)
		ocrResult := v.(OcrResult)
		return ocrResult, true
	} else {
		return OcrResult{}, false
	}
}

func getQueueLen() uint {
	return uint(atomic.LoadUint32(&RequestTrackLength))
}

func deleteRequestFromQueue(requestID string) {
	inFlightGauge.Dec()
	atomic.AddUint32(&RequestTrackLength, ^uint32(0))
	RequestsTrack.Delete(requestID)
}

func addNewOcrResultToQueue(requestID string, rpcResponseChan chan OcrResult) {
	inFlightGauge.Inc()
	atomic.AddUint32(&RequestTrackLength, 1)
	RequestsTrack.Store(requestID, rpcResponseChan)

	// this go routine will cancel the request after global timeout or if request was sent back
	// if the requestID arrives on ocrWasSentBackChan - ocrResult was send back to requester an request deletion is triggered
	// go func() {
	// 	select {
	// 	case <-ocrWasSentBackChan:
	// 		if _, ok := RequestsTrack.Load(requestID); ok {
	// 			deleteRequestFromQueue(requestID)
	// 		}
	// 		// TODO: a bug leaking goroutines if the global timeout is set to a low value the routine in ocr_rpc_client:221 will leak
	// 		// TODO since there is no listener in this goroutine since this goroutine is dead
	// 	/* case <-time.After(time.Second * time.Duration(storageTime+10)):
	// 		if _, ok := RequestsTrack.Load(requestID); ok {
	// 			deleteRequestFromQueue(requestID)
	// 		}*/
	// //default:
	//
	// 	}
	// }()
}
