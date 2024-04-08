package readenv

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type payload struct {
	MacAdr   string `json:"macAdr"`
	Uuid     string `json:"uuid"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

func getMacAdr() (string, error) {
	ifas, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	macAdr := getMostRepeat(as)
	return macAdr, nil
}

func getMostRepeat[T comparable](slice []T) T {
	hashMap := make(map[T]int)
	var mostRepeat T
	maxCount := 0
	for _, t := range slice {
		hashMap[t]++
		if hashMap[t] > maxCount {
			mostRepeat = t
			maxCount = hashMap[t]
		}
	}
	return mostRepeat
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

func getApiKey(apiUri string, macAdr string, uuid string, name string, location string) (string, error) {
	payload := payload{
		MacAdr:   macAdr,
		Uuid:     uuid,
		Name:     name,
		Location: location,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	response, err := http.Post(apiUri+"/cameras/register", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return "", errors.New(response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	return result["apiKey"].(string), nil
}

func checkApiKeyValid(apiKey string) (bool, error) {
	// TODO: Need to implement this function
	return true, nil
}
