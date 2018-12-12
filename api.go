package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

var apiClient = &http.Client{}

func makeAPIRequest(method string, path string, request map[string]interface{}) (map[string]interface{}, error) {
	if _, containsToken := request["token"]; !containsToken {
		request["token"] = state["token"]
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		log.Println("Error while marshalling json:", err)
		return nil, err
	}

	req, err := http.NewRequest(method, config["backend"].(string)+path, bytes.NewBuffer(requestJSON))
	if err != nil {
		log.Println("Error while new request:", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := apiClient.Do(req)
	if err != nil {
		log.Println("Error while executing http request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error while reading api response:", err)
		return nil, err
	}
	var parsedBody map[string]interface{}
	err = json.Unmarshal(body, &parsedBody)
	if err != nil {
		log.Println("Error while parsing api response body:", err)
		return nil, err
	}

	return parsedBody, nil
}
