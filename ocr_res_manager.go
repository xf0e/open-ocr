package ocrworker

import (
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"
)

// OcrQueueManager is used as a main component of resource manager
type OcrQueueManager struct {
	NumMessages  uint `json:"messages"` // TODO: do not read the number of messages from API because it is slow, and the clients of this product may not behave and put too many requests in too fast.
	NumConsumers uint `json:"consumers"`
	MessageBytes uint `json:"message_bytes"`
}

type ocrResManager struct {
	MemLimit uint64 `json:"mem_limit"`
	MemUsed  uint64 `json:"mem_used"`
}

const (
	memoryThreshold uint64 = 95 // if memory usage of RabbitMQ is over this value, no more requests will be added
)

func newOcrQueueManager() *OcrQueueManager {
	return &OcrQueueManager{NumMessages: 0}
}

func newOcrResManager() []ocrResManager {
	resManager := make([]ocrResManager, 0)
	return resManager
}

var (
	queueManager *OcrQueueManager
	resManager   []ocrResManager
	// StopChan is used to gracefully stop http daemon
	StopChan                 = make(chan bool, 1)
	factorForMessageAccept   uint // formula: NumMessages < NumConsumers * FactorForMessageAccept
	TechnicalErrorResManager bool
)

// CheckForAcceptRequest will check by reading the RabbitMQ API if resources for incoming request are available
func CheckForAcceptRequest(urlQueue, urlStat string) bool {
	isAvailable := false
	TechnicalErrorResManager = false
	jsonQueueStat, err := url2bytes(urlQueue)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_RESMAN").Msg("can't get Queue stats")
		TechnicalErrorResManager = true
		return false
	}
	jsonResStat, err := url2bytes(urlStat)
	if err != nil {
		log.Error().Caller().Err(err).Str("component", "OCR_RESMAN").
			Str("body", string(jsonQueueStat)).
			Bytes("payload", jsonResStat).
			Msg("error calling url2bytes for rabbitMQ stats")
		TechnicalErrorResManager = true
		return false
	}

	err = json.Unmarshal(jsonQueueStat, queueManager)
	if err != nil {
		log.Error().Caller().Err(err).Str("component", "OCR_RESMAN").
			Str("body", string(jsonQueueStat)).
			Bytes("payload", jsonQueueStat).
			Msg("error unmarshalling json")
		TechnicalErrorResManager = true
		return false
	}

	err = json.Unmarshal(jsonResStat, &resManager)
	if err != nil {
		log.Error().Caller().Err(err).Str("component", "OCR_RESMAN").Msg("error unmarshalling json")
		log.Error().Err(err).Str("component", "OCR_RESMAN").
			Str("body", string(jsonResStat))
		TechnicalErrorResManager = true
		return false
	}

	if queueManager.NumConsumers == 0 {
		TechnicalErrorResManager = true
		return false
	}

	flagForResources := schedulerByMemoryLoad()
	flagForQueue := schedulerByWorkerNumber()
	if flagForQueue && flagForResources {
		TechnicalErrorResManager = false
		isAvailable = true
	}
	return isAvailable
}

// computes the ratio of total available memory and used memory and returns a bool value if a threshold is reached
func schedulerByMemoryLoad() bool {
	resFlag := false
	var memTotalAvailable uint64
	var memTotalInUse uint64
	for k := range resManager {
		memTotalInUse += resManager[k].MemUsed
		memTotalAvailable += resManager[k].MemLimit
	}

	if memTotalInUse < ((memTotalAvailable * memoryThreshold) / 100) {
		resFlag = true
	}
	return resFlag
}

// if the number of messages in the queue too high we should not accept the new messages
func schedulerByWorkerNumber() bool {
	resFlag := false
	if getQueueLen() < (queueManager.NumConsumers * factorForMessageAccept) {
		resFlag = true
	}
	return resFlag
}

// SetResManagerState sets boolean value of resource manager; if memory of rabbitMQ and the number
// messages is not exceeding  the limit
func SetResManagerState(ampqAPIConfig *RabbitConfig) {
	var sleepFor time.Duration = 5
	resManager = newOcrResManager()
	queueManager = newOcrQueueManager()
	urlQueue := ampqAPIConfig.AmqpAPIURI + ampqAPIConfig.APIPathQueue + ampqAPIConfig.APIQueueName
	urlStat := ampqAPIConfig.AmqpAPIURI + ampqAPIConfig.APIPathStats
	factorForMessageAccept = ampqAPIConfig.FactorForMessageAccept

	boolCurValue := false
	boolOldValue := true
Loop:
	for {
		if AppStop {
			break
		} // break the loop if the have to stop the app
		select {
		case <-StopChan:
			ServiceCanAcceptMu.Lock()
			ServiceCanAccept = false
			AppStop = true
			ServiceCanAcceptMu.Unlock()
			break Loop
		default:
			// only print the RESMAN output if the state has changed
			ServiceCanAcceptMu.Lock()
			boolOldValue, boolCurValue = boolCurValue, CheckForAcceptRequest(urlQueue, urlStat)
			ServiceCanAccept = boolCurValue
			ServiceCanAcceptMu.Unlock()
			if boolCurValue != boolOldValue {
				log.Info().Str("component", "OCR_RESMAN").
					Uint("MessageBytes", queueManager.MessageBytes).
					Uint("NumConsumers", queueManager.NumConsumers).
					Uint("NumMessages", queueManager.NumMessages).
					Interface("resManager", resManager).
					Msg("OCR_RESMAN stats")

				if boolCurValue {
					log.Info().Str("component", "OCR_RESMAN").Msg("open-ocr is operational with free resources, we are ready to serve")
				} else {
					log.Info().Str("component", "OCR_RESMAN").Msg("open-ocr is alive but won't serve any requests; workers are busy or not connected")
				}
			}
			time.Sleep(sleepFor * time.Second)
		}
	}
}
