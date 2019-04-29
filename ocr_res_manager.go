package ocrworker

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"time"
)

type ocrQueueManager struct {
	NumMessages  uint `json:"messages"`
	NumConsumers uint `json:"consumers"`
	MessageBytes uint `json:"message_bytes"`
}

type ocrResManager struct {
	MemLimit uint64 `json:"mem_limit"`
	MemUsed  uint64 `json:"mem_used"`
}

const (
	factorForMessageAccept uint   = 2
	memoryThreshold        uint64 = 95
)

func newOcrQueueManager() *ocrQueueManager {
	return &ocrQueueManager{}
}

func newOcrResManager() []ocrResManager {
	resManager := make([]ocrResManager, 0)
	return resManager
}

var (
	queueManager *ocrQueueManager
	resManager   []ocrResManager
)

// checks if resources for incoming request are available
func CheckForAcceptRequest(urlQueue string, urlStat string, statusChanged bool) bool {

	isAvailable := false
	jsonQueueStat, err := url2bytes(urlQueue)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_RESMAN").Msg("can't get Que stats")
		return false
	}
	jsonResStat, err := url2bytes(urlStat)
	if err != nil {
		log.Error().Caller().Err(err).Str("component", "OCR_RESMAN").Msg("can't get RabbitMQ memory stats")
		log.Error().Err(err).Str("component", "OCR_RESMAN").
			Str("body", string(jsonResStat))
		return false
	}

	err = json.Unmarshal(jsonQueueStat, queueManager)
	if err != nil {
		log.Error().Caller().Err(err).Str("component", "OCR_RESMAN").Msg("error unmarshaling json")
		log.Error().Err(err).Str("component", "OCR_RESMAN").
			Str("body", string(jsonQueueStat))
		return false
	}

	err = json.Unmarshal(jsonResStat, &resManager)
	if err != nil {
		log.Error().Caller().Err(err).Str("component", "OCR_RESMAN").Msg("error unmarshaling json")
		log.Error().Err(err).Str("component", "OCR_RESMAN").
			Str("body", string(jsonResStat))
		return false
	}

	flagForResources := schedulerByMemoryLoad()
	flagForQueue := schedulerByWorkerNumber()
	if flagForQueue && flagForResources {
		isAvailable = true
	}

	if statusChanged {
		log.Info().Str("component", "OCR_RESMAN").
			Uint("MessageBytes", queueManager.MessageBytes).
			Uint("NumConsumers", queueManager.NumConsumers).
			Uint("NumMessages", queueManager.NumMessages).
			Interface("resManager", resManager).
			Msg("OCR_RESMAN stats")

		if isAvailable {
			log.Info().Str("component", "OCR_RESMAN").Msg("open-ocr is operational with free resources. We are ready to serve")
		} else {
			log.Info().Str("component", "OCR_RESMAN").Msg("open-ocr is alive but won't serve any requests. Workers are busy or not connected")
		}

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
	if (queueManager.NumMessages) < (queueManager.NumConsumers * factorForMessageAccept) {
		resFlag = true
	}
	return resFlag
}

// SetResManagerState returns the boolean of resource manager if memory of rabbitMQ and the number
// messages is not to high which is depends on formula factor * number of connected workers
func SetResManagerState(ampqAPIConfig RabbitConfig) {
	resManager = newOcrResManager()
	queueManager = newOcrQueueManager()
	var urlQueue, urlStat = "", ""
	urlQueue += ampqAPIConfig.AmqpAPIURI + ampqAPIConfig.APIPathQueue + ampqAPIConfig.APIQueueName
	urlStat += ampqAPIConfig.AmqpAPIURI + ampqAPIConfig.APIPathStats

	var boolValueChanged = false
	var boolNewValue = false
	var boolOldValue = true
	for {
		// only print the RESMAN output if the state has changed
		boolValueChanged = boolOldValue != boolNewValue
		if boolValueChanged {
			boolOldValue = boolNewValue
		}
		ServiceCanAcceptMu.Lock()
		ServiceCanAccept = CheckForAcceptRequest(urlQueue, urlStat, boolValueChanged)
		boolNewValue = ServiceCanAccept
		ServiceCanAcceptMu.Unlock()
		time.Sleep(1 * time.Second)
	}
}
