package main

import (
  "log"
  "io/ioutil"
  "strings"
  "time"
  "strconv"
)

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

    result := map[string]interface{} {}
    switch monitorType {
    case "memory":
      result = monitorMemory(targetMap)
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

  _, err := makeAPIRequest("POST", "/api/v1/metric", map[string]interface{} {
    "metrics": metricsData,
  })
  if err != nil {
    log.Println("Failed to make metrics api request")
  }
}

func monitorMemory(params map[string]interface{}) map[string]interface{} {
  result := map[string]interface{} {}

  memoryData, _ := ioutil.ReadFile("/proc/meminfo")
  memoryDataLines := strings.Split(string(memoryData), "\n")
  for _, line := range memoryDataLines {
    if (line == "") {
      continue
    }
    fields := strings.Fields(line)
    amount, _ := strconv.Atoi(fields[1])
    switch (fields[0]) {
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
