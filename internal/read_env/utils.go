package readenv

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type payloadGetApiKey struct {
	Uuid     string `json:"uuid"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

type payloadCheckApiKey struct{
	ApiKey string `json:"api_key"`
}

func getUuid() (string, error) {
	err := godotenv.Load()
	if err != nil {
		return "", err
	}
	// check if exist
	envMap, err := godotenv.Read()
	if err != nil {
		return "", err
	}

	uuidDevice, exist := envMap["UUID"]
	if !exist {
		// write a UUID into
		newUuid := uuid.New().String()
		return newUuid, nil
	}
	return uuidDevice, nil
}

func checkKeyExist(key string) (bool, error) {
	err := godotenv.Load()
	if err != nil {
		return false, err
	}

	envMap, err := godotenv.Read()
	if err != nil {
		return false, err
	}

	_, exists := envMap[key]
	return exists, nil
}

func getApiKey(apiUri string, uuid string, name string, location string) (string, error) {
	payload := payloadGetApiKey{
		Uuid:     uuid,
		Name:     name,
		Location: location,
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Println(err)
		return "", err
	}

	response, err := http.Post(apiUri+"camera/register/", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		log.Println(errors.New(response.Status))
		return "", errors.New(response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Println(err)
		return "", err
	}

	var result map[string]interface{}
	// log.Println(body)
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Println(err)
		return "", err
	}
	apiKey, ok := result["api_key"].(string)
	if !ok {
		err := errors.New("apiKey not found or is not a string")
		log.Println(err)
		return "", err
	}

	return apiKey, nil
}

func checkApiKeyValid(apiUri string, apiKey string) (bool, error) {
	payload:= payloadCheckApiKey{
		ApiKey: apiKey,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Println(err)
		return false, err
	}

	response, err := http.Post(apiUri+"camera/verify-api-key/", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == 200 {
		return true , nil
	}
	return false, nil
}
