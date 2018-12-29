package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/couchbaselabs/logg"
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

// AmqpAPIConfig struct for rabbitMQ API
type AmqpAPIConfig struct {
	AmqpURI   string
	Port      string
	PathQueue string
	PathStats string
	QueueName string
}

// generates the URI for API of rabbitMQ
func DefaultResManagerConfig() AmqpAPIConfig {

	AmqpAPIConfig := AmqpAPIConfig{
		AmqpURI:   "http://guest:guest@localhost:",
		Port:      "15672",
		PathQueue: "/api/queues/%2f/",
		PathStats: "/api/nodes",
		QueueName: "decode-ocr",
	}
	return AmqpAPIConfig

}

// checks if resources for incoming request are available
func CheckForAcceptRequest(config *AmqpAPIConfig, statusChanged bool) bool {
	isAvailable := false
	resManager := make([]ocrResManager, 0)
	queueManager := new(ocrQueueManager)
	var urlQueue, urlStat = "", ""
	urlQueue += config.AmqpURI + config.Port + config.PathQueue + config.QueueName
	urlStat += config.AmqpURI + config.Port + config.PathStats
	jsonQueueStat, err := url2bytes(urlQueue)
	if err != nil {
		logg.LogError(err)
		return false
	}
	jsonResStat, err := url2bytes(urlStat)
	if err != nil {
		logg.LogError(err)
		return false
	}

	err = json.Unmarshal(jsonQueueStat, &queueManager)
	if err != nil {
		msg := "Error unmarshaling json: %v"
		errMsg := fmt.Sprintf(msg, err)
		logg.LogError(fmt.Errorf(errMsg))
		return false
	}

	err = json.Unmarshal(jsonResStat, &resManager)
	if err != nil {
		msg := "Error unmarshaling json: %v"
		errMsg := fmt.Sprintf(msg, err)
		logg.LogError(fmt.Errorf(errMsg))
		return false
	}

	flagForResources := schedulerByMemoryLoad(resManager)
	flagForQueue := schedulerByWorkerNumber(queueManager)
	if flagForQueue && flagForResources {
		isAvailable = true
	}

	if statusChanged {
		logg.LogTo("OCR_RESMAN", "Queue statistics: messages size %v, number consumers %v, number messages %v,	memory stats %v",
			queueManager.MessageBytes,
			queueManager.NumConsumers,
			queueManager.NumMessages,
			resManager)
		// logg.LogTo("OCR_RESMAN", "API URL %s", urlQueue)
		// logg.LogTo("OCR_RESMAN", "API URL %s", urlStat)
		if isAvailable {
			logg.LogTo("OCR_RESMAN", "resources for request are available")
		}
	}

	return isAvailable
}

// computes the ratio of total available memory and used memory and returns a bool value if a threshold is reached
func schedulerByMemoryLoad(resManager []ocrResManager) bool {
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
func schedulerByWorkerNumber(resManger *ocrQueueManager) bool {
	resFlag := false
	if (resManger.NumMessages) < (resManger.NumConsumers * factorForMessageAccept) {
		resFlag = true
	}
	return resFlag
}
