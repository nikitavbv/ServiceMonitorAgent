package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var ioPrevState = map[string]map[string]interface{}{}
var cpuPrevState = map[string]map[string]interface{}{}
var networkPrevState = map[string]map[string]interface{}{}

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
		case "diskUsage":
			result = monitorDiskUsage(targetMap)
		case "cpu":
			result = monitorCPUUsage(targetMap)
		case "uptime":
			result = monitorUptime(targetMap)
		case "network":
			result = monitorNetwork(targetMap)
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

func monitorDiskUsage(params map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	cmd := exec.Command("df", "-x", "squashfs", "-x", "devtmpfs", "-x", "tmpfs", "-x", "fuse", "--output=source,size,used")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Println(err)
		return result
	}

	diskUsageDataLines := strings.Split(out.String(), "\n")
	for _, line := range diskUsageDataLines {
		if line == "" || strings.HasPrefix(line, "Filesystem") {
			continue
		}

		fields := strings.Fields(line)
		filesystem := fields[0]
		total, _ := strconv.Atoi(fields[1])
		used, _ := strconv.Atoi(fields[2])
		result[filesystem] = map[string]interface{}{
			"total": total,
			"used":  used,
		}
	}

	return result
}

func monitorCPUUsage(params map[string]interface{}) map[string]interface{} {
	// http://man7.org/linux/man-pages/man5/proc.5.html
	result := map[string]interface{}{}

	cpuData, _ := ioutil.ReadFile("/proc/stat")
	cpuDataLines := strings.Split(string(cpuData), "\n")
	timestamp := time.Now().UnixNano() / 1000000
	for _, line := range cpuDataLines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		cpu := fields[0]
		user, _ := strconv.Atoi(fields[1])
		nice, _ := strconv.Atoi(fields[2])
		system, _ := strconv.Atoi(fields[3])
		idle, _ := strconv.Atoi(fields[4])
		iowait, _ := strconv.Atoi(fields[5])
		irq, _ := strconv.Atoi(fields[6])
		softirq, _ := strconv.Atoi(fields[7])
		steal, _ := strconv.Atoi(fields[8])
		guest, _ := strconv.Atoi(fields[9])
		guestNice, _ := strconv.Atoi(fields[10])

		prevState, cpuPrevStateExists := cpuPrevState[cpu]
		if cpuPrevStateExists {
			prevUser := prevState["user"].(int)
			prevNice := prevState["nice"].(int)
			prevSystem := prevState["system"].(int)
			prevIdle := prevState["idle"].(int)
			prevIOWait := prevState["iowait"].(int)
			prevIrq := prevState["irq"].(int)
			prevSoftIrq := prevState["softirq"].(int)
			prevSteal := prevState["steal"].(int)
			prevGuest := prevState["guest"].(int)
			prevGuestNice := prevState["guestNice"].(int)
			prevTimestamp := prevState["timestamp"].(int64)

			result[cpu] = map[string]interface{}{
				"user":      int64(user-prevUser) / ((timestamp - prevTimestamp) / 1000),
				"nice":      int64(nice-prevNice) / ((timestamp - prevTimestamp) / 1000),
				"system":    int64(system-prevSystem) / ((timestamp - prevTimestamp) / 1000),
				"idle":      int64(idle-prevIdle) / ((timestamp - prevTimestamp) / 1000),
				"iowait":    int64(iowait-prevIOWait) / ((timestamp - prevTimestamp) / 1000),
				"irq":       int64(irq-prevIrq) / ((timestamp - prevTimestamp) / 1000),
				"softirq":   int64(softirq-prevSoftIrq) / ((timestamp - prevTimestamp) / 1000),
				"guest":     int64(guest-prevGuest) / ((timestamp - prevTimestamp) / 1000),
				"steal":     int64(steal-prevSteal) / ((timestamp - prevTimestamp) / 1000),
				"guestNice": int64(prevGuestNice-guestNice) / ((timestamp - prevTimestamp) / 1000),
			}
		}
		cpuPrevState[cpu] = map[string]interface{}{
			"user":      user,
			"nice":      nice,
			"system":    system,
			"idle":      idle,
			"iowait":    iowait,
			"irq":       irq,
			"softirq":   softirq,
			"steal":     steal,
			"guest":     guest,
			"guestNice": guestNice,
			"timestamp": timestamp,
		}
	}

	return result
}

func monitorUptime(params map[string]interface{}) map[string]interface{} {
	// http://man7.org/linux/man-pages/man5/proc.5.html
	result := map[string]interface{}{}

	uptimeData, _ := ioutil.ReadFile("/proc/uptime")
	fields := strings.Fields(string(uptimeData))
	result["uptime"] = fields[0]

	return result
}

func monitorNetwork(params map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}

	networkData, _ := ioutil.ReadFile("/proc/net/dev")
	networkDataLines := strings.Split(string(networkData), "\n")
	timestamp := time.Now().UnixNano() / 1000000
	for _, line := range networkDataLines {
		if line == "" || strings.Contains(line, "|") {
			continue
		}

		fields := strings.Fields(line)
		deviceName := fields[0]
		bytesReceived, _ := strconv.Atoi(fields[1])
		bytesSent, _ := strconv.Atoi(fields[9])

		devicePrevState, devicePrevStateExists := networkPrevState[deviceName]
		if devicePrevStateExists {
			prevBytesReceived := devicePrevState["bytesReceived"].(int)
			prevBytesSent := devicePrevState["bytesSent"].(int)
			prevTimestamp := devicePrevState["timestamp"].(int64)

			result[deviceName] = map[string]interface{}{
				"bytesSent":     int64(bytesSent-prevBytesSent) / ((timestamp - prevTimestamp) / 1000),
				"bytesReceived": int64(bytesReceived-prevBytesReceived) / ((timestamp - prevTimestamp) / 1000),
			}
		}
		networkPrevState[deviceName] = map[string]interface{}{
			"bytesReceived": bytesReceived,
			"bytesSent":     bytesSent,
			"timestamp":     timestamp,
		}
	}
	return result
}
