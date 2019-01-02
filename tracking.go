package main

import (
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"time"
)

var ioPrevState = map[string]map[string]interface{}{}

func runTrackingIteration() {
	_, hasMonitorTargets := config["monitor"]
	if !hasMonitorTargets {
		log.Println("Skipping tracking interaction - nothing to track")
		return
	}

	log.Println("Running tracking iteration")

	metricsData := []interface{}{}
	monitorTargets := config["monitor"].([]interface{})
	for _, target := range monitorTargets {
		targetMap := target.(map[string]interface{})
		monitorType, typeSet := targetMap["type"].(string)
		if !typeSet {
			continue
		}

		result := map[string]interface{}{}
		switch monitorType {
		case "memory":
			result = monitorMemory(targetMap)
		case "io":
			result = monitorIO(targetMap)
		default:
			log.Println("Unknown tracking type:", monitorType)
		}
		result["type"] = monitorType
		monitorTag, hasTag := targetMap["tag"]
		if hasTag {
			result["tag"] = monitorTag
		} else {
			result["tag"] = monitorType
		}
		result["timestamp"] = time.Now().UTC().Format(time.RFC3339)

		metricsData = append(metricsData, result)
	}

	_, err := makeAPIRequest("POST", "/api/v1/metric", map[string]interface{}{
		"metrics": metricsData,
	})
	if err != nil {
		log.Println("Failed to make metrics api request")
	}
}

func monitorMemory(params map[string]interface{}) map[string]interface{} {
	// https://www.kernel.org/doc/Documentation/filesystems/proc.txt
	result := map[string]interface{}{}

	memoryData, _ := ioutil.ReadFile("/proc/meminfo")
	memoryDataLines := strings.Split(string(memoryData), "\n")
	for _, line := range memoryDataLines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		amount, _ := strconv.Atoi(fields[1])
		switch fields[0] {
		case "MemTotal:":
			result["total"] = amount
		case "MemFree:":
			result["free"] = amount
		case "MemAvailable:":
			result["available"] = amount
		case "Buffers:":
			result["buffers"] = amount
		case "Cached:":
			result["cached"] = amount
		case "SwapTotal:":
			result["swapTotal"] = amount
		case "SwapFree:":
			result["swapFree"] = amount
		}
	}
	return result
}

func monitorIO(params map[string]interface{}) map[string]interface{} {
	// https://www.kernel.org/doc/Documentation/iostats.txt
	result := map[string]interface{}{}

	ioData, _ := ioutil.ReadFile("/proc/diskstats")
	ioDataLines := strings.Split(string(ioData), "\n")
	for _, line := range ioDataLines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		deviceName := fields[2]
		// Block size according to:
		// https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/include/linux/types.h?id=v4.4-rc6#n121
		deviceBlockSize := int64(512)
		sectorsRead, _ := strconv.Atoi(fields[5])
		sectorsWritten, _ := strconv.Atoi(fields[9])
		timestamp := time.Now().UnixNano() / 1000000
		if strings.HasPrefix(deviceName, "loop") {
			continue
		}

		devicePrevState, devicePrevStateExists := ioPrevState[deviceName]
		if devicePrevStateExists {
			prevSectorsRead := devicePrevState["sectorsRead"].(int)
			prevSectorsWritten := devicePrevState["sectorsWritten"].(int)
			prevTimestamp := devicePrevState["timestamp"].(int64)
			bytesRead := int64(sectorsRead-prevSectorsRead) * deviceBlockSize
			bytesWritten := int64(sectorsWritten-prevSectorsWritten) * deviceBlockSize
			bytesReadPerSecond := (bytesRead) / ((timestamp - prevTimestamp) / 1000)
			bytesWrittenPerSecond := (bytesWritten) / ((timestamp - prevTimestamp) / 1000)

			result[deviceName] = map[string]interface{}{
				"read":  bytesReadPerSecond,
				"write": bytesWrittenPerSecond,
			}
		}
		ioPrevState[deviceName] = map[string]interface{}{
			"sectorsRead":    sectorsRead,
			"sectorsWritten": sectorsWritten,
			"timestamp":      timestamp,
		}
	}

	return result
}
