package main

import (
	"bytes"
	"database/sql"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var nginxStatsClient = &http.Client{}

var ioPrevState = map[string]map[string]interface{}{}
var cpuPrevState = map[string]map[string]interface{}{}
var networkPrevState = map[string]map[string]interface{}{}
var nginxPrevState = map[string]map[string]interface{}{}
var mysqlPrevState = map[string]map[string]interface{}{}

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
		case "docker":
			result = monitorDocker(targetMap)
		case "nginx":
			result = monitorNGINX(targetMap)
		case "mysql":
			result = monitorMySQL(targetMap)
		default:
			log.Println("Unknown tracking type:", monitorType)
		}
		if len(result) == 0 {
			continue
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
	result["devices"] = []map[string]interface{}{}

	ioData, _ := ioutil.ReadFile("/proc/diskstats")
	ioDataLines := strings.Split(string(ioData), "\n")
	for _, line := range ioDataLines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		deviceName := fields[2]
		// Block size according to:
		// https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git
		// /tree/include/linux/types.h?id=v4.4-rc6#n121
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

			result["devices"] = append(result["devices"].([]map[string]interface{}), map[string]interface{}{
				"device": deviceName,
				"read":   bytesReadPerSecond,
				"write":  bytesWrittenPerSecond,
			})
		}
		ioPrevState[deviceName] = map[string]interface{}{
			"sectorsRead":    sectorsRead,
			"sectorsWritten": sectorsWritten,
			"timestamp":      timestamp,
		}
	}

	if len(result["devices"].([]map[string]interface{})) == 0 {
		return map[string]interface{}{}
	}
	return result
}

func monitorDiskUsage(params map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	result["filesystems"] = []map[string]interface{}{}

	cmd := exec.Command(
		"df", "-x", "squashfs", "-x", "devtmpfs",
		"-x", "tmpfs", "-x", "fuse",
		"--output=source,size,used",
	)
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
		result["filesystems"] = append(result["filesystems"].([]map[string]interface{}), map[string]interface{}{
			"filesystem": filesystem,
			"total":      total,
			"used":       used,
		})
	}

	if len(result["filesystems"].([]map[string]interface{})) == 0 {
		return map[string]interface{}{}
	}
	return result
}

func monitorCPUUsage(params map[string]interface{}) map[string]interface{} {
	// http://man7.org/linux/man-pages/man5/proc.5.html
	result := map[string]interface{}{}
	result["cpus"] = []map[string]interface{}{}

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

			result["cpus"] = append(result["cpus"].([]map[string]interface{}), map[string]interface{}{
				"cpu":       cpu,
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
			})
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

	if len(result["cpus"].([]map[string]interface{})) == 0 {
		return map[string]interface{}{}
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
	result["devices"] = []map[string]interface{}{}

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

			result["devices"] = append(result["devices"].([]map[string]interface{}), map[string]interface{}{
				"device":        deviceName,
				"bytesSent":     int64(bytesSent-prevBytesSent) / ((timestamp - prevTimestamp) / 1000),
				"bytesReceived": int64(bytesReceived-prevBytesReceived) / ((timestamp - prevTimestamp) / 1000),
			})
		}
		networkPrevState[deviceName] = map[string]interface{}{
			"bytesReceived": bytesReceived,
			"bytesSent":     bytesSent,
			"timestamp":     timestamp,
		}
	}

	if len(result["devices"].([]map[string]interface{})) == 0 {
		return map[string]interface{}{}
	}
	return result
}

func monitorDocker(params map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	result["containers"] = []map[string]interface{}{}

	cmd := exec.Command("docker", "ps", "--format", "{{.Image}}|{{.Status}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Println(err)
		return result
	}

	dockerDataLines := strings.Split(out.String(), "\n")
	for _, line := range dockerDataLines {
		if line == "" || strings.HasPrefix(line, "CONTAINER") {
			continue
		}

		fields := strings.Split(line, "|")
		containerStatus := fields[1]
		containerName := fields[0]

		result["containers"] = append(result["containers"].([]map[string]interface{}), map[string]interface{}{
			"containerName": containerName,
			"status":        containerStatus,
		})
	}

	if len(result["containers"].([]map[string]interface{})) == 0 {
		return map[string]interface{}{}
	}
	return result
}

func monitorNGINX(params map[string]interface{}) map[string]interface{} {
	// http://nginx.org/en/docs/http/ngx_http_stub_status_module.html
	result := map[string]interface{}{}
	endpoint := params["endpoint"].(string)

	response, err := http.Get(endpoint)
	if err != nil {
		log.Println("Error while nginx stats request: ", err)
		return result
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Println("Error while reading nginx response", err)
		return result
	}

	responseLines := strings.Split(string(contents), "\n")
	requests, _ := strconv.Atoi(strings.Fields(responseLines[2])[2])
	prevState, nginxPrevStateExists := nginxPrevState[endpoint]
	timestamp := time.Now().UnixNano() / 1000000
	if nginxPrevStateExists {
		prevRequests := prevState["requests"].(int)
		prevTimestamp := prevState["timestamp"].(int64)

		result["requests"] = float64(requests-prevRequests) / float64((timestamp-prevTimestamp)/1000)
	}
	nginxPrevState[endpoint] = map[string]interface{}{
		"requests":  requests,
		"timestamp": timestamp,
	}

	return result
}

func monitorMySQL(params map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	connection := params["connection"].(string)

	db, err := sql.Open("mysql", connection)
	if err != nil {
		log.Println("Failed to connect to mysql database:", err)
		return result
	}
	defer db.Close()

	rows, err := db.Query("SHOW GLOBAL STATUS LIKE 'Questions'")
	if err != nil {
		log.Println("Error while making SQL query:", err)
		return result
	}

	var varName, value string
	prevState, mysqlPrevStateExists := mysqlPrevState[connection]
	timestamp := time.Now().UnixNano() / 1000000
	for rows.Next() {
		err = rows.Scan(&varName, &value)
		if err != nil {
			log.Println("Error while reading sql result:", err)
			return result
		}
		nValue, _ := strconv.ParseFloat(value, 64)
		if mysqlPrevStateExists {
			prevValue, _ := strconv.ParseFloat(prevState[varName].(string), 64)
			prevTimestamp := prevState["timestamp"].(int64)
			result[strings.ToLower(varName)] = float64(nValue-prevValue) / float64((timestamp-prevTimestamp)/1000)
		} else {
			mysqlPrevState[connection] = map[string]interface{}{}
			prevState = mysqlPrevState[connection]
		}
		mysqlPrevState[connection][varName] = value
	}
	prevState["timestamp"] = timestamp
	mysqlPrevState[connection] = prevState

	return result
}
